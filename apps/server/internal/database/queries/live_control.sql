-- name: LockLiveControlDevice :one
SELECT device.id FROM devices device JOIN live_streams stream ON stream.device_id=device.id AND stream.project_id=device.project_id
WHERE stream.project_id=$1 AND stream.id=$2 FOR UPDATE OF device;

-- name: LockLiveControlSession :one
SELECT id,device_id,stream_key,source_type,status,playback_ref,last_active_at,status_reason,vendor_stream_ref
FROM live_streams WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: StopLiveControlSession :exec
UPDATE live_streams SET status=sqlc.arg(status),
 ended_at=CASE WHEN sqlc.arg(status)::text='stopping' THEN null ELSE now() END,
 playback_ref=CASE WHEN sqlc.arg(status)::text='stopping' THEN playback_ref ELSE null END,
 playback_locator_expires_at=null,
 lease_owner=CASE WHEN sqlc.arg(status)::text='stopping' THEN sqlc.arg(lease_owner)::text ELSE null END,
 lease_expires_at=CASE WHEN sqlc.arg(status)::text='stopping' THEN now()+interval '45 seconds' ELSE null END,
 updated_at=now() WHERE project_id=sqlc.arg(project_id) AND id=sqlc.arg(id);

-- name: InsertLiveControlCommand :one
INSERT INTO device_commands(id,project_id,team_id,device_id,live_stream_id,command_key,idempotency_key,capability_code,parameters_json,safety_context_json,status,priority,deadline_at,requested_by_user_id)
VALUES(sqlc.arg(id),sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(device_id),sqlc.arg(stream_id),sqlc.arg(command_key),sqlc.arg(idempotency_key),'stream.video.control',sqlc.arg(parameters),sqlc.arg(safety),'dispatchable',sqlc.arg(priority),now()+interval '30 seconds',sqlc.arg(actor_user_id))
ON CONFLICT(live_stream_id,command_key) WHERE live_stream_id IS NOT NULL DO UPDATE SET command_key=excluded.command_key RETURNING id;
