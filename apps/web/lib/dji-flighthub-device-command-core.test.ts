import assert from "node:assert/strict";
import test from "node:test";

import { authorizeFlightHubDiscreteCommand } from "./dji-flighthub-device-command-core.ts";

const now = new Date("2026-09-02T10:00:00Z");
const allowed = {
  projectId: 17, teamId: 11, deviceId: 42, capabilityCode: "flight.return_home", commandKey: "return_home",
  parametersEmpty: true,
  connectorStatus: "connected", featureEnabled: true, capabilityFieldVerified: true,
  stateCapturedAt: new Date("2026-09-02T09:59:50Z"), now,
  safetyPolicyVersionId: 8, currentSafetyPolicyVersionId: 8,
  approvalProjectId: 17, approvalTeamId: 11, approvalResourceType: "device", approvalResourceId: "42",
  approvalAction: "flighthub.device.return_home", approvalStatus: "approved", approvalUnexpired: true
};

test("FlightHub discrete command requires every runtime safety gate", () => {
  assert.equal(authorizeFlightHubDiscreteCommand(allowed).approvalValid, true);
  const denied = [
    { featureEnabled: false }, { capabilityFieldVerified: false }, { connectorStatus: "disabled" },
    { parametersEmpty: false },
    { stateCapturedAt: new Date("2026-09-02T09:59:00Z") }, { safetyPolicyVersionId: 7 },
    { stateCapturedAt: new Date("2026-09-02T10:00:02Z") },
    { approvalStatus: "pending" }, { approvalProjectId: 99 }, { approvalAction: "flighthub.device.future" },
    { approvalUnexpired: false }
  ];
  for (const override of denied) {
    assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, ...override }));
  }
});

test("FlightHub command key cannot borrow an adjacent capability or approval", () => {
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, commandKey: "flighttask_pause" }), /POLICY_MISMATCH/);
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, commandKey: "future_control" }), /POLICY_MISMATCH/);
  const pause = { ...allowed, commandKey: "flighttask_pause", capabilityCode: "mission.execute",
    approvalAction: "flighthub.device.flighttask_pause" };
  assert.equal(authorizeFlightHubDiscreteCommand(pause).policy.capabilityCode, "mission.execute");
});
