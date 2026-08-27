import { createHash } from "node:crypto";

export type RetentionPolicy = {
  id: string;
  projectId: number;
  retentionDays: number;
  derivativeRetentionDays: number;
};

export type RetentionAsset = {
  id: number;
  projectId: number;
  status: "available" | "pending" | "failed" | "deleted";
  availableAt: Date;
  storageKey: string;
  checksumSha256?: string | null;
  legalHold: boolean;
  retentionHoldUntil?: Date | null;
  publishedEvidence: boolean;
  publishedReportEvidence: boolean;
  sourceAssetId?: number | null;
};

export type RetentionHold = {
  projectId: number;
  assetId: number;
  status: "active" | "released";
  holdUntil?: Date | null;
};

export type CleanupDecision = {
  assetId: number;
  decision: "retain" | "delete";
  reasonCode: string;
  expiresAt: string;
};

export type CleanupPlan = {
  projectId: number;
  policyId: string;
  mode: "dry_run" | "execute";
  candidateAssetIds: number[];
  decisions: CleanupDecision[];
};

const dayMilliseconds = 24 * 60 * 60 * 1000;

export function planRetentionCleanup(input: {
  policy: RetentionPolicy;
  assets: RetentionAsset[];
  holds: RetentionHold[];
  now: Date;
  mode?: "dry_run" | "execute";
}): CleanupPlan {
  const { policy, assets, holds, now } = input;
  if (policy.projectId <= 0 || policy.retentionDays <= 0 || policy.derivativeRetentionDays <= 0) {
    throw new Error("RETENTION_POLICY_INVALID");
  }
  if (assets.some((asset) => asset.projectId !== policy.projectId)
    || holds.some((hold) => hold.projectId !== policy.projectId)) throw new Error("RETENTION_SCOPE_MISMATCH");

  const activeHolds = new Set(holds.filter((hold) => hold.status === "active"
    && (!hold.holdUntil || hold.holdUntil.getTime() > now.getTime())).map((hold) => hold.assetId));
  const protectedAssets = new Set(assets.filter((asset) => asset.legalHold
    || activeHolds.has(asset.id)
    || Boolean(asset.retentionHoldUntil && asset.retentionHoldUntil.getTime() > now.getTime())
    || asset.publishedEvidence
    || asset.publishedReportEvidence).map((asset) => asset.id));

  let propagated = true;
  while (propagated) {
    propagated = false;
    for (const asset of assets) {
      if (asset.sourceAssetId && protectedAssets.has(asset.sourceAssetId) && !protectedAssets.has(asset.id)) {
        protectedAssets.add(asset.id);
        propagated = true;
      }
    }
  }

  const decisions = assets.map((asset): CleanupDecision => {
    const retentionDays = asset.sourceAssetId ? policy.derivativeRetentionDays : policy.retentionDays;
    const expiresAt = new Date(asset.availableAt.getTime() + retentionDays * dayMilliseconds);
    if (asset.status !== "available") return { assetId: asset.id, decision: "retain", reasonCode: "ASSET_NOT_AVAILABLE", expiresAt: expiresAt.toISOString() };
    if (protectedAssets.has(asset.id)) return { assetId: asset.id, decision: "retain", reasonCode: asset.sourceAssetId ? "SOURCE_EVIDENCE_HOLD" : "EVIDENCE_OR_RETENTION_HOLD", expiresAt: expiresAt.toISOString() };
    if (expiresAt.getTime() > now.getTime()) return { assetId: asset.id, decision: "retain", reasonCode: "RETENTION_NOT_EXPIRED", expiresAt: expiresAt.toISOString() };
    return { assetId: asset.id, decision: "delete", reasonCode: asset.sourceAssetId ? "DERIVATIVE_RETENTION_EXPIRED" : "RETENTION_EXPIRED", expiresAt: expiresAt.toISOString() };
  });

  return {
    projectId: policy.projectId,
    policyId: policy.id,
    mode: input.mode ?? "dry_run",
    candidateAssetIds: decisions.filter((decision) => decision.decision === "delete").map((decision) => decision.assetId),
    decisions
  };
}

export function deletionTombstones(plan: CleanupPlan, assets: RetentionAsset[], deletedAt: Date) {
  if (plan.mode !== "execute") return [];
  const byId = new Map(assets.map((asset) => [asset.id, asset]));
  return plan.candidateAssetIds.map((assetId) => {
    const asset = byId.get(assetId);
    if (!asset || asset.projectId !== plan.projectId) throw new Error("RETENTION_ASSET_NOT_FOUND");
    return {
      projectId: plan.projectId,
      policyId: plan.policyId,
      assetId,
      storageKeyHash: createHash("sha256").update(asset.storageKey).digest("hex"),
      checksumSha256: asset.checksumSha256 ?? null,
      reasonCode: plan.decisions.find((decision) => decision.assetId === assetId)?.reasonCode ?? "RETENTION_EXPIRED",
      deletedAt: deletedAt.toISOString()
    };
  });
}
