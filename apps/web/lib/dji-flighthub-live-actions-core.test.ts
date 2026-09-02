import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { authorizeFlightHubLiveAction, flightHubLiveActionInputSchema,
  type FlightHubLiveActionAuthorization } from "./dji-flighthub-live-actions-core.ts";

const allowed: FlightHubLiveActionAuthorization = {
  teamId: 7, role: "admin", hasOperatePermission: true,
  connectorProjectId: 41, connectorTeamId: 7, connectorStatus: "connected",
  actionEnabled: true, capabilityFieldVerified: true,
  deviceProjectId: 41, deviceConnectorIdentityPresent: true,
  targetProjectId: 41, targetConnectorId: 12, targetKind: "stream-converter", targetStatus: "active"
};

const quality = flightHubLiveActionInputSchema.parse({ action: "live-quality-set", connectorInstanceId: 12,
  deviceId: 55, idempotencyKey: "quality-0001", request: { cameraIndex: "0", qualityType: "adaptive" } });
const toggle = flightHubLiveActionInputSchema.parse({ action: "live-converter-toggle", connectorInstanceId: 12,
  targetResourceId: 91, idempotencyKey: "toggle-0001", request: { enabled: true } });
const create = flightHubLiveActionInputSchema.parse({ action: "live-converter-create", connectorInstanceId: 12,
  deviceId: 55, idempotencyKey: "create-0001", request: { name: "relay", cameraIndex: "0", schema: "rtmp",
    schemaOption: { url: "rtmp://media.invalid/live" } } });
const deletion = flightHubLiveActionInputSchema.parse({ action: "live-converter-delete", connectorInstanceId: 12,
  targetResourceId: 91, idempotencyKey: "delete-0001", request: { confirmation: "DELETE" } });

test("live action authorization fails closed before a worker job can reach upstream", () => {
  for (const denied of [
    { ...allowed, hasOperatePermission: false },
    { ...allowed, actionEnabled: false },
    { ...allowed, capabilityFieldVerified: false },
    { ...allowed, connectorStatus: "disabled" },
    { ...allowed, deviceConnectorIdentityPresent: false }
  ]) assert.throws(() => authorizeFlightHubLiveAction(41, quality, denied), /FLIGHTHUB_LIVE_ACTION_/);

  assert.throws(() => authorizeFlightHubLiveAction(41, toggle, { ...allowed, targetConnectorId: 999 }),
    /TARGET_SCOPE_MISMATCH/);
});

test("converter delete requires owner or admin RBAC in addition to its dedicated gates", () => {
  assert.throws(() => authorizeFlightHubLiveAction(41, deletion, { ...allowed, role: "member" }), /PERMISSION_DENIED/);
  const plan = authorizeFlightHubLiveAction(41, deletion, allowed);
  assert.equal(plan.capability, "live.converter.delete");
  assert.equal(plan.featureFlag, "flighthub.live.converter.delete");
});

test("each executable action resolves to an independent capability and feature flag", () => {
  const plans = [quality, create, toggle, deletion].map((input) => authorizeFlightHubLiveAction(41, input, allowed));
  assert.equal(new Set(plans.map((plan) => plan.capability)).size, plans.length);
  assert.equal(new Set(plans.map((plan) => plan.featureFlag)).size, plans.length);
});

test("live action API and public job projection never expose encrypted requests", () => {
  const route = readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/live-actions/route.ts", import.meta.url), "utf8");
  const service = readFileSync(new URL("./dji-flighthub-live-actions.ts", import.meta.url), "utf8");
  const publicProjection = service.slice(service.indexOf("export async function readFlightHubLiveActionJob"));
  assert.doesNotMatch(route, /request_envelope|credential_envelope|schemaOption/i);
  assert.doesNotMatch(publicProjection, /request_envelope|requestDigest/i);
});
