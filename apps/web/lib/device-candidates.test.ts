import assert from "node:assert/strict";
import test from "node:test";

import { rankDeviceCandidates, selectMissionDevice, type CandidateDevice, type DeviceRequirement } from "./device-candidates.ts";

const requirement: DeviceRequirement = {
  deviceType: "drone",
  minimumBatteryPercent: 30,
  routeStart: [121.47, 31.23],
  capabilities: [
    { code: "flight.navigate", constraints: { maxSpeed: 12 } },
    { code: "camera.capture", constraints: { modes: ["photo"] } }
  ]
};

const devices: CandidateDevice[] = [
  {
    id: 1, type: "drone", connectionStatus: "online", batteryPercent: 75,
    position: [121.471, 31.231], capabilities: {
      "flight.navigate": { maxSpeed: 15 }, "camera.capture": { modes: ["photo", "video"] }
    }
  },
  {
    id: 2, type: "drone", connectionStatus: "degraded", batteryPercent: 95,
    position: [121.48, 31.24], capabilities: {
      "flight.navigate": { maxSpeed: 15 }, "camera.capture": { modes: ["photo"] }
    }
  },
  {
    id: 3, type: "drone", connectionStatus: "online", batteryPercent: 90,
    position: [121.47, 31.23], occupiedByTaskRunId: 88, capabilities: {
      "flight.navigate": { maxSpeed: 10 }, "camera.capture": { modes: ["video"] }
    }
  }
];

test("multiple candidates are ranked with explainable features", () => {
  const result = rankDeviceCandidates(devices, requirement);
  assert.deepEqual(result.map((candidate) => candidate.deviceId), [1, 2, 3]);
  assert.equal(result[0].features.connectionStatus, "online");
  assert.ok(typeof result[0].features.distanceMeters === "number");
  assert.deepEqual(result[2].exclusionReasons, [
    "DEVICE_OCCUPIED", "CAPABILITY_CONSTRAINT:flight.navigate.maxSpeed"
  ]);
});

test("no eligible device returns blocked with every exclusion reason", () => {
  const result = selectMissionDevice([
    { ...devices[0], connectionStatus: "offline" },
    { ...devices[1], batteryPercent: 20 }
  ], requirement);
  assert.equal(result.status, "blocked");
  assert.equal(result.selectedDeviceId, undefined);
  assert.ok(result.candidates.every((candidate) => candidate.exclusionReasons.length > 0));
});

test("manual override selects another eligible device and requires a new preflight", () => {
  const result = selectMissionDevice(devices, requirement, 2);
  assert.equal(result.status, "selected");
  assert.equal(result.selectedDeviceId, 2);
  assert.equal(result.overridden, true);
  assert.equal(result.requiresPreflight, true);
});

test("manual override cannot bypass occupation or capability constraints", () => {
  const result = selectMissionDevice(devices, requirement, 3);
  assert.equal(result.status, "blocked");
  assert.equal(result.selectedDeviceId, undefined);
  assert.equal(result.requiresPreflight, true);
});
