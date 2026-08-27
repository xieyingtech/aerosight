import assert from "node:assert/strict";
import test from "node:test";
import { mapSuspectedConstructionDetections, suspectedConstructionTemplate, type DetectionMapping } from "./suspected-construction-template.ts";

const asset = { assetId: 17, version: 4, checksumSha256: "c".repeat(64), mimeType: "image/jpeg" };

test("template identifies itself as a versioned machine clue rather than a legal conclusion", () => {
  assert.equal(suspectedConstructionTemplate.templateKey, "suspected-construction");
  assert.equal(suspectedConstructionTemplate.templateVersion, 1);
  assert.match(suspectedConstructionTemplate.description, /机器线索/);
  assert.equal(suspectedConstructionTemplate.mappingVersion, "suspected-construction/v1");
});

test("maps object bbox fixture with label, confidence and immutable asset lineage", () => {
  const detections = mapSuspectedConstructionDetections({
    response: { results: [{ id: "bbox-1", class: "new_building", score: 0.93, geometry: { type: "bbox", x: 10, y: 20, width: 80, height: 45 } }] },
    mapping: suspectedConstructionTemplate.outputMapping,
    labelMapping: suspectedConstructionTemplate.labelMapping,
    inputAsset: asset
  });
  assert.deepEqual(detections[0], {
    detectionKey: "bbox-1", label: "suspected-construction:new-building", confidence: 0.93,
    pixelGeometry: { type: "bbox", x: 10, y: 20, width: 80, height: 45 },
    inputAsset: asset, attributes: { externalLabel: "new_building" }
  });
});

test("maps vendor polygon and compact bbox array fixtures through versioned mappings", () => {
  const fixtures: Array<{ response: unknown; mapping: DetectionMapping; expectedType: string }> = [
    {
      response: { predictions: [{ key: "poly-1", category: "extension", probability: 0.81, contour: [[1, 2], [9, 2], [8, 7]] }] },
      mapping: { detectionsPath: "predictions", keyPath: "key", labelPath: "category", confidencePath: "probability", geometryPath: "contour", geometryFormat: "polygon-array" },
      expectedType: "polygon"
    },
    {
      response: { objects: [{ uuid: "box-2", label: "earthwork", confidence: 0.74, bounds: [4, 5, 20, 12] }] },
      mapping: { detectionsPath: "objects", keyPath: "uuid", labelPath: "label", confidencePath: "confidence", geometryPath: "bounds", geometryFormat: "bbox-array" },
      expectedType: "bbox"
    }
  ];
  for (const fixture of fixtures) {
    const [detection] = mapSuspectedConstructionDetections({ ...fixture, labelMapping: suspectedConstructionTemplate.labelMapping, inputAsset: asset });
    assert.equal(detection.pixelGeometry.type, fixture.expectedType);
    assert.equal(detection.inputAsset.assetId, 17);
  }
});

test("unmapped labels and malformed geometry fail instead of creating false detections", () => {
  assert.throws(() => mapSuspectedConstructionDetections({
    response: { results: [{ id: "bad", class: "legal_conclusion", score: 0.9, geometry: { type: "bbox", x: 0, y: 0, width: -1, height: 2 } }] },
    mapping: suspectedConstructionTemplate.outputMapping, labelMapping: suspectedConstructionTemplate.labelMapping, inputAsset: asset
  }), /DETECTION_LABEL_UNMAPPED|DETECTION_MAPPING_INVALID/);
});
