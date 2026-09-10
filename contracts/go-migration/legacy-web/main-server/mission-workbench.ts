import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { availableMissionActions } from "@/lib/mission-workbench-core";
import type { TaskRunStatus } from "@/lib/task-run-core";

export async function listMissionRuns(projectId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  return (await query<Record<string, unknown>>(
    `select run.id, run.status, run.state_version as "stateVersion", run.created_at as "createdAt",
            run.started_at as "startedAt", run.finished_at as "finishedAt", task.name as "taskName",
            device.name as "deviceName"
       from task_runs run
       join tasks task on task.id = run.task_id and task.project_id = run.project_id
       left join devices device on device.id = run.selected_device_id and device.project_id = run.project_id
      where run.project_id = $1 order by run.created_at desc`, [projectId]
  )).rows;
}

export async function readMissionWorkbench(projectId: number, taskRunId: number) {
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const run = (await query<Record<string, unknown>>(
    `select run.id, run.status, run.state_version as "stateVersion", run.state_reason as "stateReason",
            run.current_step_position as "currentStepPosition", run.preflight_snapshot_json as preflight,
            run.trigger_source as "triggerSource",run.trigger_key as "triggerKey",
            run.input_snapshot_json as "inputSnapshot",run.output_snapshot_json as "outputSnapshot",
            run.created_at as "createdAt", run.started_at as "startedAt", run.finished_at as "finishedAt",
            task.id as "taskId",task.name as "taskName", version.id as "taskVersionId",version.version as "taskVersion", device.id as "deviceId",
            device.name as "deviceName", device.status as "deviceStatus",
            policy.version as "safetyPolicyVersion", approval.status as "approvalStatus"
       from task_runs run
       join tasks task on task.id = run.task_id and task.project_id = run.project_id
       left join task_versions version on version.id = run.task_version_id and version.project_id = run.project_id
       left join devices device on device.id = run.selected_device_id and device.project_id = run.project_id
       left join safety_policy_versions policy on policy.id = run.safety_policy_version_id and policy.project_id = run.project_id
       left join approval_requests approval on approval.id = run.approval_request_id and approval.project_id = run.project_id
      where run.project_id = $1 and run.id = $2`, [projectId, taskRunId]
  )).rows[0];
  if (!run) throw new Error("TASK_RUN_NOT_FOUND");
  const steps = (await query<Record<string, unknown>>(
    `select run_step.id,run_step.position,step.step_key as key,step.name,step.uses,step.action,step.capability_code as "capabilityCode",
            step.input_schema_json as "inputSchema",step.output_schema_json as "outputSchema",step.condition_json as condition,
            step.depends_on_json as "dependsOn",step.retry_policy_json as retry,step.failure_policy_json->>'onFailure' as "onFailure",
            run_step.status, run_step.attempt_count as "attemptCount", run_step.result_json as result,
            run_step.input_snapshot_json as "inputSnapshot",run_step.output_snapshot_json as "outputSnapshot",
            run_step.condition_result_json as "conditionResult",run_step.execution_key as "executionKey",
            command.id as "commandId", command.status as "commandStatus", command.deadline_at as "deadlineAt",
            command.result_json as "commandResult"
       from task_run_steps run_step
       join task_steps step on step.id = run_step.task_step_id and step.project_id = run_step.project_id
       left join lateral (
         select candidate.id, candidate.status, candidate.deadline_at, candidate.result_json
           from device_commands candidate where candidate.task_run_step_id = run_step.id
          order by candidate.created_at desc limit 1
       ) command on true
      where run_step.project_id = $1 and run_step.task_run_id = $2 order by run_step.position`, [projectId, taskRunId]
  )).rows;
  const audit = (await query<Record<string, unknown>>(`select id,request_id as "requestId",idempotency_key as "idempotencyKey",
    action,resource_type as "resourceType",resource_id as "resourceId",status,policy_result_json as policy,
    actor_user_id as "actorUserId",actor_agent_id as "actorAgentId",created_at as "createdAt",completed_at as "completedAt"
    from audit_events where project_id=$1 and (
      (resource_type='task_run' and resource_id=$2::text)
      or (resource_type='task_version' and resource_id=$3::text)
      or (resource_type='task' and resource_id=$4::text)
    ) order by created_at,id`, [projectId,taskRunId,run.taskVersionId,run.taskId])).rows;
  return {
    run,
    steps,
    audit,
    actions: availableMissionActions(run.status as TaskRunStatus, access.permissions)
  };
}
