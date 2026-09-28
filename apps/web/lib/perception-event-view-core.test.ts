import assert from "node:assert/strict";
import test from "node:test";
import { buildPerceptionEventEvidence } from "./perception-event-view-core.ts";

test("image-only fixture preserves an arbitrary algorithm title without inventing map position", () => {
  const model = buildPerceptionEventEvidence({ event: { id: "e-1", title: "OCR 文字识别", status: "open" }, detections: [{
    id: 1, label: "text", confidence: 0.8, locationQuality: "unavailable",
    geographicGeometry: null, pixelGeometry: { type: "bbox", x: 1, y: 2, width: 3, height: 4 }, inputAssetId: 9
  }], feedback: [] });
  assert.equal(model.event.title, "OCR 文字识别");
  assert.equal(model.event.hasMapLocation, false);
  assert.match(model.event.locationSummary, /仅展示影像内标注/);
  assert.equal(model.detections[0].geographicGeometry, null);
});

test("missing event title uses a generic result title", () => {
  const model = buildPerceptionEventEvidence({event: {}, detections: [], feedback: []});
  assert.equal(model.event.title, "算法识别结果");
});

test("complete evidence fixture retains location quality, model version, original asset and annotation", () => {
  const model = buildPerceptionEventEvidence({ event: { id: "e-2", status: "investigating" }, detections: [{
    id: 2, label: "object:region", confidence: 0.94, locationQuality: "estimated",
    geographicGeometry: { type: "Polygon", coordinates: [[[120,30],[120.1,30],[120,30.1],[120,30]]] },
    horizontalErrorMeters: 2.4, projectionMethod: "nadir-ray-ground-plane",
    pixelGeometry: { type: "polygon", coordinates: [[1,2],[3,4],[5,6]] },
    modelOrProcess: "detection-v2", modelVersion: 3, mappingVersion: "detection/v1",
    inputAssetId: 9, assetVersion: 4, assetChecksumSha256: "a".repeat(64), mimeType: "image/jpeg"
  }], feedback: [] });
  assert.equal(model.event.hasMapLocation, true);
  assert.equal(model.detections[0].modelOrProcess, "detection-v2");
  assert.equal(model.detections[0].assetVersion, 4);
  assert.equal((model.detections[0].pixelGeometry as { type: string }).type, "polygon");
});
