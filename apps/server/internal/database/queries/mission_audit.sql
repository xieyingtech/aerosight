-- name: GetMissionAuditRun :one
SELECT trigger_source,task_version_id,safety_policy_version_id,approval_request_id,preflight_snapshot_json
FROM task_runs WHERE project_id=$1 AND id=$2;

-- name: GetMissionAuditRequest :one
SELECT request_id,action,actor_user_id,actor_agent_id,created_at FROM audit_events
WHERE project_id=$1 AND ((resource_type='task_run' AND resource_id=sqlc.arg(run_id)::text)
 OR (resource_type='task_version' AND resource_id=sqlc.narg(version_id)::text))
ORDER BY CASE WHEN action IN('agent.request_mission_start','task_run.transition','task_run.emergency_stop') THEN 0 ELSE 1 END,created_at LIMIT 1;

-- name: GetMissionAuditApproval :one
SELECT request.id,request.status,request.required_approvals,count(decision.id)::int AS received_approvals
FROM approval_requests request LEFT JOIN approvals decision ON decision.approval_request_id=request.id AND decision.project_id=request.project_id
WHERE request.project_id=$1 AND request.id=$2 GROUP BY request.id;

-- name: GetMissionAuditCommands :many
SELECT to_jsonb(r) FROM (
 SELECT command.id::text,command.command_key AS action,command.capability_code AS "capabilityCode",command.status,command.priority,
 attempt.attempt,attempt.status AS "attemptStatus",attempt.error_code AS "errorCode"
 FROM device_commands command LEFT JOIN LATERAL (
  SELECT item.attempt,item.status,item.error_code FROM command_attempts item
  WHERE item.command_id=command.id AND item.project_id=command.project_id ORDER BY item.attempt DESC LIMIT 1
 ) attempt ON true
 WHERE command.project_id=$1 AND command.task_run_id=$2 ORDER BY command.created_at
) r;

-- name: GetEmergencyDrillDevice :one
SELECT (device.status IN('online','degraded'))::boolean AS connected,
 (EXISTS(SELECT 1 FROM device_capabilities capability WHERE capability.device_id=device.id AND capability.project_id=device.project_id
 AND capability.capability_code IN('safety.emergency_stop','flight.return_home','command.rth')))::boolean AS capability_declared
FROM task_runs run JOIN devices device ON device.id=run.selected_device_id AND device.project_id=run.project_id
WHERE run.project_id=$1 AND run.id=$2;
