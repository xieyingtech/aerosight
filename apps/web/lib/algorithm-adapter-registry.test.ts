import assert from "node:assert/strict";
import test from "node:test";
import { algorithmAdapterCapability, listAlgorithmAdapterCapabilities, requireEnabledAlgorithmAdapter } from "./algorithm-adapter-registry.ts";

test("every declared provider protocol has explicit capabilities and implementation status", () => {
  const capabilities = listAlgorithmAdapterCapabilities();
  assert.deepEqual(capabilities.map((item) => item.providerType).sort(), ["ai-sdk", "http-json", "kserve-v2", "ogc-processes"]);
  for (const capability of capabilities) {
    assert.equal(capability.contractVersion, "aerosight.algorithm.input/v1");
    assert.ok(["enabled", "unavailable"].includes(capability.implementationStatus));
  }
});

test("enabled http-json adapter declares only protocol behavior covered by contract tests", () => {
  const capability = requireEnabledAlgorithmAdapter("http-json");
  assert.deepEqual(capability.executionModes, ["synchronous", "asynchronous"]);
  assert.equal(capability.supportsPolling, false);
  assert.equal(capability.supportsSignedCallbacks, false);
});

test("unimplemented adapters fail explicitly and cannot report a successful probe", () => {
  for (const providerType of ["kserve-v2", "ogc-processes", "ai-sdk"] as const) {
    const capability = algorithmAdapterCapability(providerType);
    assert.equal(capability.implementationStatus, "unavailable");
    assert.ok(capability.unavailableReason);
    assert.throws(() => requireEnabledAlgorithmAdapter(providerType), new RegExp(`ALGORITHM_ADAPTER_UNAVAILABLE:${providerType}`));
  }
});
