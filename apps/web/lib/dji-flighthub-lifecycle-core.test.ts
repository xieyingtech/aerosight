import assert from "node:assert/strict";
import test from "node:test";

import {
  assertFlightHubConnectorEnabled,
  buildFlightHubSyncRequest,
  flightHubTokenUpdateSchema,
} from "./dji-flighthub-lifecycle-core.ts";

const TOKEN = "updated-token-must-stay-secret";

test("token update accepts only an in-memory replacement credential", () => {
  assert.deepEqual(flightHubTokenUpdateSchema.parse({ token: TOKEN }), { token: TOKEN });
  assert.throws(() => flightHubTokenUpdateSchema.parse({ token: TOKEN, projectUuid: "forged" }));
  assert.throws(() => flightHubTokenUpdateSchema.parse({ token: "" }));
});

test("sync requests are stable, read-only, and credential free", () => {
  for (const trigger of ["initial", "manual", "credential-update", "capability-probe"] as const) {
    const request = buildFlightHubSyncRequest("42", trigger);
    assert.deepEqual(request, {
      connectorInstanceId: "42",
      connectorKey: "dji.flighthub2",
      discoveryMode: "poll",
      trigger,
    });
    assert(!JSON.stringify(request).includes(TOKEN));
  }
  assert.throws(() => buildFlightHubSyncRequest("42;drop", "manual"));
});

test("disabled connectors cannot enqueue another synchronization", () => {
	for (const status of ["connecting", "connected", "degraded"]) {
		assert.doesNotThrow(() => assertFlightHubConnectorEnabled(status));
	}
	for (const status of ["disabled", "failed", "unknown", ""]) {
		assert.throws(() => assertFlightHubConnectorEnabled(status), /CONNECTOR_DISABLED/);
	}
});
