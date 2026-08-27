import assert from "node:assert/strict";
import test from "node:test";

import { deletionTombstones, planRetentionCleanup, type RetentionAsset } from "./retention-cleanup-core.ts";

const old = new Date("2026-01-01T00:00:00Z");
const recent = new Date("2026-08-20T00:00:00Z");
const checksum = "a".repeat(64);
const assets: RetentionAsset[] = [
  { id: 1, projectId: 17, status: "available", availableAt: old, storageKey: "projects/17/evidence.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: true, publishedReportEvidence: false },
  { id: 2, projectId: 17, status: "available", availableAt: old, storageKey: "projects/17/expired.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: false, publishedReportEvidence: false },
  { id: 3, projectId: 17, status: "available", availableAt: old, storageKey: "projects/17/evidence-thumb.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: false, publishedReportEvidence: false, sourceAssetId: 1 },
  { id: 4, projectId: 17, status: "available", availableAt: old, storageKey: "projects/17/expired-thumb.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: false, publishedReportEvidence: false, sourceAssetId: 2 },
  { id: 5, projectId: 17, status: "available", availableAt: recent, storageKey: "projects/17/recent.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: false, publishedReportEvidence: false },
  { id: 6, projectId: 17, status: "available", availableAt: old, storageKey: "projects/17/held.jpg", checksumSha256: checksum, legalHold: false, publishedEvidence: false, publishedReportEvidence: false }
];
const policy = { id: "policy-7", projectId: 17, retentionDays: 90, derivativeRetentionDays: 30 };
const now = new Date("2026-08-27T00:00:00Z");

test("dry-run cleanup preserves evidence and holds while identifying ordinary and derived expiry", () => {
  const plan = planRetentionCleanup({
    policy, assets, now,
    holds: [{ projectId: 17, assetId: 6, status: "active", holdUntil: new Date("2026-09-01T00:00:00Z") }]
  });
  assert.equal(plan.mode, "dry_run");
  assert.deepEqual(plan.candidateAssetIds, [2, 4]);
  assert.equal(plan.decisions.find((item) => item.assetId === 1)?.decision, "retain");
  assert.equal(plan.decisions.find((item) => item.assetId === 3)?.reasonCode, "SOURCE_EVIDENCE_HOLD");
  assert.equal(plan.decisions.find((item) => item.assetId === 6)?.decision, "retain");
  assert.deepEqual(deletionTombstones(plan, assets, now), []);
});

test("execute plan creates minimal deletion tombstones without raw storage keys", () => {
  const plan = planRetentionCleanup({ policy, assets, holds: [], now, mode: "execute" });
  const tombstones = deletionTombstones(plan, assets, now);
  assert.deepEqual(tombstones.map((item) => item.assetId), [2, 4, 6]);
  assert.ok(tombstones.every((item) => /^[a-f0-9]{64}$/.test(item.storageKeyHash)));
  assert.ok(!JSON.stringify(tombstones).includes("projects/17/"));
});

test("cleanup rejects cross-project candidates before planning", () => {
  assert.throws(() => planRetentionCleanup({
    policy, assets: [...assets, { ...assets[0], id: 99, projectId: 18 }], holds: [], now
  }), /RETENTION_SCOPE_MISMATCH/);
});
