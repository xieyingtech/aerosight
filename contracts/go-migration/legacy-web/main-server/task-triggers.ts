import "server-only";

import { randomUUID } from "node:crypto";
import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { assertUserCallableTaskTrigger, planTaskTrigger, taskTriggerInvocationSchema } from "@/lib/task-trigger-core";

type TriggerVersionRow = {
  projectId: number;
  teamId: number;
  taskId: number;
  taskStatus: string;
  taskVersionId: number;
  taskVersionStatus: string;
  trigger: Record<string, unknown>;
  inputSchema: Record<string, unknown>;
  concurrencyLimit: number;
};

export async function triggerTaskRunAsCurrentUser(input: {
  projectId: number;
  taskId: number;
  invocation: unknown;
  requestId?: string | null;
}) {
  const invocation = taskTriggerInvocationSchema.parse(input.invocation);
  assertUserCallableTaskTrigger(invocation);
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "mission:operate");
  const preliminaryKey = `${invocation.type}:${invocation.idempotencyKey}`;
  return withAuditedProjectWrite({
    projectId: input.projectId,
    teamId: access.teamId,
    requestId: correlationId(input.requestId),
    idempotencyKey: preliminaryKey,
    actorUserId: user.id,
    action: "task_run.trigger",
    resourceType: "task",
    resourceId: String(input.taskId),
    input: { type: invocation.type, idempotencyKey: invocation.idempotencyKey, inputs: invocation.inputs },
    policyResult: { permission: "mission:operate", triggerType: invocation.type }
  }, async (client) => {
    await client.query("select pg_advisory_xact_lock($1,$2)", [input.projectId, input.taskId]);
    const version = (await client.query<TriggerVersionRow>(
      `select task.project_id as "projectId",task.team_id as "teamId",task.id as "taskId",task.status as "taskStatus",
              version.id::int as "taskVersionId",version.status as "taskVersionStatus",version.trigger_json as trigger,
              version.input_schema_json as "inputSchema",version.concurrency_limit as "concurrencyLimit"
         from tasks task
         join task_versions version on version.id=task.current_published_version_id and version.project_id=task.project_id
        where task.project_id=$1 and task.id=$2 for update of task,version`,
      [input.projectId, input.taskId]
    )).rows[0];
    if (!version) throw new Error("TASK_TRIGGER_VERSION_NOT_FOUND");
    const existing = (await client.query<{ id: number; status: string }>(
      `select id,status from task_runs
        where project_id=$1 and task_version_id=$2 and trigger_key=$3`,
      [input.projectId, version.taskVersionId, preliminaryKey]
    )).rows[0];
    if (existing) return { taskRunId: existing.id, status: existing.status, replayed: true };
    const activeRunCount = Number((await client.query<{ count: string }>(
      `select count(*)::text as count from task_runs
        where project_id=$1 and task_version_id=$2
          and status in ('queued','blocked','ready','dispatching','running','paused','canceling')`,
      [input.projectId, version.taskVersionId]
    )).rows[0]?.count ?? 0);
    const plan = planTaskTrigger({
      trigger: version.trigger,
      inputSchema: version.inputSchema,
      invocation,
      authorization: {
        projectId: input.projectId,
        taskProjectId: version.projectId,
        taskVersionStatus: version.taskVersionStatus,
        taskStatus: version.taskStatus,
        concurrencyLimit: version.concurrencyLimit,
        activeRunCount,
        actor: { type: "user", id: String(user.id) }
      }
    });
    const run = (await client.query<{ id: number; status: string }>(
      `insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,trigger_key,status,
                             input_snapshot_json,created_by_user_id,state_reason)
       values($1,$2,$3,$4,$5,$6,'queued',$7,$8,'trigger-accepted')
       returning id,status`,
      [input.projectId, version.teamId, version.taskId, version.taskVersionId, plan.triggerSource,
        plan.triggerKey, plan.inputSnapshot, user.id]
    )).rows[0];
    await client.query(
      `insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status,execution_key)
       select project_id,team_id,$3,id,position,'pending',$4||':step:'||step_key
         from task_steps where project_id=$1 and task_version_id=$2 order by position`,
      [input.projectId, version.taskVersionId, run.id, plan.triggerKey]
    );
    await publishProjectEvent(client, {
      projectId: input.projectId,
      teamId: version.teamId,
      eventId: randomUUID(),
      eventType: "task_run.triggered",
      payload: { taskRunId: run.id, taskVersionId: version.taskVersionId, triggerType: plan.triggerSource },
      enqueue: true
    });
    return { taskRunId: run.id, status: run.status, replayed: false };
  });
}
