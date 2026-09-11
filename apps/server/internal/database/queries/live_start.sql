-- name: LockLiveStartDevice :one
SELECT device.status,device.type AS device_type,device.adapter_id,adapter.adapter_type,device.device_type_id,
 coalesce((SELECT array_agg(c.capability_code) FROM device_capabilities c WHERE c.device_id=device.id AND c.project_id=device.project_id AND c.availability='available'),'{}')::text[] AS capabilities,
 coalesce((SELECT greatest(1,least(16,coalesce((c.constraints_json->>'maxConcurrentSessions')::int,1))) FROM device_capabilities c
 WHERE c.device_id=device.id AND c.project_id=device.project_id AND c.capability_code IN ('stream.video.control','camera.live') AND c.availability='available'
 ORDER BY c.capability_code='stream.video.control' DESC LIMIT 1),1)::int AS max_concurrent_sessions,
 profile.media_ingest_base_url,adapter.credential_envelope_json,definition.connector_key,
 coalesce((flags.flighthub_action_flags_json->>'live.control')::boolean,false)::boolean as live_action_enabled,
 exists(select 1 from connector_capability_snapshots capability where capability.project_id=device.project_id
 and capability.connector_instance_id=adapter.id and capability.capability_code='live.control' and capability.status='supported'
 and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
 and capability.region='cn' and capability.deployment='cn-public-cloud' and capability.evidence_level='field-write'
 and capability.device_model=device.device_model and capability.firmware_version is null
 and (capability.expires_at is null or capability.expires_at>now())) as live_capability_verified
FROM devices device LEFT JOIN device_adapters adapter ON adapter.id=device.adapter_id AND adapter.project_id=device.project_id
LEFT JOIN connector_definitions definition ON definition.id=adapter.connector_definition_id
LEFT JOIN project_feature_flags flags ON flags.project_id=device.project_id
LEFT JOIN device_network_profiles profile ON profile.id=adapter.network_profile_id AND profile.project_id=adapter.project_id
WHERE device.project_id=$1 AND device.id=$2 FOR UPDATE OF device;

-- name: ReadLiveStartChannel :one
SELECT id,channel_key FROM device_stream_channels WHERE project_id=$1 AND device_id=$2 AND data_type='video' AND availability='available'
AND (sqlc.narg(stream_key)::text IS NULL OR channel_key=sqlc.narg(stream_key)::text) ORDER BY channel_key LIMIT 1;

-- name: FindLiveStartReplay :one
SELECT id FROM live_streams WHERE project_id=$1 AND device_id=$2 AND stream_key=$3 AND status IN ('requested','starting','live','degraded','stopping') LIMIT 1;

-- name: CountLiveStartSessions :one
SELECT count(*)::int FROM live_streams WHERE project_id=$1 AND device_id=$2 AND status IN ('requested','starting','live','degraded','stopping');

-- name: ReadDJILiveTopology :one
SELECT parent_identity.external_device_id,(camera_identity.identity_json->>'productType')::int AS camera_type,(camera_identity.identity_json->>'productSubtype')::int AS camera_subtype
FROM device_external_identities camera_identity JOIN device_relationships relation ON relation.project_id=camera_identity.project_id AND relation.to_device_id=camera_identity.device_id AND relation.valid_until IS NULL AND relation.relation_type IN ('contains','mounted-on')
JOIN device_external_identities parent_identity ON parent_identity.project_id=relation.project_id AND parent_identity.adapter_id=camera_identity.adapter_id AND parent_identity.device_id=relation.from_device_id
WHERE camera_identity.project_id=$1 AND camera_identity.device_id=$2 ORDER BY relation.valid_from DESC LIMIT 1;

-- name: InsertLiveStartSession :one
INSERT INTO live_streams(project_id,team_id,device_id,adapter_id,stream_key,stream_channel_id,source_type,status,ingest_ref,vendor_stream_ref,playback_ref,started_by_user_id,last_active_at,lease_owner,lease_expires_at)
VALUES(sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(device_id),sqlc.narg(adapter_id),sqlc.arg(stream_key),sqlc.narg(channel_id),sqlc.arg(source_type),sqlc.arg(status),sqlc.arg(ingest_ref),sqlc.narg(vendor_ref),sqlc.narg(playback_ref),sqlc.arg(actor_user_id),
CASE WHEN sqlc.arg(status)::text='live' THEN now() ELSE null END,sqlc.narg(lease_owner),CASE WHEN sqlc.narg(lease_owner)::text IS NULL THEN null ELSE now()+interval '45 seconds' END) RETURNING id;
