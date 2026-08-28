import assert from "node:assert/strict";
import test from "node:test";
import { issueEvidenceSummary } from "./issue-view-core.ts";

test("image-only issue never invents a map location", () => {
  const summary = issueEvidenceSummary({ detections: [{ id: 1, geometry: null }], assets: [{ id: 9 }] });
  assert.equal(summary.hasMapLocation, false);
  assert.match(summary.locationLabel, /仅影像级/);
  assert.equal(summary.completeEvidence, true);
});

test("located detections and assets form complete issue evidence", () => {
  const summary = issueEvidenceSummary({ detections: [{ id: 1, geometry: { type: "Point", coordinates: [120, 30] } }], assets: [{ id: 9 }] });
  assert.equal(summary.hasMapLocation, true);
  assert.equal(summary.detectionCount, 1);
  assert.equal(summary.assetCount, 1);
  assert.equal(summary.completeEvidence, true);
});
