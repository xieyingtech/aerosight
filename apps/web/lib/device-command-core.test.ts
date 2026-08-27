import assert from "node:assert/strict";
import test from "node:test";

import { actionPatternMatches, assertDeviceCommandSafety, authorizeCapabilityAction, confirmationPhrase } from "./device-command-core.ts";

const base = {
  requestProjectId: 17, deviceProjectId: 17, deviceId: 42,
  capabilityCode: "dock.debug.control", riskLevel: "critical" as const,
  capabilityAvailability: "available" as const, deviceStatus: "online" as const,
  activeTaskCount: 0, confirmation: confirmationPhrase(42, "dock.debug.control")
};

test("high risk command requires exact second confirmation", () => {
  assert.throws(() => assertDeviceCommandSafety({ ...base, confirmation: null }), /CONFIRMATION_REQUIRED/);
  assert.throws(() => assertDeviceCommandSafety({ ...base, confirmation: "CONFIRM 7 dock.debug.control" }), /CONFIRMATION_REQUIRED/);
  assert.equal(assertDeviceCommandSafety(base).confirmationRequired, true);
});

test("cross-project, unavailable, offline and active-task commands fail closed", () => {
  assert.throws(() => assertDeviceCommandSafety({ ...base, deviceProjectId: 18 }), /SCOPE_DENIED/);
  assert.throws(() => assertDeviceCommandSafety({ ...base, capabilityAvailability: "unavailable" }), /CAPABILITY_UNAVAILABLE/);
  assert.throws(() => assertDeviceCommandSafety({ ...base, deviceStatus: "degraded" }), /NOT_ONLINE/);
  assert.throws(() => assertDeviceCommandSafety({ ...base, activeTaskCount: 1 }), /ACTIVE_TASK_CONFLICT/);
});

test("return-home may override an active task but still requires confirmation", () => {
  const input = { ...base, capabilityCode: "flight.return_home", activeTaskCount: 1,
    confirmation: confirmationPhrase(42, "flight.return_home") };
  assert.equal(assertDeviceCommandSafety(input).activeTaskOverride, true);
});

test("capability grants support scoped wildcards and explicit deny wins", () => {
  assert.equal(actionPatternMatches("dock.*", "dock.debug.control"), true);
  assert.equal(authorizeCapabilityAction({ role: "member", action: "dock.debug.control",
    grants: [{ actionPattern: "dock.*", effect: "allow" }] }), true);
  assert.throws(() => authorizeCapabilityAction({ role: "admin", action: "dock.debug.control",
    grants: [{ actionPattern: "dock.*", effect: "deny" }] }), /EXPLICITLY_DENIED/);
  assert.throws(() => authorizeCapabilityAction({ role: "member", action: "flight.return_home", grants: [] }), /NOT_GRANTED/);
});
