import assert from "node:assert/strict";
import test from "node:test";

import { dependencyHealthFromRecord, evaluateProjectHealth, type DependencyName } from "./dependency-health-core.ts";

for (const [dependency, reason, capability] of [
  ["object_storage", "OBJECT_STORAGE_UNAVAILABLE", "media_ingestion"],
  ["algorithm_service", "ALGORITHM_SERVICE_UNAVAILABLE", "algorithm_execution"],
  ["model_service", "MODEL_SERVICE_UNAVAILABLE", "ai_generation"],
  ["device_adapter", "DEVICE_ADAPTER_UNAVAILABLE", "realtime_device_control"]
] as Array<[DependencyName, string, string]>) {
  test(`${dependency} outage degrades new work while preserving historical access`, () => {
    const health = evaluateProjectHealth(dependencyHealthFromRecord({ [dependency]: "unavailable" }));
    assert.equal(health.status, "degraded");
    assert.equal(health.ready, true);
    assert.equal(health.historicalDataAvailable, true);
    assert.equal(health.capabilityAvailability.historical_queries, "available");
    assert.equal(health.capabilityAvailability[capability], "degraded");
    assert.deepEqual(health.degradationReasons, [reason]);
  });
}

test("database outage fails readiness instead of presenting stale history as available", () => {
  const health = evaluateProjectHealth(dependencyHealthFromRecord({ database: "unavailable" }));
  assert.equal(health.status, "unavailable");
  assert.equal(health.ready, false);
  assert.equal(health.historicalDataAvailable, false);
});

test("disabled optional service is explicit but is not reported as an outage", () => {
  const health = evaluateProjectHealth(dependencyHealthFromRecord({ model_service: "disabled" }));
  assert.equal(health.status, "healthy");
  assert.equal(health.capabilityAvailability.ai_generation, "disabled");
  assert.deepEqual(health.degradationReasons, []);
});
