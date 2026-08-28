import assert from "node:assert/strict";
import test from "node:test";

import { buildAlgorithmCatalogEntry } from "./algorithm-catalog-core.ts";

test("dynamic catalog exposes an arbitrary OCR definition entirely from server data", () => {
  const entry = buildAlgorithmCatalogEntry({
    definitionId: "12", configurationSnapshotId: "31", name: "通用票据 OCR", description: null,
    capabilityCode: "perception.ocr", providerType: "http-json", providerStatus: "active",
    executionMode: "synchronous", modelOrProcess: "ocr-v2",
    inputSchema: { type: "object", required: ["asset"] },
    parametersSchema: { type: "object", properties: { language: { type: "string" } } },
    outputSchema: { type: "object", properties: { text: { type: "string" } } },
    displayMetadata: { resultRenderer: "ocr" }
  });
  assert.equal(entry.capabilityCode, "perception.ocr");
  assert.equal(entry.display.resultRenderer, "ocr");
  assert.equal(entry.provider.available, true);
  assert.equal(entry.configurationSnapshotId, "31");
  assert.equal(JSON.stringify(entry).includes("违建"), false);
});
