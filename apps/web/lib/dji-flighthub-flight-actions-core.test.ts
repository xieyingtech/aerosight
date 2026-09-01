import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { authorizeFlightHubAction, flightHubActionInputSchema, type FlightHubActionAuthorization } from "./dji-flighthub-flight-actions-core.ts";

const createInput = flightHubActionInputSchema.parse({
  connectorInstanceId: 43,
  taskRunId: 47,
  approvalRequestId: "158065e2-e28b-4de7-851b-f80dec2a31dd",
  idempotencyKey: "flight-action-0001",
  action: "flight-task-create",
  waylineResourceId: 53,
  request: { name: "巡检任务", timeZone: "Asia/Shanghai", taskType: "immediate" }
});

const allowed: FlightHubActionAuthorization = {
  hasPermission: true, teamId: 11, connectorProjectId: 17, connectorTeamId: 11, connectorStatus: "connected",
  actionEnabled: true, capabilityFieldVerified: true, taskRunProjectId: 17, taskRunTeamId: 11,
  taskRunStatus: "ready", selectedDeviceId: 41, safetyPolicyVersionId: 51, preflightAllowed: true,
  deviceIdentityPresent: true, approvalProjectId: 17, approvalTeamId: 11, approvalStatus: "approved",
  approvalResourceType: "task_run", approvalResourceId: "47", approvalAction: "flighthub.flight-task.create",
  approvalUnexpired: true, approvalPreflightAllowed: true, waylineProjectId: 17, waylineConnectorId: 43,
  waylineKind: "wayline", targetProjectId: null, targetConnectorId: null, targetKind: null, targetTaskRunId: null
};

test("FlightHub flight action rejects permission, flag, capability, preflight, and approval failures", () => {
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, hasPermission: false }), /PERMISSION_DENIED/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, actionEnabled: false }), /ACTION_DISABLED/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, capabilityFieldVerified: false }), /ACTION_DISABLED/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, preflightAllowed: false }), /PREFLIGHT_FAILED/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, approvalStatus: "pending" }), /APPROVAL_REQUIRED/);
});

test("FlightHub flight action rejects cross-project device, wayline, task, and approval scope", () => {
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, connectorProjectId: 99 }), /SCOPE_MISMATCH/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, deviceIdentityPresent: false }), /DEVICE_SCOPE_MISMATCH/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, waylineProjectId: 99 }), /WAYLINE_SCOPE_MISMATCH/);
  assert.throws(() => authorizeFlightHubAction(17, createInput, { ...allowed, approvalProjectId: 99 }), /APPROVAL_SCOPE_MISMATCH/);
});

test("FlightHub flight action is an asynchronous remotely reconciled intent", () => {
  const plan = authorizeFlightHubAction(17, createInput, allowed);
  assert.equal(plan.dispatchPath, "project-outbox-connector-action-job");
  assert.equal(plan.completion, "await-remote-reconciliation");
  assert.equal(plan.deviceId, 41);
});

test("status and resumption require a canonical remote task in the same project", () => {
  const statusInput = flightHubActionInputSchema.parse({
    connectorInstanceId: 43, taskRunId: 47, approvalRequestId: createInput.approvalRequestId,
    idempotencyKey: "flight-action-0002", action: "flight-task-status", targetResourceId: 61,
    request: { desiredStatus: "suspended" }
  });
  const statusAllowed: FlightHubActionAuthorization = {
    ...allowed, taskRunStatus: "running", approvalAction: "flighthub.flight-task.status",
    waylineProjectId: null, waylineConnectorId: null, waylineKind: null,
    targetProjectId: 17, targetConnectorId: 43, targetKind: "flight-task", targetTaskRunId: "47"
  };
  assert.equal(authorizeFlightHubAction(17, statusInput, statusAllowed).completion, "await-remote-reconciliation");
  assert.throws(() => authorizeFlightHubAction(17, statusInput, { ...statusAllowed, targetTaskRunId: "99" }), /REMOTE_TASK_SCOPE_MISMATCH/);
});

test("FlightHub action API binds connector scope from the route and exposes only safe job state", () => {
  const route = readFileSync(new URL("../app/api/projects/[id]/connectors/dji-flighthub/[connectorId]/flight-actions/route.ts", import.meta.url), "utf8");
  const service = readFileSync(new URL("./dji-flighthub-flight-actions.ts", import.meta.url), "utf8");
  assert.match(route, /\{ \.\.\.body, connectorInstanceId \}/);
  assert.match(route, /status: 202/);
  for (const forbidden of ["request_envelope_json", "request_digest", "remote_id", "identity_json", "credential_envelope_json"]) {
    const publicProjection = service.slice(service.indexOf("export async function readFlightHubActionJob"));
    assert(!publicProjection.includes(forbidden), `public action status reads ${forbidden}`);
  }
});
