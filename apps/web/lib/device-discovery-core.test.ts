import assert from "node:assert/strict";
import test from "node:test";

import { canConfirmDiscovery, filterDiscoveries, type DeviceDiscovery } from "./device-discovery-core.ts";

const rows: DeviceDiscovery[] = [
  { id: "1", connectorId: "10", connectorName: "司空 A", connectorKey: "dji.flighthub2", externalDeviceId: "dock-a", externalDeviceType: "dji.dock2", parentExternalId: null, status: "discovered", suggestedTypeKey: "dji.dock2", suggestedTypeName: "DJI Dock 2", matchConfidence: 1, deviceId: null, lastSeenAt: "2026-09-01T00:00:00Z" },
  { id: "2", connectorId: "11", connectorName: "司空 B", connectorKey: "dji.flighthub2", externalDeviceId: "aircraft-b", externalDeviceType: "dji.matrice4td", parentExternalId: "dock-b", status: "conflicted", suggestedTypeKey: "dji.matrice4td", suggestedTypeName: "DJI Matrice 4TD", matchConfidence: 1, deviceId: null, lastSeenAt: "2026-09-01T00:00:00Z" },
];

test("discovery filters keep connector and ancestor source context", () => {
  assert.deepEqual(filterDiscoveries(rows, { connectorId: "11", status: "conflicted" }).map((item) => item.parentExternalId), ["dock-b"]);
  assert.deepEqual(filterDiscoveries(rows, { query: "dock 2" }).map((item) => item.id), ["1"]);
});

test("only pending discoveries can be confirmed", () => {
  assert.equal(canConfirmDiscovery("discovered"), true);
  for (const status of ["managed", "conflicted", "ignored", "missing"] as const) assert.equal(canConfirmDiscovery(status), false);
});
