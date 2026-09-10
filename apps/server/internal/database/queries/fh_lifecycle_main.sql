-- name: FHExpireReadCapabilities :exec
update connector_capability_snapshots
set expires_at=case when verified_at<now() then now() else verified_at+interval '1 microsecond' end,updated_at=now()
where project_id=$1 and connector_instance_id=$2 and evidence_level='live-read';

-- name: FHReconnect :exec
update device_adapters set status='connecting',lease_owner=null,lease_expires_at=null,
last_health_json='{}'::jsonb,last_checked_at=null,updated_at=now()
where id=$1 and project_id=$2 and status='disabled';

-- name: FHRestoreBindings :execrows
update device_connector_bindings set status='active',unbound_at=null
where project_id=$1 and connector_instance_id=$2 and status='disabled';
