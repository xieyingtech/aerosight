import assert from "node:assert/strict";
import test from "node:test";
import { buildAlgorithmRunDiagnostics } from "./algorithm-run-view-core.ts";

const failedRun = {
  id: "00000000-0000-4000-8000-000000000001", status: "failed",
  inputSnapshot: {
    inputAsset: { assetId: 41, version: 3, checksumSha256: "a".repeat(64), mimeType: "image/jpeg", accessUrl: "https://signed.example/secret" },
    definition: { configurationSnapshotId: 8, providerType: "http-json", modelOrProcess: "construction-v2", mappingVersion: "mapping-v4" },
    parameters: { threshold: 0.72 }, context: { capturedAt: "2026-08-27T01:00:00Z" },
    callback: { token: "must-never-render" }
  },
  canonicalResult: { source: { modelRevision: "2.3.1", modelDigest: "sha256:abc" }, mappingDiagnostics: ["detection[0] missing geometry", 9] },
  rawResultObjectKey: "projects/2/algorithm-runs/run/raw-result.json", rawResultChecksumSha256: "b".repeat(64),
  createdAt: new Date("2026-08-27T01:00:00Z"), startedAt: new Date("2026-08-27T01:00:01Z"), finishedAt: new Date("2026-08-27T01:00:04Z"),
  errorCode: "provider_format_drift", errorMessage: "mapping failed"
};

test("failed run exposes configuration and provider provenance without signed input secrets", () => {
  const view = buildAlgorithmRunDiagnostics(failedRun, new Set(["algorithm:manage"]));
  assert.equal(view.durationMs, 3000);
  assert.deepEqual(view.diagnostics, ["detection[0] missing geometry"]);
  assert.equal(view.provenance.mappingVersion, "mapping-v4");
  assert.equal(view.provenance.modelRevision, "2.3.1");
  assert.equal(view.input.assetId, 41);
  assert.ok(view.rawResult?.checksumSha256);
  assert.equal(view.retryAllowed, true);
  assert.doesNotMatch(JSON.stringify(view), /signed\.example|must-never-render/);
});

test("retry is denied to viewers and for non-failed runs", () => {
  assert.equal(buildAlgorithmRunDiagnostics(failedRun, new Set(["project:view"])).retryAllowed, false);
  assert.equal(buildAlgorithmRunDiagnostics({ ...failedRun, status: "succeeded" }, new Set(["algorithm:manage"])).retryAllowed, false);
});
