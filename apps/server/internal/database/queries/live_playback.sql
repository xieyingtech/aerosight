-- name: LockLivePlayback :one
SELECT stream.id,stream.device_id,stream.stream_key,stream.source_type,stream.status,stream.playback_ref,stream.last_active_at,stream.status_reason,
 stream.start_accepted_at,stream.local_authorization_revoked_at,stream.supplier_credential_expires_at,
 device.device_type_id,coalesce(channel.capability_code,'camera.live')::text AS capability_code,
 coalesce(profile.media_playback_base_url,'')::text AS hls_base_url,coalesce(profile.config_json->>'webrtcPlaybackBaseUrl','')::text AS webrtc_base_url
FROM live_streams stream JOIN devices device ON device.id=stream.device_id AND device.project_id=stream.project_id
LEFT JOIN device_stream_channels channel ON channel.id=stream.stream_channel_id AND channel.project_id=stream.project_id
LEFT JOIN device_adapters adapter ON adapter.id=stream.adapter_id AND adapter.project_id=stream.project_id
LEFT JOIN device_network_profiles profile ON profile.id=adapter.network_profile_id AND profile.project_id=adapter.project_id
WHERE stream.project_id=$1 AND stream.id=$2 FOR UPDATE OF stream;

-- name: SetLivePlaybackExpiry :exec
UPDATE live_streams SET playback_locator_expires_at=$3,updated_at=now() WHERE project_id=$1 AND id=$2;

-- name: AuthorizeFlightHubPlayback :one
UPDATE live_streams SET last_playback_at=now(),
 playback_locator_expires_at=least(supplier_credential_expires_at,now()+interval '60 seconds'),updated_at=now()
 WHERE project_id=$1 AND id=$2 AND source_type='dji_flighthub'
 AND status IN ('starting','live','degraded') AND (status<>'starting' OR start_accepted_at IS NOT NULL)
 AND local_authorization_revoked_at IS NULL AND supplier_credential_expires_at>now()
 AND supplier_credential_envelope_json IS NOT NULL
 RETURNING supplier,supplier_protocol,supplier_adapter_version,supplier_credential_envelope_json,playback_locator_expires_at;
