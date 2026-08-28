import assert from "node:assert/strict";
import test from "node:test";

import { coerceSchemaParameters, startAlgorithmRunInputSchema } from "./algorithm-run-input.ts";

test("generic schema parameters support a newly cataloged OCR algorithm", () => {
  const schema = { properties: { language: { type: "string" }, confidence: { type: "number" }, deskew: { type: "boolean" } } };
  const parameters = coerceSchemaParameters(schema, { language: "zh-CN", confidence: "0.8", deskew: "true" });
  const input = startAlgorithmRunInputSchema.parse({ configurationSnapshotId: 31, assetId: 9, parameters });
  assert.deepEqual(input.parameters, { language: "zh-CN", confidence: 0.8, deskew: true });
  assert.equal(input.configurationSnapshotId, 31);
});

test("legacy definition version input maps to an internal configuration snapshot", () => {
  const input = startAlgorithmRunInputSchema.parse({ definitionVersionId: 31, assetId: 9 });
  assert.deepEqual(input, { configurationSnapshotId: 31, assetId: 9, parameters: {} });
});
