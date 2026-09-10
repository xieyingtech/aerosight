-- name: LockDeviceCommandTarget :one
SELECT device.project_id,device.device_type_id,device_type.type_key,device.status,capability.availability,capability.risk_level
FROM devices device JOIN device_types device_type ON device_type.id=device.device_type_id JOIN device_capabilities capability ON capability.device_id=device.id AND capability.project_id=device.project_id
WHERE device.project_id=$1 AND device.id=$2 AND capability.capability_code=$3 FOR UPDATE OF device,capability;

-- name: LockDeviceCommandGrants :many
SELECT action_pattern,effect FROM device_capability_grants
WHERE project_id=sqlc.arg(project_id) AND team_id=sqlc.arg(team_id) AND user_id=sqlc.arg(user_id)
AND (expires_at IS NULL OR expires_at>now())
AND (scope_type='project' OR (scope_type='device_type' AND device_type_id=sqlc.arg(device_type_id)) OR (scope_type='device' AND device_id=sqlc.arg(device_id))) FOR SHARE;

-- name: FindExistingDeviceCommand :one
SELECT id::text,status FROM device_commands WHERE project_id=$1 AND device_id=$2 AND idempotency_key=$3 FOR UPDATE;

-- name: CountDeviceCommandConflicts :one
SELECT count(*)::int AS count FROM task_runs run
WHERE run.project_id=$1 AND run.status IN ('dispatching','running','paused','canceling')
AND (run.selected_device_id=$2 OR run.selected_device_id IN (
SELECT relation.to_device_id FROM device_relationships relation WHERE relation.project_id=$1 AND relation.from_device_id=$2 AND relation.valid_until IS NULL));

-- name: InsertDeviceCommand :one
INSERT INTO device_commands(id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,parameters_json,safety_context_json,status,priority,deadline_at,requested_by_user_id)
VALUES(sqlc.arg(id),sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(device_id),sqlc.arg(command_key),sqlc.arg(idempotency_key),sqlc.arg(capability_code),sqlc.arg(parameters),sqlc.arg(safety),'dispatchable',sqlc.arg(priority),now()+(sqlc.arg(deadline_seconds)::double precision*interval '1 second'),sqlc.arg(user_id))
ON CONFLICT(device_id,idempotency_key) DO UPDATE SET idempotency_key=excluded.idempotency_key RETURNING id::text,status;
