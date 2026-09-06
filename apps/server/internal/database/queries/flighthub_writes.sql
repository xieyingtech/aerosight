-- name: CreateFlightHubConnector :one
INSERT INTO device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,vendor,protocol_version,status,config_json,capabilities_json,onboarding_policy,discovery_scope_json,external_scope_key)
SELECT sqlc.arg(project_id),sqlc.arg(team_id),sqlc.arg(name),'dji-flighthub2',definition.id,'dji','flighthub-openapi-v2','connecting','{"region":"cn","readOnly":true}','{"inventoryRead":true,"stateRead":true}','review',sqlc.arg(discovery_scope)::jsonb,sqlc.arg(external_scope)::text
FROM connector_definitions definition WHERE connector_key='dji.flighthub2' AND version='1.0.0' AND status='active'
RETURNING id,status,created_at;

-- name: UpdateFlightHubCredentials :execrows
UPDATE device_adapters SET credential_envelope_json=$3::jsonb,status='connecting',last_health_json='{}',last_checked_at=now(),updated_at=now()
WHERE id=$1 AND project_id=$2;

-- name: FindFlightHubConnector :one
SELECT adapter.id,adapter.status,coalesce(adapter.discovery_scope_json->>'projectUuid','')::text AS project_uuid,
coalesce(adapter.discovery_scope_json->>'projectName','')::text AS project_name
FROM device_adapters adapter JOIN connector_definitions definition ON definition.id=adapter.connector_definition_id
WHERE adapter.id=$1 AND adapter.project_id=$2 AND definition.connector_key='dji.flighthub2' AND definition.version='1.0.0';

-- name: LockFlightHubConnector :one
SELECT adapter.id,adapter.status,coalesce(adapter.discovery_scope_json->>'projectUuid','')::text AS project_uuid,
coalesce(adapter.discovery_scope_json->>'projectName','')::text AS project_name
FROM device_adapters adapter JOIN connector_definitions definition ON definition.id=adapter.connector_definition_id
WHERE adapter.id=$1 AND adapter.project_id=$2 AND definition.connector_key='dji.flighthub2' AND definition.version='1.0.0'
FOR UPDATE OF adapter;

-- name: LockFlightHubSyncQueue :exec
SELECT pg_advisory_xact_lock(hashtext(sqlc.arg(lock_key)::text));

-- name: FindQueuedFlightHubSync :one
SELECT event_id FROM outbox_events WHERE project_id=$1 AND event_type='connector.sync.requested'
AND payload_json->>'connectorInstanceId'=$2::text AND status IN ('pending','processing') ORDER BY id LIMIT 1;

-- name: DisableFlightHubConnector :execrows
UPDATE device_adapters SET status='disabled',lease_owner=null,lease_expires_at=null,
last_health_json='{"ok":false,"code":"CONNECTOR_DISCONNECTED"}',updated_at=now() WHERE id=$1 AND project_id=$2;

-- name: DisableFlightHubBindings :exec
UPDATE device_connector_bindings SET status='disabled',unbound_at=coalesce(unbound_at,now())
WHERE project_id=$1 AND connector_instance_id=$2 AND status<>'disabled';

-- name: CancelFlightHubSyncRuns :exec
UPDATE connector_sync_runs SET status='cancelled',error_code='CONNECTOR_DISCONNECTED',started_at=coalesce(started_at,now()),finished_at=coalesce(finished_at,now())
WHERE project_id=$1 AND connector_instance_id=$2 AND status IN ('pending','running');

-- name: CancelFlightHubSyncQueue :exec
UPDATE outbox_events SET status='dead',last_error='CONNECTOR_DISCONNECTED',completed_at=now(),locked_by=null,locked_until=null
WHERE project_id=$1 AND event_type='connector.sync.requested' AND payload_json->>'connectorInstanceId'=$2::text AND status IN ('pending','processing');
