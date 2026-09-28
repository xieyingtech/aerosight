import assert from "node:assert/strict";
import test from "node:test";
import { mapAlgorithmDetections, type DetectionMapping } from "./algorithm-detection-mapping.ts";

const asset = { assetId: 17, version: 4, checksumSha256: "c".repeat(64), mimeType: "image/jpeg" };

test("maps object bbox fixture with label, confidence and immutable asset lineage", () => {
  const detections = mapAlgorithmDetections({
    response: { results: [{ id: "bbox-1", class: "object", score: 0.93, geometry: { type: "bbox", x: 10, y: 20, width: 80, height: 45 } }] },
    mapping: { detectionsPath: "results", keyPath: "id", labelPath: "class", confidencePath: "score", geometryPath: "geometry", geometryTypePath: "geometry.type", geometryFormat: "object" } satisfies DetectionMapping,
    labelMapping: { object: "object", region: "region", target: "target" },
    inputAsset: asset
  });
  assert.deepEqual(detections[0], {
    detectionKey: "bbox-1", label: "object", confidence: 0.93,
    pixelGeometry: { type: "bbox", x: 10, y: 20, width: 80, height: 45 },
    inputAsset: asset, attributes: { externalLabel: "object" }
  });
});

test("maps vendor polygon and compact bbox array fixtures through versioned mappings", () => {
  const fixtures: Array<{ response: unknown; mapping: DetectionMapping; expectedType: string }> = [
    {
      response: { predictions: [{ key: "poly-1", category: "region", probability: 0.81, contour: [[1, 2], [9, 2], [8, 7]] }] },
      mapping: { detectionsPath: "predictions", keyPath: "key", labelPath: "category", confidencePath: "probability", geometryPath: "contour", geometryFormat: "polygon-array" },
      expectedType: "polygon"
    },
    {
      response: { objects: [{ uuid: "box-2", label: "target", confidence: 0.74, bounds: [4, 5, 20, 12] }] },
      mapping: { detectionsPath: "objects", keyPath: "uuid", labelPath: "label", confidencePath: "confidence", geometryPath: "bounds", geometryFormat: "bbox-array" },
      expectedType: "bbox"
    }
  ];
  for (const fixture of fixtures) {
    const [detection] = mapAlgorithmDetections({ ...fixture, labelMapping: { object: "object", region: "region", target: "target" }, inputAsset: asset });
    assert.equal(detection.pixelGeometry.type, fixture.expectedType);
    assert.equal(detection.inputAsset.assetId, 17);
  }
});

test("unmapped labels and malformed geometry fail instead of creating false detections", () => {
  assert.throws(() => mapAlgorithmDetections({
    response: { results: [{ id: "bad", class: "unmapped", score: 0.9, geometry: { type: "bbox", x: 0, y: 0, width: -1, height: 2 } }] },
    mapping: { detectionsPath: "results", keyPath: "id", labelPath: "class", confidencePath: "score", geometryPath: "geometry", geometryTypePath: "geometry.type", geometryFormat: "object" } satisfies DetectionMapping, labelMapping: { object: "object", region: "region", target: "target" }, inputAsset: asset
  }), /DETECTION_LABEL_UNMAPPED|DETECTION_MAPPING_INVALID/);
});
