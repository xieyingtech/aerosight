import "server-only";

import { readFile } from "node:fs/promises";
import { relative, resolve, sep } from "node:path";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import {
  assertMediaActionAllowed,
  MediaAccessSigner,
  type MediaAccessAction,
  safeDownloadName
} from "@/lib/media-access-core";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type AssetAccessRow = {
  id: number;
  projectId: number;
  storageKey: string;
  mimeType: string | null;
  kind: string;
  fileName: string | null;
  sensitive: boolean;
};

async function loadAsset(projectId: number, assetId: number) {
  const result = await query<AssetAccessRow>(
    `select asset.id, asset.project_id as "projectId", asset.storage_key as "storageKey",
            asset.mime_type as "mimeType", asset.kind,
            intent.file_name as "fileName",
            (coalesce(asset.metadata_json->>'sensitive', 'false') = 'true' or exists (
              select 1 from evidence_links evidence
               where evidence.project_id = asset.project_id and evidence.asset_id = asset.id
                 and evidence.is_published
            )) as sensitive
       from assets asset
       left join asset_upload_intents intent
         on intent.project_id = asset.project_id and intent.asset_id = asset.id
      where asset.project_id = $1 and asset.id = $2 and asset.status = 'available'`,
    [projectId, assetId]
  );
  if (!result.rows[0]) throw new Error("ASSET_NOT_FOUND");
  return result.rows[0];
}

export async function issueMediaAccess(
  projectId: number,
  assetId: number,
  action: MediaAccessAction,
  requestId?: string | null
) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const asset = await loadAsset(projectId, assetId);
  assertMediaActionAllowed({ action, role: access.role, permissions: access.permissions, sensitive: asset.sensitive });
  const issue = () => new MediaAccessSigner(getWebRuntimeConfig().authSecret).issue({
    projectId, assetId, action, ttlSeconds: 120
  });
  if (action === "download" && asset.sensitive) {
    return withAuditedProjectWrite(
      {
        projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
        action: "media.sensitive_download", resourceType: "asset", resourceId: String(assetId),
        input: { action, assetId }, policyResult: { permission: "project:view", sensitive: true }
      },
      async () => issue()
    );
  }
  return issue();
}

export async function readAuthorizedMediaContent(input: {
  projectId: number;
  assetId: number;
  action: MediaAccessAction;
  expires: string;
  signature: string;
}) {
  const { access } = await requireCurrentProjectPermission(input.projectId, "project:view");
  const asset = await loadAsset(input.projectId, input.assetId);
  assertMediaActionAllowed({
    action: input.action, role: access.role, permissions: access.permissions, sensitive: asset.sensitive
  });
  const verified = new MediaAccessSigner(getWebRuntimeConfig().authSecret).verify(input);
  if (!verified) throw new Error("MEDIA_ACCESS_TOKEN_INVALID");

  const root = getWebRuntimeConfig().objectStorageLocalRoot;
  if (!root) throw new Error("OBJECT_STORAGE_UNAVAILABLE");
  if (!asset.storageKey.startsWith(`projects/${input.projectId}/`) || asset.storageKey.includes("\\")) {
    throw new Error("INVALID_ASSET_STORAGE_KEY");
  }
  const absoluteRoot = resolve(root);
  const objectPath = resolve(absoluteRoot, asset.storageKey);
  const scopedPath = relative(absoluteRoot, objectPath);
  if (scopedPath === ".." || scopedPath.startsWith(`..${sep}`)) throw new Error("INVALID_ASSET_STORAGE_KEY");
  const body = await readFile(objectPath);
  return {
    body,
    contentType: asset.mimeType ?? "application/octet-stream",
    disposition: input.action === "download"
      ? `attachment; filename="${safeDownloadName(asset.fileName, `asset-${asset.id}`)}"`
      : "inline"
  };
}
