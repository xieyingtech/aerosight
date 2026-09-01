import assert from "node:assert/strict";
import test from "node:test";

import { connectorDiagnosticHealth, type FlightHubDiagnosticsPayload } from "./dji-flighthub-diagnostics-ui-core.ts";

function payload(statuses: FlightHubDiagnosticsPayload["capabilities"][number]["status"][]): FlightHubDiagnosticsPayload {
  return {
    connector: { id: "1", name: "test", status: "connected", lastErrorCode: null, lastCheckedAt: null },
    resourceWatermarks: [],
    capabilities: statuses.map((status, index) => ({
      capabilityCode: `capability.${index}`, status, evidenceLevel: "live-read", region: "cn", deployment: "cn-public-cloud",
      deviceModel: null, firmwareVersion: null, reason: null, endpointId: null, layers: {}, verifiedAt: "2026-09-01T12:00:00Z", expiresAt: null,
    })),
  };
}

test("empty and not-applicable capabilities do not degrade connector health", () => {
  assert.equal(connectorDiagnosticHealth(payload(["supported", "empty", "not_applicable"])).status, "supported");
});

test("permission, compatibility, transient, and failed states remain actionable", () => {
  for (const status of ["forbidden", "unverified", "degraded"] as const) {
    assert.equal(connectorDiagnosticHealth(payload(["supported", status])).status, "degraded");
  }
  assert.equal(connectorDiagnosticHealth(payload(["failed"])).status, "failed");
});
