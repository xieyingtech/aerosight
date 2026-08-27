import assert from "node:assert/strict";
import test from "node:test";

import {
  algorithmAdapterInputJsonSchema, algorithmAdapterInputSchema,
  canonicalAlgorithmResultJsonSchema, canonicalAlgorithmResultSchema
} from "./algorithm-adapter-contract.ts";

const checksum = "a".repeat(64);
const input = {
  schemaVersion: "aerosight.algorithm.input/v1" as const,
  runId: "20000000-0000-4000-8000-000000000001",
  projectId: 17,
  definition: { definitionVersionId: 3, providerType: "http-json" as const, modelOrProcess: "construction-v2", executionMode: "synchronous" as const, mappingVersion: "mapping-v4" },
  inputAsset: { assetId: 42, version: 2, checksumSha256: checksum, mimeType: "image/jpeg", accessUrl: "https://objects.example.test/signed", accessExpiresAt: "2026-08-28T10:05:00Z" },
  context: { capturedAt: "2026-08-28T10:00:00Z", deviceId: 8, taskRunId: 19, position: { longitude: 121.47, latitude: 31.23, altitudeMeters: 80 }, coordinateReference: "EPSG:4326", calibrationVersion: "camera-v3", quality: { horizontalAccuracyMeters: 1.2 } },
  parameters: { threshold: 0.7 }, callback: null
};

const result = {
  schemaVersion: "aerosight.algorithm.result/v1" as const,
  runId: input.runId,
  source: { providerType: "http-json" as const, providerId: 6, modelOrProcess: "construction-v2", modelVersion: "2.3.1", definitionVersionId: 3, mappingVersion: "mapping-v4" },
  inputAsset: { assetId: 42, version: 2, checksumSha256: checksum, mimeType: "image/jpeg" },
  capturedAt: input.context.capturedAt,
  detections: [{ detectionKey: "building-1", label: "suspected-construction", confidence: 0.91,
    pixelGeometry: { type: "polygon" as const, coordinates: [[10, 10], [100, 10], [100, 100], [10, 10]] },
    geographicGeometry: null, attributes: { reviewRequired: true }, derivedAssetIds: [] }],
  rawResult: { objectKey: "projects/17/algorithm-runs/raw.json", checksumSha256: checksum, contentType: "application/json" },
  completedAt: "2026-08-28T10:00:04Z"
};

test("input contract covers scoped asset and spatiotemporal context", () => {
  assert.equal(algorithmAdapterInputSchema.parse(input).definition.mappingVersion, "mapping-v4");
  assert.equal(algorithmAdapterInputSchema.safeParse({ ...input, projectId: 0 }).success, false);
});

test("callback credentials are required only for callback execution", () => {
  assert.equal(algorithmAdapterInputSchema.safeParse({ ...input, definition: { ...input.definition, executionMode: "callback" } }).success, false);
  assert.equal(algorithmAdapterInputSchema.safeParse({ ...input, definition: { ...input.definition, executionMode: "callback" }, callback: { url: "https://aerosight.example.test/callback", token: "x".repeat(32) } }).success, true);
});

test("canonical result requires model, mapping, asset and raw-result lineage", () => {
  assert.equal(canonicalAlgorithmResultSchema.parse(result).detections[0].label, "suspected-construction");
  const missingSource = structuredClone(result) as Record<string, unknown>;
  delete missingSource.source;
  assert.equal(canonicalAlgorithmResultSchema.safeParse(missingSource).success, false);
  assert.equal(canonicalAlgorithmResultSchema.safeParse({ ...result, rawResult: { objectKey: "raw", checksumSha256: "bad", contentType: "application/json" } }).success, false);
});

test("contracts export JSON Schema 2020-12 with required fields", () => {
  assert.equal(algorithmAdapterInputJsonSchema.$schema, "https://json-schema.org/draft/2020-12/schema");
  assert.ok((algorithmAdapterInputJsonSchema.required as string[]).includes("inputAsset"));
  assert.ok((canonicalAlgorithmResultJsonSchema.required as string[]).includes("rawResult"));
});
