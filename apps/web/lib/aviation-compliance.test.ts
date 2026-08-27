import assert from "node:assert/strict";
import test from "node:test";

import { aviationComplianceSnapshot } from "./aviation-compliance.ts";
import { evaluateMissionPreflight, type SafetyPolicy } from "./mission-preflight.ts";

const boundary = [[120, 30], [122, 30], [122, 32], [120, 32], [120, 30]] as Array<[number, number]>;
const requiredPolicy: SafetyPolicy = {
  policyVersionId: "aviation-policy-v4",
  projectBoundary: boundary,
  restrictedAreas: [],
  maxAltitudeMeters: 120,
  maxSpeedMetersPerSecond: 15,
  minimumBatteryPercent: 30,
  requiredCompliance: [
    "realNameRegistration", "remoteIdentification", "operationApproval",
    "takeoffConfirmation", "responsibleOperator"
  ],
  optionalCompliance: ["incidentReport"]
};

const mission = (compliance: ReturnType<typeof aviationComplianceSnapshot>) => ({
  route: [[120.2, 30.2, 80], [120.4, 30.4, 90]] as Array<[number, number, number]>,
  plannedSpeedMetersPerSecond: 10,
  batteryPercent: 80,
  plannedStartAt: new Date("2026-08-27T08:00:00Z"),
  compliance
});

test("versioned aviation policy requires device and run compliance fields", () => {
  const compliance = aviationComplianceSnapshot(
    { registrationNumber: "UAS-CN-SANDBOX-001", remoteIdentificationCode: "RID-SANDBOX-001", responsibleUserId: 7 },
    { operationApprovalReference: "APPROVAL-SANDBOX-001" }
  );
  const result = evaluateMissionPreflight(requiredPolicy, mission(compliance));
  assert.equal(result.policyVersionId, "aviation-policy-v4");
  assert.equal(result.allowed, false);
  assert.ok(result.checks.some((item) => item.code === "COMPLIANCE_takeoffConfirmation" && item.severity === "hard_failure"));
});

test("published policy exemption remains visible and does not invent confirmation", () => {
  const compliance = aviationComplianceSnapshot(
    { registrationNumber: "UAS-CN-SANDBOX-001", remoteIdentificationCode: "RID-SANDBOX-001", responsibleUserId: 7 },
    { operationApprovalReference: "APPROVAL-SANDBOX-001" }
  );
  const result = evaluateMissionPreflight({
    ...requiredPolicy,
    policyVersionId: "aviation-policy-v5",
    exemptions: [{ field: "takeoffConfirmation", reason: "室内封闭试验", validUntil: new Date("2026-08-28T00:00:00Z") }]
  }, mission(compliance));
  assert.equal(result.allowed, true);
  assert.equal(compliance.takeoffConfirmation, undefined);
  assert.ok(result.checks.some((item) => item.code === "COMPLIANCE_takeoffConfirmation" && item.severity === "warning"));
});

test("complete confirmation, approval expiry, responsibility, and incident fields are deterministic", () => {
  const confirmedAt = new Date("2026-08-27T07:59:00Z");
  const snapshot = aviationComplianceSnapshot(
    { registrationNumber: " UAS-CN-SANDBOX-001 ", registrationValidUntil: new Date("2027-01-01T00:00:00Z"), remoteIdentificationCode: "RID-SANDBOX-001" },
    {
      operationApprovalReference: "APPROVAL-SANDBOX-001",
      operationApprovalValidUntil: new Date("2026-08-27T12:00:00Z"),
      takeoffConfirmedAt: confirmedAt,
      takeoffConfirmedByUserId: 8,
      responsibleUserId: 9,
      incidentReportReference: "INCIDENT-SANDBOX-001",
      incidentReportedAt: new Date("2026-08-27T09:00:00Z")
    }
  );
  assert.equal(snapshot.realNameRegistration?.reference, "UAS-CN-SANDBOX-001");
  assert.equal(snapshot.takeoffConfirmation?.reference, "user:8@2026-08-27T07:59:00.000Z");
  assert.equal(snapshot.responsibleOperator?.reference, "user:9");
  assert.equal(snapshot.incidentReport?.reference, "INCIDENT-SANDBOX-001@2026-08-27T09:00:00.000Z");
});
