import { randomUUID } from "node:crypto";

import {
  assertProjectObjectKey,
  buildProjectObjectKey,
  normalizeSha256,
  type PresignedObjectAccess,
  type S3CompatibleObjectStorage,
  type StoredObjectMetadata,
  verifyStoredObject
} from "./object-storage-core.ts";

export type MediaKind = "image" | "video" | "point-cloud" | "report" | "other";
export type UploadIntentStatus = "pending" | "completed" | "failed" | "expired";

export type UploadIntent = {
  id: string;
  projectId: number;
  teamId: number;
  actorUserId: number | null;
  logicalKey: string;
  objectKey: string;
  fileName: string;
  kind: MediaKind;
  mimeType: string;
  expectedSizeBytes: number;
  expectedChecksumSha256: string;
  deviceId: number | null;
  taskRunId: number | null;
  issueId: number | null;
  status: UploadIntentStatus;
  assetId: number | null;
  expiresAt: string;
  failureCode: string | null;
};

export type MediaAsset = {
  id: number;
  projectId: number;
  teamId: number;
  logicalKey: string;
  version: number;
  kind: MediaKind;
  mimeType: string;
  storageKey: string;
  objectVersion: string | null;
  sizeBytes: number;
  checksumSha256: string;
  status: "available";
  supersedesAssetId: number | null;
};

export type EvidenceLink = {
  id: number;
  projectId: number;
  targetType: "detection" | "track" | "event" | "report" | "issue" | "task_run";
  targetId: string;
  assetId: number;
  assetVersion: number;
  assetChecksumSha256: string;
  startOffsetMs: number | null;
  endOffsetMs: number | null;
  published: boolean;
};

export interface MediaIngestionRepository {
  createUploadIntent(intent: UploadIntent): Promise<UploadIntent>;
  getUploadIntent(projectId: number, uploadId: string): Promise<UploadIntent | null>;
  markUploadFailed(projectId: number, uploadId: string, failureCode: string): Promise<void>;
  completeUpload(input: {
    intent: UploadIntent;
    object: StoredObjectMetadata;
  }): Promise<MediaAsset>;
}

export async function startMediaUpload(
  repository: MediaIngestionRepository,
  storage: S3CompatibleObjectStorage,
  input: {
    projectId: number;
    teamId: number;
    actorUserId?: number;
    logicalKey: string;
    fileName: string;
    kind: MediaKind;
    mimeType: string;
    sizeBytes: number;
    checksumSha256: string;
    deviceId?: number;
    taskRunId?: number;
    issueId?: number;
    expiresInSeconds?: number;
    now?: Date;
  }
): Promise<{ intent: UploadIntent; upload: PresignedObjectAccess }> {
  if (!Number.isSafeInteger(input.sizeBytes) || input.sizeBytes < 0) throw new Error("INVALID_OBJECT_SIZE");
  if (!input.logicalKey.trim() || input.logicalKey.length > 256) throw new Error("INVALID_LOGICAL_KEY");
  if (!input.mimeType.includes("/") || input.mimeType.length > 255) throw new Error("INVALID_MIME_TYPE");
  const id = randomUUID();
  const expiresInSeconds = input.expiresInSeconds ?? 900;
  const now = input.now ?? new Date();
  const intent: UploadIntent = {
    id,
    projectId: input.projectId,
    teamId: input.teamId,
    actorUserId: input.actorUserId ?? null,
    logicalKey: input.logicalKey.trim(),
    objectKey: buildProjectObjectKey(input.projectId, id, input.fileName),
    fileName: input.fileName,
    kind: input.kind,
    mimeType: input.mimeType,
    expectedSizeBytes: input.sizeBytes,
    expectedChecksumSha256: normalizeSha256(input.checksumSha256),
    deviceId: input.deviceId ?? null,
    taskRunId: input.taskRunId ?? null,
    issueId: input.issueId ?? null,
    status: "pending",
    assetId: null,
    expiresAt: new Date(now.getTime() + expiresInSeconds * 1000).toISOString(),
    failureCode: null
  };
  const storedIntent = await repository.createUploadIntent(intent);
  try {
    const upload = await storage.createPresignedUpload({
      key: storedIntent.objectKey,
      contentType: storedIntent.mimeType,
      checksumSha256: storedIntent.expectedChecksumSha256,
      expiresInSeconds
    });
    return { intent: storedIntent, upload };
  } catch (error) {
    await repository.markUploadFailed(input.projectId, id, "PRESIGN_FAILED");
    throw error;
  }
}

