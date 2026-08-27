import assert from "node:assert/strict";
import test from "node:test";

import { evaluateMissionPreflight, type MissionPreflightInput, type SafetyPolicy } from "./mission-preflight.ts";

const square = (west: number, south: number, east: number, north: number) =>
  [[west, south], [east, south], [east, north], [west, north], [west, south]] as Array<[number, number]>;

const policy: SafetyPolicy = {
  policyVersionId: "policy-v3",
  projectBoundary: square(120, 30, 122, 32),
  restrictedAreas: [square(120.8, 30.8, 121.2, 31.2)],
  maxAltitudeMeters: 120,
  maxSpeedMetersPerSecond: 15,
  minimumBatteryPercent: 30,
  allowedWindows: [{ weekdays: [1], startMinute: 8 * 60, endMinute: 18 * 60 }],
  requiredCompliance: ["flightApproval", "remoteIdentification"],
  optionalCompliance: ["insurance"]
};

const input: MissionPreflightInput = {
  route: [[120.2, 30.2, 80], [120.4, 30.4, 90]],
  plannedSpeedMetersPerSecond: 10,
  batteryPercent: 80,
  plannedStartAt: new Date("2026-08-24T10:00:00Z"),
  compliance: {
    flightApproval: { reference: "approval-1", validUntil: new Date("2026-08-25T00:00:00Z") },
    remoteIdentification: { reference: "rid-1" }
  }
};

test("valid mission passes with an optional compliance warning", () => {
  const result = evaluateMissionPreflight(policy, input);
  assert.equal(result.allowed, true);
  assert.equal(result.policyVersionId, "policy-v3");
  assert.ok(result.checks.some((item) => item.code === "COMPLIANCE_insurance" && item.severity === "warning"));
});

for (const [name, mutate, code] of [
  ["project boundary", (draft: MissionPreflightInput) => { draft.route[1] = [123, 31, 80]; }, "PROJECT_BOUNDARY"],
  ["restricted area", (draft: MissionPreflightInput) => { draft.route = [[120.7, 31, 80], [121.3, 31, 80]]; }, "RESTRICTED_AREA"],
  ["altitude", (draft: MissionPreflightInput) => { draft.route[0][2] = 121; }, "MAX_ALTITUDE"],
  ["speed", (draft: MissionPreflightInput) => { draft.plannedSpeedMetersPerSecond = 16; }, "MAX_SPEED"],
  ["battery", (draft: MissionPreflightInput) => { draft.batteryPercent = 29; }, "BATTERY"],
  ["time window", (draft: MissionPreflightInput) => { draft.plannedStartAt = new Date("2026-08-24T20:00:00Z"); }, "TIME_WINDOW"],
  ["compliance", (draft: MissionPreflightInput) => { delete draft.compliance.flightApproval; }, "COMPLIANCE_flightApproval"]
] as Array<[string, (draft: MissionPreflightInput) => void, string]>) {
  test(`${name} violation is a hard failure`, () => {
    const draft = structuredClone(input);
    mutate(draft);
    const result = evaluateMissionPreflight(policy, draft);
    assert.equal(result.allowed, false);
    assert.ok(result.checks.some((item) => item.code === code && item.severity === "hard_failure"));
  });
}

test("near-minimum battery and a valid exemption remain visible warnings", () => {
  const result = evaluateMissionPreflight({
    ...policy,
    exemptions: [{ field: "flightApproval", reason: "室内封闭场景", validUntil: new Date("2026-08-25T00:00:00Z") }]
  }, { ...structuredClone(input), batteryPercent: 35, compliance: { remoteIdentification: { reference: "rid-1" } } });
  assert.equal(result.allowed, true);
  assert.ok(result.checks.some((item) => item.code === "BATTERY" && item.severity === "warning"));
  assert.ok(result.checks.some((item) => item.code === "COMPLIANCE_flightApproval" && item.severity === "warning"));
});
