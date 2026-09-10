-- name: FHCommandRoutes :many
SELECT to_jsonb(r) FROM (
select binding.connector_instance_id::text as "connectorInstanceId",definition.connector_key as "connectorKey",
              adapter.status as "connectorStatus",binding.priority
         from device_connector_bindings binding
         join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
         join connector_definitions definition on definition.id=adapter.connector_definition_id
        where binding.project_id=sqlc.arg(p1) and binding.device_id=sqlc.arg(p2) and binding.status='active'
        order by binding.priority desc,binding.connector_instance_id limit 2
) r;

-- name: FHCommandGovernance :many
SELECT to_jsonb(r) FROM (
select coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(sqlc.arg(p5)::text,true),false) as "featureEnabled",
          exists(select 1 from connector_capability_snapshots capability where capability.project_id=sqlc.arg(p1)
            and capability.connector_instance_id=sqlc.arg(p3)::bigint and capability.capability_code=sqlc.arg(p6)
			and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
			and capability.region='cn' and capability.deployment='cn-public-cloud'
            and capability.status='supported' and capability.evidence_level='field-write'
            and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
            and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
          latest.captured_at as "stateCapturedAt",project.current_safety_policy_version_id::text as "currentSafetyPolicyVersionId",
          approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",
          approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",
          approval.action as "approvalAction",approval.status as "approvalStatus",
          coalesce(approval.expires_at>now(),false) as "approvalUnexpired"
        from projects project
        join devices device on device.project_id=project.id and device.id=sqlc.arg(p2)
		join device_adapters adapter on adapter.id=sqlc.arg(p3)::bigint and adapter.project_id=project.id
        left join project_feature_flags flags on flags.project_id=project.id
        left join device_latest_telemetry latest on latest.project_id=project.id and latest.device_id=sqlc.arg(p2) and latest.adapter_id=sqlc.arg(p3)::bigint
        left join approval_requests approval on approval.id=sqlc.narg(p4)::uuid and approval.project_id=project.id
        where project.id=sqlc.arg(p1)
) r;
