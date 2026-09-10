import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { authorizeFlightHubGeospatialAction, flightHubGeospatialActionInputSchema,
  type FlightHubGeospatialActionAuthorization } from "./dji-flighthub-geospatial-actions-core.ts";

const allowed: FlightHubGeospatialActionAuthorization = {
  teamId: 7, role: "admin", hasOperatePermission: true,
  connectorProjectId: 41, connectorTeamId: 7, connectorStatus: "connected",
  actionEnabled: true, capabilityFieldVerified: true,
  targetProjectId: 41, targetConnectorId: 12, targetKind: "map-element", targetStatus: "active",
  targetRemoteVersion: "version-2"
};

const point = { type: "Feature" as const, properties: { color: "#00ff00" },
  geometry: { type: "Point" as const, coordinates: [120.5, 30.25, 15] } };
const create = flightHubGeospatialActionInputSchema.parse({ action: "map-element-create", connectorInstanceId: 12,
  idempotencyKey: "map-create-0001", request: { name: "safety point", resource: { type: 0, content: point } } });
const update = flightHubGeospatialActionInputSchema.parse({ action: "map-element-update", connectorInstanceId: 12,
  targetResourceId: 91, expectedRemoteVersion: "version-2", idempotencyKey: "map-update-0001",
  request: { name: "safety point updated", content: point } });
const deletion = flightHubGeospatialActionInputSchema.parse({ action: "map-element-delete", connectorInstanceId: 12,
  targetResourceId: 91, expectedRemoteVersion: "version-2", idempotencyKey: "map-delete-0001",
  request: { confirmation: "DELETE" } });

test("map element writes fail closed behind RBAC capability and feature gates", () => {
  for (const denied of [
    { ...allowed, hasOperatePermission: false },
    { ...allowed, actionEnabled: false },
    { ...allowed, capabilityFieldVerified: false },
    { ...allowed, connectorStatus: "disabled" },
    { ...allowed, targetConnectorId: 999 },
    { ...allowed, targetKind: "flight-area" }
  ]) assert.throws(() => authorizeFlightHubGeospatialAction(41, update, denied), /FLIGHTHUB_GEOSPATIAL_ACTION_/);
  assert.equal(authorizeFlightHubGeospatialAction(41, create, allowed).capability, "geospatial.write");
});

test("stale map element versions cannot overwrite a newer projection", () => {
  assert.throws(() => authorizeFlightHubGeospatialAction(41, update, { ...allowed, targetRemoteVersion: "version-3" }),
    /VERSION_CONFLICT/);
  const plan = authorizeFlightHubGeospatialAction(41, update, allowed);
  assert.equal("expectedRemoteVersion" in plan ? plan.expectedRemoteVersion : null, "version-2");
});

test("map element delete requires owner/admin and its dedicated feature flag", () => {
  assert.throws(() => authorizeFlightHubGeospatialAction(41, deletion, { ...allowed, role: "member" }), /PERMISSION_DENIED/);
  const plan = authorizeFlightHubGeospatialAction(41, deletion, allowed);
  assert.equal(plan.capability, "geospatial.element.delete");
  assert.equal(plan.featureFlag, "flighthub.geospatial.delete");
});

test("geospatial action API audits intent without exposing encrypted request payloads", async () => {
const handler=await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/flighthub_actions.go", import.meta.url), "utf8");const query=await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/fh_geospatial_actions.sql", import.meta.url), "utf8");const projection=query.slice(query.indexOf("-- name: FHGeospatialRead"));assert.match(handler,/database.AuditedWrite/);assert.match(handler,/credentials.EncryptJSON/);assert.doesNotMatch(projection,/request_envelope|requestDigest|remote_id/);assert.match(projection,/connector_instance_id/);
});

test("geospatial action schema requires explicit update fields and exact delete confirmation", () => {
  assert.throws(() => flightHubGeospatialActionInputSchema.parse({ ...update, request: {} }));
  assert.throws(() => flightHubGeospatialActionInputSchema.parse({ ...deletion, request: { confirmation: "yes" } }));
});
