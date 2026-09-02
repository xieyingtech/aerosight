import assert from "node:assert/strict";
import test from "node:test";

import { authorizeFlightHubDiscreteCommand, validateFlightHubCommandParameters } from "./dji-flighthub-device-command-core.ts";

const now = new Date("2026-09-02T10:00:00Z");
const allowed = {
  projectId: 17, teamId: 11, deviceId: 42, capabilityCode: "flight.return_home", commandKey: "return_home",
  parametersValid: true, deviceTypeKey: "dji.matrice4td", deviceOnline: true,
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
    { parametersValid: false }, { deviceOnline: false },
    { stateCapturedAt: new Date("2026-09-02T09:59:00Z") }, { safetyPolicyVersionId: 7 },
    { stateCapturedAt: new Date("2026-09-02T10:00:02Z") },
    { approvalStatus: "pending" }, { approvalProjectId: 99 }, { approvalAction: "flighthub.device.future" },
    { approvalUnexpired: false }
  ];
  for (const override of denied) {
    assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, ...override }));
  }
});

test("camera and lens commands require exact device model, parameters, capability and approval", () => {
  const camera = { ...allowed, commandKey: "camera.change", capabilityCode: "camera.change", deviceTypeKey: "dji.dock3",
    approvalAction: "flighthub.device.camera.change" };
  assert.equal(authorizeFlightHubDiscreteCommand(camera).policy.connectorCapabilityCode, "device.camera.change");
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...camera, deviceTypeKey: "dji.matrice4td" }), /MODEL_UNSUPPORTED/);
  const lens = { ...allowed, commandKey: "camera.change_lens", capabilityCode: "camera.lens.change",
    approvalAction: "flighthub.device.camera.change_lens" };
  assert.equal(authorizeFlightHubDiscreteCommand(lens).policy.connectorCapabilityCode, "device.lens.change");
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...lens, approvalAction: "flighthub.device.camera.change" }), /APPROVAL_REQUIRED/);
  assert.equal(validateFlightHubCommandParameters("camera.change", { cameraIndex: "cam:0", cameraPosition: "outdoor" }), true);
  assert.equal(validateFlightHubCommandParameters("camera.change_lens", { cameraIndex: "cam:0", lensType: "wide" }), true);
  assert.equal(validateFlightHubCommandParameters("camera.change_lens", { cameraIndex: "cam:0", lensType: "wide", sn: "spoofed" }), false);
});

test("FlightHub command key cannot borrow an adjacent capability or approval", () => {
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, commandKey: "flighttask_pause" }), /POLICY_MISMATCH/);
  assert.throws(() => authorizeFlightHubDiscreteCommand({ ...allowed, commandKey: "future_control" }), /POLICY_MISMATCH/);
  const pause = { ...allowed, commandKey: "flighttask_pause", capabilityCode: "mission.execute",
    approvalAction: "flighthub.device.flighttask_pause" };
  assert.equal(authorizeFlightHubDiscreteCommand(pause).policy.capabilityCode, "mission.execute");
});
