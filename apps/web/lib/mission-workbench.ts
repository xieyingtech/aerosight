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
            run.created_at as "createdAt", run.started_at as "startedAt", run.finished_at as "finishedAt",
            task.name as "taskName", version.version as "taskVersion", device.id as "deviceId",
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
    `select run_step.position, step.name, step.action, step.capability_code as "capabilityCode",
            run_step.status, run_step.attempt_count as "attemptCount", run_step.result_json as result,
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
  return {
    run,
    steps,
    actions: availableMissionActions(run.status as TaskRunStatus, access.permissions)
  };
}
