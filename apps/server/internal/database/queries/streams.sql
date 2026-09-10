-- name: ReadProjectEvents :many
SELECT cursor::text,event_id,event_type,payload_json,occurred_at
FROM project_events WHERE project_id=$1 AND cursor>$2::bigint ORDER BY project_events.cursor LIMIT 501;

-- name: ResolveChannel :one
SELECT channel.stable_channel_id,channel.device_id,channel.data_type,membership.role,
 device.device_type_id,channel.capability_code,channel.availability
FROM device_stream_channels channel
JOIN devices device ON device.id=channel.device_id AND device.project_id=channel.project_id
JOIN projects project ON project.id=channel.project_id
JOIN team_members membership ON membership.team_id=project.team_id AND membership.user_id=sqlc.arg(user_id)
WHERE channel.project_id=sqlc.arg(project_id) AND channel.stable_channel_id=sqlc.arg(channel_id);

-- name: ChannelGrants :many
SELECT scope_type,device_type_id,device_id,action_pattern,effect
FROM device_capability_grants
WHERE project_id=sqlc.arg(project_id) AND user_id=sqlc.arg(user_id) AND (expires_at IS NULL OR expires_at>now())
 AND (scope_type='project' OR (scope_type='device_type' AND device_type_id=sqlc.arg(device_type_id)) OR (scope_type='device' AND device_id=sqlc.arg(device_id)));

-- name: ReadChannelTelemetry :many
SELECT id::text AS cursor,event_id,captured_at,payload_json,quality_json
FROM device_telemetry WHERE project_id=$1 AND device_id=$2 AND id>$3::bigint ORDER BY id LIMIT 501;

-- name: ReadChannelEvents :many
SELECT cursor::text,event_id,occurred_at AS captured_at,payload_json,'{}'::jsonb AS quality_json
FROM project_events WHERE project_id=$1 AND cursor>$3::bigint AND payload_json->>'deviceId'=$2::text ORDER BY project_events.cursor LIMIT 501;
