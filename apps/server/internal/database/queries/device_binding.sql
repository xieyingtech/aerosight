-- name: LockDiscoveredDevice :one
SELECT id,adapter_id,device_id,identity_json FROM device_external_identities
WHERE project_id=$1 AND id=$2 FOR UPDATE;

-- name: InsertDiscoveredDevice :one
WITH selected_type AS (
 SELECT id FROM device_types WHERE status='active' AND type_key IN (sqlc.arg(type_key)::text,'legacy.device')
 ORDER BY CASE WHEN type_key=sqlc.arg(type_key)::text THEN 0 ELSE 1 END,version DESC LIMIT 1
)
INSERT INTO devices(project_id,adapter_id,device_type_id,name,type,status,metadata_json)
SELECT sqlc.arg(project_id),sqlc.arg(adapter_id),selected_type.id,sqlc.arg(name),sqlc.arg(device_type),'unknown',
 jsonb_build_object('identityId',sqlc.arg(identity_id)::bigint,'requestedDeviceTypeKey',sqlc.arg(type_key)::text)
FROM selected_type RETURNING id;

-- name: BindDiscoveredDevice :exec
UPDATE device_external_identities SET device_id=$3,bound_at=now() WHERE project_id=$1 AND id=$2;

-- name: DeclareDiscoveredCapability :exec
INSERT INTO device_capabilities(device_id,project_id,capability_code,declared_by_adapter_id)
VALUES($1,$2,$3,$4) ON CONFLICT(device_id,capability_code) DO UPDATE
SET version_number=device_capabilities.version_number+1,declared_by_adapter_id=excluded.declared_by_adapter_id,updated_at=now();
