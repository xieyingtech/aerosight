import assert from "node:assert/strict";
import test from "node:test";
import { buildPerceptionEventEvidence } from "./perception-event-view-core.ts";

test("image-only fixture shows suspected wording and no invented map position", () => {
  const model = buildPerceptionEventEvidence({ event: { id: "e-1", title: "illegal building", status: "open" }, detections: [{
    id: 1, label: "suspected-construction", confidence: 0.8, locationQuality: "unavailable",
    geographicGeometry: null, pixelGeometry: { type: "bbox", x: 1, y: 2, width: 3, height: 4 }, inputAssetId: 9
  }], feedback: [] });
  assert.equal(model.event.title, "疑似违建");
  assert.equal(model.event.hasMapLocation, false);
  assert.match(model.event.locationSummary, /仅展示影像内标注/);
  assert.equal(model.detections[0].geographicGeometry, null);
});

test("complete evidence fixture retains location quality, model version, original asset and annotation", () => {
  const model = buildPerceptionEventEvidence({ event: { id: "e-2", status: "investigating" }, detections: [{
    id: 2, label: "suspected-construction:new-building", confidence: 0.94, locationQuality: "estimated",
    geographicGeometry: { type: "Polygon", coordinates: [[[120,30],[120.1,30],[120,30.1],[120,30]]] },
    horizontalErrorMeters: 2.4, projectionMethod: "nadir-ray-ground-plane",
    pixelGeometry: { type: "polygon", coordinates: [[1,2],[3,4],[5,6]] },
    modelOrProcess: "construction-v2", modelVersion: 3, mappingVersion: "suspected-construction/v1",
    inputAssetId: 9, assetVersion: 4, assetChecksumSha256: "a".repeat(64), mimeType: "image/jpeg"
  }], feedback: [] });
  assert.equal(model.event.hasMapLocation, true);
  assert.equal(model.detections[0].modelOrProcess, "construction-v2");
  assert.equal(model.detections[0].assetVersion, 4);
  assert.equal((model.detections[0].pixelGeometry as { type: string }).type, "polygon");
});
