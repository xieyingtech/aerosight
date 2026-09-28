import assert from "node:assert/strict";
import test from "node:test";
import { issueEvidenceSummary } from "./issue-view-core.ts";

test("image-only issue never invents a map location", () => {
  const summary = issueEvidenceSummary({ detections: [{ id: 1, geometry: null }], assets: [{ id: 9 }] });
  assert.equal(summary.hasMapLocation, false);
  assert.match(summary.locationLabel, /尚未关联地理位置/);
  assert.equal(summary.completeEvidence, true);
});

test("empty issue states that neither location nor evidence has been linked", () => {
  const summary = issueEvidenceSummary({ detections: [], assets: [] });
  assert.equal(summary.hasMapLocation, false);
  assert.equal(summary.hasEvidence, false);
  assert.equal(summary.locationLabel, "尚未关联地理位置");
  assert.equal(summary.evidenceLabel, "暂无关联证据");
});

test("media without detections is still linked evidence", () => {
  const summary = issueEvidenceSummary({ detections: [], assets: [{id: 9}] });
  assert.equal(summary.hasEvidence, true);
  assert.equal(summary.evidenceLabel, "0 条检测 · 1 个媒体");
});

test("located detections and assets form complete issue evidence", () => {
  const summary = issueEvidenceSummary({ detections: [{ id: 1, geometry: { type: "Point", coordinates: [120, 30] } }], assets: [{ id: 9 }] });
  assert.equal(summary.hasMapLocation, true);
  assert.equal(summary.detectionCount, 1);
  assert.equal(summary.assetCount, 1);
  assert.equal(summary.completeEvidence, true);
});
