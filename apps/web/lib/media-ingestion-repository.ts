import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";

import { db } from "@/lib/db";
import type {
  MediaAsset,
  MediaIngestionRepository,
  MediaKind,
  UploadIntent,
  UploadIntentStatus
} from "@/lib/media-ingestion-core";
import type { StoredObjectMetadata } from "@/lib/object-storage-core";
import { publishProjectEvent } from "@/lib/project-events";

type UploadIntentRow = {
  id: string;
  projectId: number;
  teamId: number;
  actorUserId: number | null;
  logicalKey: string;
  objectKey: string;
  fileName: string;
  kind: MediaKind;
  mimeType: string;
  expectedSizeBytes: string;
  expectedChecksumSha256: string;
  deviceId: number | null;
  taskRunId: number | null;
  issueId: number | null;
  status: UploadIntentStatus;
  assetId: number | null;
  expiresAt: Date;
  failureCode: string | null;
};

type AssetRow = {
  id: number;
  projectId: number;
  teamId: number;
  logicalKey: string;
  version: number;
  kind: MediaKind;
  mimeType: string;
  storageKey: string;
  objectVersion: string | null;
  sizeBytes: string;
  checksumSha256: string;
  status: "available";
  supersedesAssetId: number | null;
};

function toIntent(row: UploadIntentRow): UploadIntent {
  return {
    ...row,
    expectedSizeBytes: Number(row.expectedSizeBytes),
    expiresAt: row.expiresAt.toISOString()
  };
}

function toAsset(row: AssetRow): MediaAsset {
  return { ...row, sizeBytes: Number(row.sizeBytes) };
}

const intentProjection = `id, project_id as "projectId", team_id as "teamId",
  actor_user_id as "actorUserId", logical_key as "logicalKey", object_key as "objectKey",
  file_name as "fileName", kind, mime_type as "mimeType",
  expected_size_bytes as "expectedSizeBytes", expected_checksum_sha256 as "expectedChecksumSha256",
  device_id as "deviceId", task_run_id as "taskRunId", issue_id as "issueId",
  status, asset_id as "assetId", expires_at as "expiresAt", failure_code as "failureCode"`;

const assetProjection = `id, project_id as "projectId", team_id as "teamId",
  logical_key as "logicalKey", version, kind, mime_type as "mimeType",
  storage_key as "storageKey", object_version as "objectVersion", size_bytes as "sizeBytes",
  checksum_sha256 as "checksumSha256", status, supersedes_asset_id as "supersedesAssetId"`;

export class PostgresMediaIngestionRepository implements MediaIngestionRepository {
  async createUploadIntent(intent: UploadIntent) {
    const result = await db.query<UploadIntentRow>(
      `insert into asset_upload_intents (
         id, project_id, team_id, actor_user_id, logical_key, object_key, file_name,
         kind, mime_type, expected_size_bytes, expected_checksum_sha256,
         device_id, task_run_id, issue_id, status, expires_at
       ) values (
         $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'pending', $15
       ) returning ${intentProjection}`,
      [
        intent.id, intent.projectId, intent.teamId, intent.actorUserId, intent.logicalKey,
        intent.objectKey, intent.fileName, intent.kind, intent.mimeType, intent.expectedSizeBytes,
        intent.expectedChecksumSha256, intent.deviceId, intent.taskRunId, intent.issueId, intent.expiresAt
      ]
    );
    return toIntent(result.rows[0]);
  }

  async getUploadIntent(projectId: number, uploadId: string) {
    const result = await db.query<UploadIntentRow>(
      `select ${intentProjection}
         from asset_upload_intents
        where project_id = $1 and id = $2`,
      [projectId, uploadId]
    );
    return result.rows[0] ? toIntent(result.rows[0]) : null;
  }

  async markUploadFailed(projectId: number, uploadId: string, failureCode: string) {
    await db.query(
      `update asset_upload_intents
          set status = case when $3 = 'UPLOAD_EXPIRED' then 'expired' else 'failed' end,
              failure_code = $3
        where project_id = $1 and id = $2 and status = 'pending'`,
      [projectId, uploadId, failureCode]
    );
  }

  async completeUpload({ intent, object }: { intent: UploadIntent; object: StoredObjectMetadata }) {
    const client = await db.connect();
    try {
      await client.query("begin");
      const locked = await client.query<UploadIntentRow>(
        `select ${intentProjection}
           from asset_upload_intents
          where project_id = $1 and id = $2
          for update`,
        [intent.projectId, intent.id]
      );
      const current = locked.rows[0];
      if (!current) throw new Error("UPLOAD_INTENT_NOT_FOUND");
      if (current.status === "completed" && current.assetId) {
        const asset = await this.#loadAsset(client, intent.projectId, current.assetId);
        await client.query("commit");
        return asset;
      }
      if (current.status !== "pending") throw new Error(`UPLOAD_INTENT_${current.status.toUpperCase()}`);

      await client.query("select pg_advisory_xact_lock(hashtextextended($1, 0))", [
        `asset:${intent.projectId}:${current.logicalKey}`
      ]);
      const previous = await client.query<{ id: number; version: number }>(
        `select id, version from assets
          where project_id = $1 and logical_key = $2
          order by version desc limit 1`,
        [intent.projectId, current.logicalKey]
      );
      const previousAsset = previous.rows[0];
      const inserted = await client.query<AssetRow>(
        `insert into assets (
           project_id, team_id, device_id, task_run_id, issue_id, kind, mime_type,
           storage_key, logical_key, version, status, object_version, size_bytes,
           checksum, checksum_sha256, metadata_json, available_at, supersedes_asset_id
         ) values (
           $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'available', $11, $12,
           $13, $13, jsonb_build_object('uploadId', $14::text), now(), $15
         ) returning ${assetProjection}`,
        [
          intent.projectId, current.teamId, current.deviceId, current.taskRunId, current.issueId,
          current.kind, current.mimeType, current.objectKey, current.logicalKey,
          (previousAsset?.version ?? 0) + 1, object.versionId, object.sizeBytes,
          object.checksumSha256, current.id, previousAsset?.id ?? null
        ]
      );
      const asset = toAsset(inserted.rows[0]);
      await client.query(
        `update asset_upload_intents
            set status = 'completed', asset_id = $3, failure_code = null, completed_at = now()
          where project_id = $1 and id = $2`,
        [intent.projectId, intent.id, asset.id]
      );
      await publishProjectEvent(client, {
        projectId: intent.projectId,
        teamId: current.teamId,
        eventId: randomUUID(),
        eventType: "asset.available",
        payload: { assetId: asset.id, logicalKey: asset.logicalKey, version: asset.version }
      });
      await client.query("commit");
      return asset;
    } catch (error) {
      await client.query("rollback");
      throw error;
    } finally {
      client.release();
    }
  }

  async #loadAsset(client: PoolClient, projectId: number, assetId: number) {
    const result = await client.query<AssetRow>(
      `select ${assetProjection} from assets where project_id = $1 and id = $2 and status = 'available'`,
      [projectId, assetId]
    );
    if (!result.rows[0]) throw new Error("COMPLETED_ASSET_NOT_FOUND");
    return toAsset(result.rows[0]);
  }
}
