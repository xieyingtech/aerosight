-- name: FHControlledAccess :many
SELECT to_jsonb(r) FROM (
select membership.role,adapter.status as "connectorStatus",
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",
	  adapter.discovery_scope_json->>'accountFingerprint' as "accountFingerprint",
      definition.manifest_json as manifest,coalesce(flags.flighthub_action_flags_json,'{}'::jsonb) as "featureFlags",
      (membership.role='owner' or exists(select 1 from project_permissions permission where permission.project_id=project.id
        and permission.team_id=project.team_id and permission.user_id=sqlc.arg(p1) and permission.permission='organization:manage')) as "managementGranted"
    from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(p1)
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id
      and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    left join project_feature_flags flags on flags.project_id=project.id
    where project.id=sqlc.arg(p2) and adapter.id=sqlc.arg(p3)
) r;

-- name: FHControlledCapabilities :many
SELECT to_jsonb(r) FROM (
select distinct capability_code as "capabilityCode"
    from connector_capability_snapshots capability
	where capability.project_id=sqlc.arg(p1) and capability.connector_instance_id=sqlc.arg(p2) and capability.status='supported'
      and capability.account_fingerprint=sqlc.narg(p3) and capability.evidence_level='field-write'
	  and capability.region='cn' and capability.deployment='cn-public-cloud'
	  and (capability.expires_at is null or capability.expires_at>now())
	  and ((capability.device_model is null and capability.firmware_version is null) or exists(
	    select 1 from device_external_identities identity
		join devices device on device.id=identity.device_id and device.project_id=identity.project_id
		where identity.project_id=sqlc.arg(p1) and identity.adapter_id=sqlc.arg(p2) and identity.discovery_status='managed'
		  and device.device_model=capability.device_model and device.firmware_version=capability.firmware_version
	  ))
) r;

-- name: FHControlledJobs :many
SELECT to_jsonb(r) FROM (
select * from (
      select id::text,action_kind as action,status,last_error_code as "lastErrorCode",completed_at as "completedAt",updated_at as "updatedAt"
        from connector_action_jobs where connector_action_jobs.project_id=sqlc.arg(p1) and connector_action_jobs.connector_instance_id=sqlc.arg(p2)
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_device_admin_jobs where connector_device_admin_jobs.project_id=sqlc.arg(p1) and connector_device_admin_jobs.connector_instance_id=sqlc.arg(p2)
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_management_write_jobs where connector_management_write_jobs.project_id=sqlc.arg(p1) and connector_management_write_jobs.connector_instance_id=sqlc.arg(p2)
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_geospatial_action_jobs where connector_geospatial_action_jobs.project_id=sqlc.arg(p1) and connector_geospatial_action_jobs.connector_instance_id=sqlc.arg(p2)
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_model_delete_jobs where connector_model_delete_jobs.project_id=sqlc.arg(p1) and connector_model_delete_jobs.connector_instance_id=sqlc.arg(p2)
      union all select id::text,action_kind,status,last_error_code,completed_at,updated_at
        from connector_live_action_jobs where connector_live_action_jobs.project_id=sqlc.arg(p1) and connector_live_action_jobs.connector_instance_id=sqlc.arg(p2)
    ) jobs order by "updatedAt" desc limit 50
) r;
