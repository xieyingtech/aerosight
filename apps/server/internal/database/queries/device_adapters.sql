-- name: ListDeviceAdapters :many
SELECT to_jsonb(r) FROM (
 SELECT id::text AS id,project_id AS "projectId",name,adapter_type AS "adapterType",vendor,
 protocol_version AS "protocolVersion",status,config_json AS config,last_health_json AS "lastHealth",
 last_checked_at AS "lastCheckedAt",updated_at AS "updatedAt"
 FROM device_adapters WHERE project_id=$1 ORDER BY name
) r;

-- name: InsertDeviceAdapter :one
WITH inserted AS (
 INSERT INTO device_adapters(project_id,team_id,name,adapter_type,vendor,protocol_version,config_json)
 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *
)
SELECT to_jsonb(r) FROM (
 SELECT id::text AS id,project_id AS "projectId",name,adapter_type AS "adapterType",vendor,
 protocol_version AS "protocolVersion",status,config_json AS config,last_health_json AS "lastHealth",
 last_checked_at AS "lastCheckedAt",updated_at AS "updatedAt" FROM inserted
) r;

-- name: StoreDeviceAdapterEnvelope :exec
UPDATE device_adapters SET credential_envelope_json=sqlc.arg(envelope)::jsonb WHERE project_id=$1 AND id=$2;

-- name: SetDeviceAdapterEnabled :one
UPDATE device_adapters SET status=$3,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING id::text,status;

-- name: LockDJIAdapterEnvelope :one
SELECT credential_envelope_json FROM device_adapters WHERE project_id=$1 AND id=$2 AND adapter_type='dji' FOR UPDATE;

-- name: UpdateDJIAdapterEnvelope :exec
UPDATE device_adapters SET credential_envelope_json=sqlc.arg(envelope)::jsonb,status='connecting',updated_at=now() WHERE project_id=$1 AND id=$2;
