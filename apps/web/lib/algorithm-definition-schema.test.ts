import assert from "node:assert/strict";
import test from "node:test";

import { algorithmDefinitionInputSchema, algorithmDefinitionVersionInputSchema } from "./algorithm-definition-schema.ts";

test("algorithm definitions accept generic OCR without a fixed business category", () => {
  const definition = algorithmDefinitionInputSchema.parse({ providerId: 4, name: "票据 OCR", capabilityCode: "perception.ocr" });
  const version = algorithmDefinitionVersionInputSchema.parse({
    executionMode: "synchronous", modelOrProcess: "ocr-v2",
    inputSchema: { type: "object", required: ["asset"] },
    parametersSchema: { type: "object", properties: { language: { type: "string" } } },
    outputSchema: { type: "object", properties: { text: { type: "string" } } },
    protocolConfig: {}, outputMapping: { kind: "ocr", text: "$.text" }
  });
  assert.equal(definition.capabilityCode, "perception.ocr");
  assert.equal(version.outputMapping.kind, "ocr");
});

test("algorithm version schema rejects invalid schema and publish threshold", () => {
  const input = {
    executionMode: "synchronous", modelOrProcess: "model",
    inputSchema: { type: 7 }, parametersSchema: {}, outputSchema: {}, protocolConfig: {}, outputMapping: {},
    publishThreshold: 2
  };
  assert.equal(algorithmDefinitionVersionInputSchema.safeParse(input).success, false);
});
