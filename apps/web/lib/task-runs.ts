import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { requiredMissionPermission, type MissionAction } from "@/lib/mission-workbench-core";
import { transitionTaskRun, type TaskRunStatus } from "@/lib/task-run-core";

export async function transitionMissionRun(input: {
  projectId: number;
  taskRunId: number;
  expectedVersion: number;
  nextStatus: TaskRunStatus;
  reason: string;
  requestId?: string | null;
}) {
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "mission:operate");
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId: correlationId(input.requestId),
    actorUserId: user.id, action: "task_run.transition", resourceType: "task_run",
    resourceId: String(input.taskRunId), input, policyResult: { permission: "mission:operate" }
  }, async (client) => {
    const current = await client.query<{ status: TaskRunStatus; stateVersion: number }>(
      `select status, state_version as "stateVersion" from task_runs
        where project_id = $1 and id = $2 for update`, [input.projectId, input.taskRunId]
    );
    if (!current.rows[0]) throw new Error("TASK_RUN_NOT_FOUND");
    const next = transitionTaskRun(current.rows[0], input.expectedVersion, input.nextStatus, input.reason);
    const updated = await client.query(
      `update task_runs set status = $4, state_version = state_version + 1, state_reason = $5,
          started_at = case when $4 = 'running' and started_at is null then now() else started_at end,
          finished_at = case when $4 in ('succeeded','failed','canceled') then now() else finished_at end
        where project_id = $1 and id = $2 and state_version = $3
        returning id, status, state_version as "stateVersion", state_reason as reason`,
      [input.projectId, input.taskRunId, input.expectedVersion, next.status, input.reason]
    );
    if (!updated.rows[0]) throw new Error("TASK_RUN_VERSION_CONFLICT");
    await publishProjectEvent(client, {
      projectId: input.projectId, teamId: access.teamId, eventId: randomUUID(),
      eventType: "task_run.transitioned",
      payload: { taskRunId: input.taskRunId, from: current.rows[0].status, to: next.status, stateVersion: next.stateVersion, reason: input.reason },
      enqueue: true
    });
    return updated.rows[0];
  });
}

export async function controlMissionRun(input: {
  projectId: number;
  taskRunId: number;
  expectedVersion: number;
  action: MissionAction;
  reason: string;
  requestId?: string | null;
}) {
  const permission = requiredMissionPermission(input.action);
  const { user, access } = await requireCurrentProjectPermission(input.projectId, permission);
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId: correlationId(input.requestId),
    actorUserId: user.id, action: `task_run.${input.action}`, resourceType: "task_run",
    resourceId: String(input.taskRunId), input, policyResult: { permission }
  }, async (client) => {
    const current = await client.query<{
      status: TaskRunStatus; stateVersion: number; approvalRequestId: string | null;
    }>(
      `select status, state_version as "stateVersion", approval_request_id as "approvalRequestId"
         from task_runs where project_id = $1 and id = $2 for update`, [input.projectId, input.taskRunId]
    );
    const row = current.rows[0];
    if (!row) throw new Error("TASK_RUN_NOT_FOUND");
    if (row.stateVersion !== input.expectedVersion) throw new Error("TASK_RUN_VERSION_CONFLICT");
    if (input.action === "approve") {
      if (!row.approvalRequestId) throw new Error("TASK_RUN_APPROVAL_NOT_REQUIRED");
      await client.query(
        `insert into approvals (project_id, team_id, approval_request_id, approver_user_id, decision, reason)
         values ($1, $2, $3, $4, 'approved', $5)`,
        [input.projectId, access.teamId, row.approvalRequestId, user.id, input.reason]
      );
      return { id: input.taskRunId, status: row.status, stateVersion: row.stateVersion, approval: "approved" };
    }
    const nextStatus: TaskRunStatus = input.action === "pause" ? "paused"
      : input.action === "resume" ? "running"
        : input.action === "cancel" && row.status === "queued" ? "canceled" : "canceling";
    const next = input.action === "emergency_stop" && row.status === "canceling"
      ? { status: "canceling" as const, stateVersion: row.stateVersion + 1, reason: input.reason }
      : transitionTaskRun(row, input.expectedVersion, nextStatus, input.reason);
    const updated = await client.query(
      `update task_runs set status = $4, state_version = state_version + 1, state_reason = $5
        where project_id = $1 and id = $2 and state_version = $3
        returning id, status, state_version as "stateVersion"`,
      [input.projectId, input.taskRunId, input.expectedVersion, next.status, input.reason]
    );
    if (!updated.rows[0]) throw new Error("TASK_RUN_VERSION_CONFLICT");
    const control = (input.action === "cancel" && next.status === "canceling") || input.action === "emergency_stop";
    await publishProjectEvent(client, {
      projectId: input.projectId, teamId: access.teamId, eventId: randomUUID(),
      eventType: control ? "mission.control" : "task_run.transitioned",
      payload: control
        ? { taskRunId: input.taskRunId, control: input.action }
        : { taskRunId: input.taskRunId, from: row.status, to: next.status, stateVersion: next.stateVersion, reason: input.reason },
      enqueue: true
    });
    return updated.rows[0];
  });
}
