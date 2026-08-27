import assert from "node:assert/strict";
import test from "node:test";

import { buildMissionAuditTrace, planEmergencyStopDrill } from "./mission-audit-trace-core.ts";

for (const actorType of ["user", "agent"] as const) {
  test(`${actorType} request correlates preflight, approval, command, and ACK`, () => {
    const trace = buildMissionAuditTrace({
      projectId: 17,
      taskRunId: 42,
      triggerSource: actorType,
      request: { requestId: `request-${actorType}`, action: "mission.start", actorType, actorId: 9, createdAt: "2026-08-27T08:00:00Z" },
      preflight: { policyVersionId: 4, allowed: true, checks: [] },
      approval: { id: "approval-1", status: "approved", requiredApprovals: 1, receivedApprovals: 1 },
      commands: [{ id: "command-1", action: "flight.route", capabilityCode: "flight.route", status: "acknowledged", priority: 10, attempt: 1, attemptStatus: "acknowledged" }]
    });
    assert.equal(trace.complete, true);
    assert.deepEqual(trace.correlation, { requestId: `request-${actorType}`, approvalRequestId: "approval-1", commandIds: ["command-1"] });
  });
}

test("emergency stop ACK drill confirms stop and retains priority semantics", () => {
  const trace = planEmergencyStopDrill({
    projectId: 17, taskRunId: 42, requestId: "drill-ack", actorUserId: 9,
    deviceConnected: true, capabilityDeclared: true, outcome: "ack"
  });
  assert.equal(trace.safetyState, "confirmed");
  assert.equal(trace.stages.commands[0].priority, 100);
  assert.equal(trace.stages.commands[0].action, "safety.emergency_stop");
});

for (const outcome of ["nack", "timeout", "disconnected"] as const) {
  test(`emergency stop ${outcome} drill never reports a false safe state`, () => {
    const trace = planEmergencyStopDrill({
      projectId: 17, taskRunId: 42, requestId: `drill-${outcome}`, actorUserId: 9,
      deviceConnected: outcome !== "disconnected", capabilityDeclared: true, outcome
    });
    assert.notEqual(trace.safetyState, "confirmed");
    assert.ok(["rejected", "unknown"].includes(trace.safetyState));
  });
}
