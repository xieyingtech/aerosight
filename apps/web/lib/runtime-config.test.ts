import assert from "node:assert/strict";
import test from "node:test";

import { correlationId, redactSensitive } from "./observability.ts";
import { parseWebRuntimeConfig } from "./runtime-config.ts";

test("runtime configuration reports all required values", () => {
  assert.throws(
    () => parseWebRuntimeConfig({}),
    /DATABASE_URL is required; AUTH_SECRET must contain at least 16 characters/
  );
});

test("runtime configuration accepts valid values and defaults log level", () => {
  const config = parseWebRuntimeConfig({
    DATABASE_URL: "postgresql://database.example/aerosight",
    AUTH_SECRET: "0123456789abcdef"
  });
  assert.equal(config.logLevel, "info");
  assert.equal(config.aiProvider, "disabled");
});

test("AI deployment configuration fails closed and accepts a complete provider",()=>{
  const base={DATABASE_URL:"postgresql://database.example/aerosight",AUTH_SECRET:"0123456789abcdef"};
  assert.throws(()=>parseWebRuntimeConfig({...base,AI_PROVIDER:"openai"}),/AI_MODEL is required.*OPENAI_API_KEY is required/);
  const config=parseWebRuntimeConfig({...base,AI_PROVIDER:"openai",AI_MODEL:"gpt-5-mini",OPENAI_API_KEY:"secret-ref-resolved-at-deploy"});
  assert.equal(config.aiProvider,"openai");assert.equal(config.aiModel,"gpt-5-mini");
});

test("correlation IDs accept safe upstream values and replace unsafe values", () => {
  assert.equal(correlationId("request-123"), "request-123");
  assert.notEqual(correlationId("contains spaces"), "contains spaces");
});

test("redaction removes nested secrets and credentials embedded in errors", () => {
  const redacted = redactSensitive({
    authorization: "Bearer top-secret",
    nested: { apiKey: "key-value", url: "https://example.test/run?token=token-value" },
    error: new Error("request failed with Bearer leaked-token")
  });
  const serialized = JSON.stringify(redacted);
  assert(!serialized.includes("top-secret"));
  assert(!serialized.includes("key-value"));
  assert(!serialized.includes("token-value"));
  assert(!serialized.includes("leaked-token"));
});
