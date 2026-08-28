import assert from "node:assert/strict";
import test from "node:test";

import { aiProviderInputSchema, normalizedAIAPIKey } from "./ai-provider-policy.ts";

const input = {
  name: "默认 OpenAI", providerType: "openai" as const, baseUrl: "https://api.openai.com/v1",
  modelId: "gpt-5-mini", apiKey: "provider-secret", enabled: true, isDefault: true
};

test("AI provider accepts a write-only API key and valid default state", () => {
  assert.equal(aiProviderInputSchema.parse(input).modelId, "gpt-5-mini");
  assert.equal(normalizedAIAPIKey(input), "provider-secret");
  assert.equal(aiProviderInputSchema.safeParse({ ...input, enabled: false }).success, false);
});

test("blank AI provider API key means preserve the stored envelope", () => {
  assert.equal(normalizedAIAPIKey({ apiKey: "" }), null);
  assert.equal(normalizedAIAPIKey({}), null);
});
