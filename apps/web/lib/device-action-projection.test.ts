import assert from "node:assert/strict";
import test from "node:test";

import { projectDeviceCapabilities } from "./device-action-projection.ts";

const capability = { code: "flight.return_home", availability: "available", reason: null, risk: "high" as const };

test("owners and admins receive catalog actions without explicit grants", () => {
  for (const role of ["owner", "admin"] as const) {
    const [projected] = projectDeviceCapabilities({
      deviceId: 3, deviceTypeId: "9", deviceStatus: "online", role, capabilities: [capability], grants: []
    });
    assert.equal(projected?.authorized, true);
    assert.equal(projected?.actions[0]?.key, "return_home");
    assert.equal(projected?.actions[0]?.enabled, true);
  }
});

test("members receive only explicitly granted actions and deny takes precedence", () => {
  const allowed = projectDeviceCapabilities({
    deviceId: 3, deviceTypeId: "9", deviceStatus: "online", role: "member", capabilities: [capability],
    grants: [{ scopeType: "device", deviceTypeId: null, deviceId: 3, actionPattern: "flight.*", effect: "allow" }]
  });
  assert.equal(allowed[0]?.actions.length, 1);
  const denied = projectDeviceCapabilities({
    deviceId: 3, deviceTypeId: "9", deviceStatus: "online", role: "member", capabilities: [capability],
    grants: [
      { scopeType: "project", deviceTypeId: null, deviceId: null, actionPattern: "flight.*", effect: "allow" },
      { scopeType: "device", deviceTypeId: null, deviceId: 3, actionPattern: "flight.return_home", effect: "deny" }
    ]
  });
  assert.equal(denied[0]?.authorized, false);
  assert.deepEqual(denied[0]?.actions, []);
});

test("authorized actions are disabled when device or capability is unavailable", () => {
  const [offline] = projectDeviceCapabilities({
    deviceId: 3, deviceTypeId: "9", deviceStatus: "offline", role: "admin", capabilities: [capability], grants: []
  });
  assert.equal(offline?.actions[0]?.enabled, false);
  assert.match(offline?.actions[0]?.unavailableReason ?? "", /不在线/);
});
