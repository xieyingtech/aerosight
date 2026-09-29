import assert from "node:assert/strict";
import test from "node:test";

import { aviationComplianceSnapshot } from "./aviation-compliance.ts";
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
