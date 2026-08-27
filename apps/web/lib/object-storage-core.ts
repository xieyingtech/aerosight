import { createHash, createHmac, timingSafeEqual } from "node:crypto";

export type StoredObjectMetadata = {
  key: string;
  sizeBytes: number;
  checksumSha256: string;
  contentType: string | null;
  versionId: string | null;
};

export type PresignedObjectAccess = {
  url: string;
  expiresAt: string;
  headers: Record<string, string>;
};

export interface S3CompatibleObjectStorage {
  createPresignedUpload(input: {
    key: string;
    contentType: string;
    checksumSha256: string;
    expiresInSeconds: number;
  }): Promise<PresignedObjectAccess>;
  headObject(key: string): Promise<StoredObjectMetadata | null>;
  createPresignedDownload(input: {
    key: string;
    expiresInSeconds: number;
  }): Promise<PresignedObjectAccess>;
}

const safeSegment = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/;

export function sha256Hex(value: Uint8Array | string) {
  return createHash("sha256").update(value).digest("hex");
}

export function normalizeSha256(value: string) {
  const normalized = value.trim().toLowerCase();
  if (!/^[a-f0-9]{64}$/.test(normalized)) throw new Error("INVALID_SHA256");
  return normalized;
}

export function buildProjectObjectKey(projectId: number, uploadId: string, fileName: string) {
  if (!Number.isSafeInteger(projectId) || projectId <= 0) throw new Error("INVALID_PROJECT_ID");
  if (!safeSegment.test(uploadId)) throw new Error("INVALID_UPLOAD_ID");
  const normalizedName = fileName.normalize("NFKC").replace(/[^a-zA-Z0-9._-]+/g, "-");
  const trimmedName = normalizedName.replace(/^-+|-+$/g, "").slice(0, 128);
  if (!trimmedName || !safeSegment.test(trimmedName)) throw new Error("INVALID_FILE_NAME");
  return `projects/${projectId}/uploads/${uploadId}/${trimmedName}`;
}

export function assertProjectObjectKey(projectId: number, key: string) {
  if (key.includes("\\") || key.includes("\0") || key.startsWith("/") || key.includes("//")) {
    throw new Error("INVALID_OBJECT_KEY");
  }
  let decoded: string;
  try {
    decoded = decodeURIComponent(key);
  } catch {
    throw new Error("INVALID_OBJECT_KEY");
  }
  if (decoded !== key || decoded.split("/").some((segment) => segment === "." || segment === "..")) {
    throw new Error("INVALID_OBJECT_KEY");
  }
  const prefix = `projects/${projectId}/uploads/`;
  if (!key.startsWith(prefix)) throw new Error("OBJECT_KEY_PROJECT_MISMATCH");
  const segments = key.split("/");
  if (segments.length !== 5 || !safeSegment.test(segments[3]) || !safeSegment.test(segments[4])) {
    throw new Error("INVALID_OBJECT_KEY");
  }
  return key;
}

export async function verifyStoredObject(
  storage: S3CompatibleObjectStorage,
  input: { projectId: number; key: string; sizeBytes: number; checksumSha256: string }
) {
  assertProjectObjectKey(input.projectId, input.key);
  const metadata = await storage.headObject(input.key);
  if (!metadata) throw new Error("OBJECT_NOT_FOUND");
  if (metadata.sizeBytes !== input.sizeBytes) throw new Error("OBJECT_SIZE_MISMATCH");
  if (metadata.checksumSha256 !== normalizeSha256(input.checksumSha256)) {
    throw new Error("OBJECT_CHECKSUM_MISMATCH");
  }
  return metadata;
}

type MemoryObject = StoredObjectMetadata & { body: Uint8Array };

export class MemoryS3CompatibleObjectStorage implements S3CompatibleObjectStorage {
  readonly #objects = new Map<string, MemoryObject>();
  readonly #options: {
    baseUrl?: string;
    signingSecret?: string;
    now?: () => Date;
  };

  constructor(options: { baseUrl?: string; signingSecret?: string; now?: () => Date } = {}) {
    this.#options = options;
  }

  async createPresignedUpload(input: {
    key: string;
    contentType: string;
    checksumSha256: string;
    expiresInSeconds: number;
  }) {
    return this.#sign("PUT", input.key, input.expiresInSeconds, {
      "content-type": input.contentType,
      "x-amz-checksum-sha256": normalizeSha256(input.checksumSha256)
    });
  }

  async createPresignedDownload(input: { key: string; expiresInSeconds: number }) {
    if (!this.#objects.has(input.key)) throw new Error("OBJECT_NOT_FOUND");
    return this.#sign("GET", input.key, input.expiresInSeconds, {});
  }

  async headObject(key: string) {
    const object = this.#objects.get(key);
    if (!object) return null;
    const { body: _body, ...metadata } = object;
    return metadata;
  }

  async putObject(input: { key: string; body: Uint8Array | string; contentType?: string }) {
    const body = typeof input.body === "string" ? new TextEncoder().encode(input.body) : input.body;
    const versionId = String((this.#objects.get(input.key)?.versionId ? Number(this.#objects.get(input.key)?.versionId) : 0) + 1);
    const object: MemoryObject = {
      key: input.key,
      body,
      sizeBytes: body.byteLength,
      checksumSha256: sha256Hex(body),
      contentType: input.contentType ?? null,
      versionId
    };
    this.#objects.set(input.key, object);
    return object;
  }

  verifyPresignedUrl(url: string, method: "GET" | "PUT") {
    const parsed = new URL(url);
    const expires = parsed.searchParams.get("expires");
    const signature = parsed.searchParams.get("signature");
    if (!expires || !signature || Number(expires) <= this.#now().getTime()) return false;
    const key = parsed.pathname.replace(/^\//, "");
    const expected = this.#signature(method, key, expires);
    const actualBuffer = Buffer.from(signature);
    const expectedBuffer = Buffer.from(expected);
    return actualBuffer.length === expectedBuffer.length && timingSafeEqual(actualBuffer, expectedBuffer);
  }

  #sign(method: "GET" | "PUT", key: string, expiresInSeconds: number, headers: Record<string, string>) {
    if (!Number.isInteger(expiresInSeconds) || expiresInSeconds < 1 || expiresInSeconds > 3600) {
      throw new Error("INVALID_PRESIGN_EXPIRY");
    }
    const expiresAt = new Date(this.#now().getTime() + expiresInSeconds * 1000);
    const expires = String(expiresAt.getTime());
    const signature = this.#signature(method, key, expires);
    const baseUrl = this.#options.baseUrl ?? "https://object-storage.test";
    return {
      url: `${baseUrl.replace(/\/$/, "")}/${key}?expires=${expires}&signature=${signature}`,
      expiresAt: expiresAt.toISOString(),
      headers
    };
  }

  #signature(method: string, key: string, expires: string) {
    return createHmac("sha256", this.#options.signingSecret ?? "aerosight-object-storage-test")
      .update(`${method}\n${key}\n${expires}`)
      .digest("hex");
  }

  #now() {
    return this.#options.now?.() ?? new Date();
  }
}
