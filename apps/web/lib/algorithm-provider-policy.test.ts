import assert from "node:assert/strict";
import test from "node:test";

import { algorithmProviderInputSchema, publicAlgorithmProvider } from "./algorithm-provider-policy.ts";
import { effectiveProjectPermissions } from "./project-permission-policy.ts";

const input = {
  name: "违建识别服务", providerType: "http-json" as const, baseUrl: "https://algorithm.example.test/v1",
  secretRef: "secret://projects/17/algorithms/provider-1", authType: "bearer" as const,
  allowedHeaders: ["X-Request-Source"], timeoutSeconds: 30, concurrencyLimit: 4, rateLimitPerMinute: 120
};

test("owner and admin manage providers while ordinary member cannot", () => {
  assert.equal(effectiveProjectPermissions("owner").has("algorithm:manage"), true);
  assert.equal(effectiveProjectPermissions("admin").has("algorithm:manage"), true);
  assert.equal(effectiveProjectPermissions("member").has("algorithm:manage"), false);
  assert.equal(effectiveProjectPermissions("member", ["algorithm:manage"]).has("algorithm:manage"), true);
});

test("provider input accepts secret references but rejects inline secret fields", () => {
  assert.equal(algorithmProviderInputSchema.parse(input).providerType, "http-json");
  assert.equal(algorithmProviderInputSchema.safeParse({ ...input, apiKey: "plaintext" }).success, false);
  assert.equal(algorithmProviderInputSchema.safeParse({ ...input, secretRef: "plaintext-key" }).success, false);
});

test("public provider view never returns its secret reference", () => {
  const view = publicAlgorithmProvider({ id: 1, ...input });
  assert.equal(view.secretConfigured, true);
  assert.equal("secretRef" in view, false);
  assert.equal(JSON.stringify(view).includes("provider-1"), false);
});
