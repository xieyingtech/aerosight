import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
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