export async function completeMediaUpload(
  repository: MediaIngestionRepository,
  storage: S3CompatibleObjectStorage,
  input: { projectId: number; uploadId: string; now?: Date }
) {
  const intent = await repository.getUploadIntent(input.projectId, input.uploadId);
  if (!intent) throw new Error("UPLOAD_INTENT_NOT_FOUND");
  if (intent.projectId !== input.projectId) throw new Error("UPLOAD_INTENT_NOT_FOUND");
  if (intent.status === "completed") {
    return repository.completeUpload({ intent, object: {
      key: intent.objectKey,
      sizeBytes: intent.expectedSizeBytes,
      checksumSha256: intent.expectedChecksumSha256,
      contentType: intent.mimeType,
      versionId: null
    } });
  }
  if (intent.status !== "pending") throw new Error(`UPLOAD_INTENT_${intent.status.toUpperCase()}`);
  if (new Date(intent.expiresAt).getTime() <= (input.now ?? new Date()).getTime()) {
    await repository.markUploadFailed(input.projectId, input.uploadId, "UPLOAD_EXPIRED");
    throw new Error("UPLOAD_EXPIRED");
  }
  assertProjectObjectKey(input.projectId, intent.objectKey);
  try {
    const object = await verifyStoredObject(storage, {
      projectId: input.projectId,
      key: intent.objectKey,
      sizeBytes: intent.expectedSizeBytes,
      checksumSha256: intent.expectedChecksumSha256
    });
    return await repository.completeUpload({ intent, object });
  } catch (error) {
    const code = error instanceof Error ? error.message : "OBJECT_VERIFICATION_FAILED";
    // A missing object is an interrupted upload and remains retryable until expiry.
    if (code !== "OBJECT_NOT_FOUND") {
      await repository.markUploadFailed(input.projectId, input.uploadId, code);
    }
    throw error;
  }
}

export class MemoryMediaIngestionRepository implements MediaIngestionRepository {
  readonly intents = new Map<string, UploadIntent>();
  readonly assets = new Map<number, MediaAsset>();
  readonly evidenceLinks = new Map<number, EvidenceLink>();
  #nextAssetId = 1;
  #nextEvidenceId = 1;

  async createUploadIntent(intent: UploadIntent) {
    const key = this.#intentKey(intent.projectId, intent.id);
    if (this.intents.has(key)) throw new Error("UPLOAD_INTENT_EXISTS");
    this.intents.set(key, structuredClone(intent));
    return structuredClone(intent);
  }

  async getUploadIntent(projectId: number, uploadId: string) {
    return structuredClone(this.intents.get(this.#intentKey(projectId, uploadId)) ?? null);
  }

  async markUploadFailed(projectId: number, uploadId: string, failureCode: string) {
    const key = this.#intentKey(projectId, uploadId);
    const intent = this.intents.get(key);
    if (!intent || intent.status === "completed") return;
    this.intents.set(key, { ...intent, status: failureCode === "UPLOAD_EXPIRED" ? "expired" : "failed", failureCode });
  }

  async completeUpload({ intent, object }: { intent: UploadIntent; object: StoredObjectMetadata }) {
    const key = this.#intentKey(intent.projectId, intent.id);
    const current = this.intents.get(key);
    if (!current) throw new Error("UPLOAD_INTENT_NOT_FOUND");
    if (current.status === "completed" && current.assetId) {
      const existing = this.assets.get(current.assetId);
      if (!existing) throw new Error("COMPLETED_ASSET_NOT_FOUND");
      return structuredClone(existing);
    }
    if (current.status !== "pending") throw new Error(`UPLOAD_INTENT_${current.status.toUpperCase()}`);
    const previous = [...this.assets.values()]
      .filter((asset) => asset.projectId === intent.projectId && asset.logicalKey === intent.logicalKey)
      .sort((a, b) => b.version - a.version)[0];
    const asset: MediaAsset = {
      id: this.#nextAssetId++,
      projectId: intent.projectId,
      teamId: intent.teamId,
      logicalKey: intent.logicalKey,
      version: (previous?.version ?? 0) + 1,
      kind: intent.kind,
      mimeType: intent.mimeType,
      storageKey: intent.objectKey,
      objectVersion: object.versionId,
      sizeBytes: object.sizeBytes,
      checksumSha256: object.checksumSha256,
      status: "available",
      supersedesAssetId: previous?.id ?? null
    };
    this.assets.set(asset.id, asset);
    this.intents.set(key, { ...current, status: "completed", assetId: asset.id, failureCode: null });
    return structuredClone(asset);
  }

  linkEvidence(input: {
    projectId: number;
    targetType: EvidenceLink["targetType"];
    targetId: string;
    assetId: number;
    startOffsetMs?: number;
    endOffsetMs?: number;
    published?: boolean;
  }) {
    const asset = this.assets.get(input.assetId);
    if (!asset || asset.projectId !== input.projectId) throw new Error("ASSET_NOT_FOUND");
    const link: EvidenceLink = {
      id: this.#nextEvidenceId++,
      projectId: input.projectId,
      targetType: input.targetType,
      targetId: input.targetId,
      assetId: asset.id,
      assetVersion: asset.version,
      assetChecksumSha256: asset.checksumSha256,
      startOffsetMs: input.startOffsetMs ?? null,
      endOffsetMs: input.endOffsetMs ?? null,
      published: input.published ?? false
    };
    this.evidenceLinks.set(link.id, link);
    return structuredClone(link);
  }

  #intentKey(projectId: number, uploadId: string) {
    return `${projectId}:${uploadId}`;
  }
}
