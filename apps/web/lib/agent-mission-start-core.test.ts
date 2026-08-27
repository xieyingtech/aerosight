import assert from "node:assert/strict";
import test from "node:test";
import { createAgentExecutionContext } from "./agent-execution-context-core.ts";
import { authorizeAgentMissionStart, type AgentMissionStartAuthorization } from "./agent-mission-start-core.ts";

const context = createAgentExecutionContext({ userId: 7, teamId: 11, projectId: 17, sessionId: 23 });
const input = { taskVersionId: 31, approvalRequestId: "158065e2-e28b-4de7-851b-f80dec2a31dd", idempotencyKey: "fc7b7baa-d8c7-4c26-9e31-4789d1b5e04b" };
const allowed: AgentMissionStartAuthorization = {
  hasPermission: true,
  taskProjectId: 17,
  taskVersionStatus: "published",
  approvalStatus: "approved",
  approvalProjectId: 17,
  approvalResourceType: "task_version",
  approvalResourceId: "31",
  approvalAction: "mission.start",
  preflightAllowed: true,
  deviceCommandsEnabled: true,
  selectedDeviceId: 41,
  safetyPolicyVersionId: 51
};

test("protected mission start fails without current permission", () => {
  assert.throws(() => authorizeAgentMissionStart(context, input, { ...allowed, hasPermission: false }), /AGENT_MISSION_PERMISSION_DENIED/);
});

test("protected mission start fails without approved scoped preflight", () => {
  assert.throws(() => authorizeAgentMissionStart(context, input, { ...allowed, approvalStatus: "pending" }), /AGENT_MISSION_APPROVAL_REQUIRED/);
  assert.throws(() => authorizeAgentMissionStart(context, input, { ...allowed, preflightAllowed: false }), /AGENT_MISSION_PREFLIGHT_FAILED/);
  assert.throws(() => authorizeAgentMissionStart(context, input, { ...allowed, approvalProjectId: 999 }), /AGENT_MISSION_APPROVAL_SCOPE_MISMATCH/);
});

test("legal start enters command ledger workflow and waits for device ACK", () => {
  const plan = authorizeAgentMissionStart(context, input, allowed);
  assert.equal(plan.dispatchPath, "project-outbox-command-ledger");
  assert.equal(plan.directAdapterAccess, false);
  assert.equal(plan.completion, "await-device-ack");
  assert.equal(plan.projectId, 17);
});
