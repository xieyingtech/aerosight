-- name: GetAdapterNetworkProfile :one
SELECT a.adapter_type,a.network_profile_id,p.mode,p.mqtt_endpoint,p.api_public_base_url,p.websocket_public_url,
 p.media_ingest_base_url,p.media_playback_base_url,p.tls_required,p.config_json,
 (a.credential_envelope_json IS NOT NULL)::boolean AS has_credential
FROM device_adapters a LEFT JOIN device_network_profiles p ON p.id=a.network_profile_id AND p.project_id=a.project_id
WHERE a.project_id=$1 AND a.id=$2;

-- name: RecordNetworkValidation :exec
UPDATE device_network_profiles SET status=$3,last_validation_json=$4,last_validated_at=now(),updated_at=now()
WHERE project_id=$1 AND id=$2;

-- name: InsertDJINetworkProfile :one
INSERT INTO device_network_profiles(project_id,team_id,name,mode,mqtt_endpoint,api_public_base_url,websocket_public_url,
 media_ingest_base_url,media_playback_base_url,tls_required,status,config_json,last_validation_json)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'unverified',$11,'{"status":"unverified","policyIssues":[]}') RETURNING id;

-- name: InsertDJISetupAdapter :one
WITH inserted AS (
 INSERT INTO device_adapters(project_id,team_id,name,adapter_type,vendor,protocol_version,status,config_json,network_profile_id)
 VALUES($1,$2,$3,'dji','dji','cloud-api-mqtt5','connecting',$4,$5) RETURNING *
)
SELECT to_jsonb(r) FROM (
 SELECT id::text AS id,project_id AS "projectId",name,adapter_type AS "adapterType",vendor,
 protocol_version AS "protocolVersion",status,config_json AS config,last_health_json AS "lastHealth",
 last_checked_at AS "lastCheckedAt",updated_at AS "updatedAt" FROM inserted
) r;

-- name: RecordAdapterHealth :exec
UPDATE device_adapters SET last_health_json=$3,last_checked_at=now(),updated_at=now()
WHERE project_id=$1 AND id=$2;
