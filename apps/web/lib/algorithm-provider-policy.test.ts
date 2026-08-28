import assert from "node:assert/strict";
import test from "node:test";

import { algorithmCredentialPayload, algorithmProviderInputSchema } from "./algorithm-provider-policy.ts";
import { effectiveProjectPermissions } from "./project-permission-policy.ts";

const input = {
  name: "违建识别服务", providerType: "http-json" as const, baseUrl: "https://algorithm.example.test/v1",
  credential: "plaintext-token", authType: "bearer" as const,
  allowedHeaders: ["X-Request-Source"], timeoutSeconds: 30, concurrencyLimit: 4, rateLimitPerMinute: 120
};

test("owner and admin manage providers while ordinary member cannot", () => {
  assert.equal(effectiveProjectPermissions("owner").has("algorithm:manage"), true);
  assert.equal(effectiveProjectPermissions("admin").has("algorithm:manage"), true);
  assert.equal(effectiveProjectPermissions("member").has("algorithm:manage"), false);
  assert.equal(effectiveProjectPermissions("member", ["algorithm:manage"]).has("algorithm:manage"), true);
});

test("provider input accepts a write-only credential", () => {
  assert.equal(algorithmProviderInputSchema.parse(input).providerType, "http-json");
  assert.equal(algorithmProviderInputSchema.safeParse({ ...input, apiKey: "plaintext" }).success, false);
  assert.deepEqual(algorithmCredentialPayload(input), { token: "plaintext-token" });
});

test("blank credentials preserve the stored envelope", () => {
  assert.equal(algorithmCredentialPayload({ authType: "bearer", credential: "" }), null);
  assert.equal(algorithmCredentialPayload({ authType: "bearer" }), null);
});
