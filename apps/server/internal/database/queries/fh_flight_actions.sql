-- name: FHFlightAuthorize :many
SELECT to_jsonb(r) FROM (
select (member.role in('owner','admin') or exists(select 1 from project_permissions permission
        where permission.project_id=run.project_id and permission.team_id=run.team_id
          and permission.user_id=member.user_id and permission.permission='mission:operate')) as "hasPermission",sqlc.arg(p3)::int as "teamId",
      adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      coalesce(flags.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb,false) as "actionEnabled",
      exists(select 1 from connector_capability_snapshots capability
        where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
          and capability.capability_code='flight.execute' and capability.status='supported'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
          and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
      run.project_id as "taskRunProjectId",run.team_id as "taskRunTeamId",run.status as "taskRunStatus",
      run.selected_device_id as "selectedDeviceId",run.safety_policy_version_id as "safetyPolicyVersionId",
      coalesce((run.preflight_snapshot_json->>'allowed')::boolean,false) as "preflightAllowed",
      identity.id is not null as "deviceIdentityPresent",
      approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.status as "approvalStatus",
      approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",approval.action as "approvalAction",
      coalesce(approval.expires_at>now(),false) as "approvalUnexpired",
      coalesce((approval.context_json#>>'{preflight,allowed}')::boolean,false) as "approvalPreflightAllowed",
      wayline.project_id as "waylineProjectId",wayline.connector_instance_id as "waylineConnectorId",wayline.resource_kind as "waylineKind",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",target.resource_kind as "targetKind",
      target.canonical_target_id as "targetTaskRunId"
    from task_runs run
    join device_adapters adapter on adapter.id=sqlc.arg(p4) and adapter.project_id=run.project_id
	join devices device on device.id=run.selected_device_id and device.project_id=run.project_id
    join team_members member on member.team_id=run.team_id and member.user_id=sqlc.arg(p8)
    left join project_feature_flags flags on flags.project_id=run.project_id
    left join device_external_identities identity on identity.project_id=run.project_id
      and identity.adapter_id=adapter.id and identity.device_id=run.selected_device_id
    left join approval_requests approval on approval.id=sqlc.arg(p5) and approval.project_id=run.project_id
    left join connector_remote_resources wayline on wayline.id=sqlc.arg(p6) and wayline.project_id=run.project_id
      and wayline.status='active'
    left join connector_remote_resources target on target.id=sqlc.arg(p7) and target.project_id=run.project_id
      and target.status='active'
    where run.project_id=sqlc.arg(p1) and run.id=sqlc.arg(p2)
) r;

-- name: FHFlightLanding :many
SELECT to_jsonb(r) FROM (
select 1 from device_external_identities
        where project_id=sqlc.arg(p1) and adapter_id=sqlc.arg(p2) and device_id=sqlc.arg(p3)
) r;

-- name: FHFlightInsert :one
insert into connector_action_jobs(
        id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,target_resource_id,
        approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json
      ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13),sqlc.arg(p14))
      on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing
      returning id::text,status;

-- name: FHFlightExisting :many
SELECT to_jsonb(r) FROM (
select id::text,status,request_digest as "requestDigest",task_run_id as "taskRunId",device_id as "deviceId",
          approval_request_id::text as "approvalRequestId",requested_by_user_id as "requestedByUserId",
          wayline_resource_id as "waylineResourceId",target_resource_id as "targetResourceId"
        from connector_action_jobs where project_id=sqlc.arg(p1) and connector_instance_id=sqlc.arg(p2) and action_kind=sqlc.arg(p3) and idempotency_key=sqlc.arg(p4)
) r;

-- name: FHFlightEnqueue :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.flight_action.requested','connector_action_job',sqlc.arg(p4),sqlc.arg(p5),16)
       on conflict(event_id) do nothing;

-- name: FHFlightRead :many
SELECT to_jsonb(r) FROM (
select id::text,task_run_id as "taskRunId",action_kind as action,status,
      attempt_count as "attemptCount",reconciliation_count as "reconciliationCount",
      last_error_code as "lastErrorCode",accepted_at as "acceptedAt",reconciled_at as "reconciledAt",
      unknown_at as "unknownAt",completed_at as "completedAt",created_at as "createdAt"
    from connector_action_jobs where id=sqlc.arg(p1)::uuid and project_id=sqlc.arg(p2) and connector_instance_id=sqlc.arg(p3)
) r;
