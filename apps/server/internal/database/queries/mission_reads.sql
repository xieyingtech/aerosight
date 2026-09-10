-- name: ListMissionRuns :many
SELECT to_jsonb(r) FROM (
 SELECT run.id,run.status,run.state_version AS "stateVersion",run.created_at AS "createdAt",
 run.started_at AS "startedAt",run.finished_at AS "finishedAt",task.name AS "taskName",device.name AS "deviceName"
 FROM task_runs run JOIN tasks task ON task.id=run.task_id AND task.project_id=run.project_id
 LEFT JOIN devices device ON device.id=run.selected_device_id AND device.project_id=run.project_id
 WHERE run.project_id=$1 ORDER BY run.created_at DESC
) r;

-- name: ListTaskDefinitions :many
SELECT to_jsonb(r) FROM (
 SELECT id,name,description,trigger_type AS "triggerType",status,updated_at AS "updatedAt"
 FROM tasks WHERE project_id=$1 ORDER BY updated_at DESC
) r;

-- name: GetTaskDefinition :one
SELECT to_jsonb(r) FROM (
 SELECT id,project_id AS "projectId",name,description,trigger_type AS "triggerType",status,updated_at AS "updatedAt"
 FROM tasks WHERE project_id=$1 AND id=$2
) r;

-- name: GetMissionWorkbenchRun :one
SELECT to_jsonb(r) FROM (
 SELECT run.id,run.status,run.state_version AS "stateVersion",run.state_reason AS "stateReason",
 run.trigger_source AS "triggerSource",run.trigger_key AS "triggerKey",
 run.input_snapshot_json AS "inputSnapshot",run.output_snapshot_json AS "outputSnapshot",
 run.current_step_position AS "currentStepPosition",run.preflight_snapshot_json AS preflight,
 run.created_at AS "createdAt",run.started_at AS "startedAt",run.finished_at AS "finishedAt",
 task.id AS "taskId",version.id AS "taskVersionId",task.name AS "taskName",version.version AS "taskVersion",device.id AS "deviceId",device.name AS "deviceName",
 device.status AS "deviceStatus",policy.version AS "safetyPolicyVersion",approval.status AS "approvalStatus"
 FROM task_runs run JOIN tasks task ON task.id=run.task_id AND task.project_id=run.project_id
 LEFT JOIN task_versions version ON version.id=run.task_version_id AND version.project_id=run.project_id
 LEFT JOIN devices device ON device.id=run.selected_device_id AND device.project_id=run.project_id
 LEFT JOIN safety_policy_versions policy ON policy.id=run.safety_policy_version_id AND policy.project_id=run.project_id
 LEFT JOIN approval_requests approval ON approval.id=run.approval_request_id AND approval.project_id=run.project_id
 WHERE run.project_id=$1 AND run.id=$2
) r;

-- name: GetMissionWorkbenchSteps :many
SELECT to_jsonb(r) FROM (
 SELECT run_step.id,step.step_key AS key,step.uses,
 step.input_schema_json AS "inputSchema",step.output_schema_json AS "outputSchema",step.condition_json AS condition,
 step.depends_on_json AS "dependsOn",step.retry_policy_json AS retry,step.failure_policy_json->>'onFailure' AS "onFailure",
 run_step.input_snapshot_json AS "inputSnapshot",run_step.output_snapshot_json AS "outputSnapshot",
 run_step.condition_result_json AS "conditionResult",run_step.execution_key AS "executionKey",
 run_step.position,step.name,step.action,step.capability_code AS "capabilityCode",run_step.status,
 run_step.attempt_count AS "attemptCount",run_step.result_json AS result,command.id::text AS "commandId",
 command.status AS "commandStatus",command.deadline_at AS "deadlineAt",command.result_json AS "commandResult"
 FROM task_run_steps run_step JOIN task_steps step ON step.id=run_step.task_step_id AND step.project_id=run_step.project_id
 LEFT JOIN LATERAL (
  SELECT candidate.id,candidate.status,candidate.deadline_at,candidate.result_json FROM device_commands candidate
  WHERE candidate.task_run_step_id=run_step.id AND candidate.project_id=run_step.project_id
  ORDER BY candidate.created_at DESC LIMIT 1
 ) command ON true
 WHERE run_step.project_id=$1 AND run_step.task_run_id=$2 ORDER BY run_step.position
) r;

-- name: GetMissionWorkbenchAudit :many
SELECT to_jsonb(r) FROM (
 SELECT audit.id,audit.request_id AS "requestId",audit.idempotency_key AS "idempotencyKey",
 audit.action,audit.resource_type AS "resourceType",audit.resource_id AS "resourceId",audit.status,
 audit.policy_result_json AS policy,audit.actor_user_id AS "actorUserId",audit.actor_agent_id AS "actorAgentId",
 audit.created_at AS "createdAt",audit.completed_at AS "completedAt"
 FROM audit_events audit JOIN task_runs run ON run.project_id=audit.project_id
 WHERE run.project_id=$1 AND run.id=$2 AND (
 (audit.resource_type='task_run' AND audit.resource_id=run.id::text) OR
 (audit.resource_type='task_version' AND audit.resource_id=run.task_version_id::text) OR
 (audit.resource_type='task' AND audit.resource_id=run.task_id::text)) ORDER BY audit.created_at,audit.id
) r;
