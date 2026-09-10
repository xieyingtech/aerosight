-- name: FHMemberTarget :many
SELECT to_jsonb(r) FROM (
select project.team_id::int as "teamId",
      (member.role='owner' or exists(select 1 from project_permissions permission where permission.project_id=project.id
        and permission.team_id=project.team_id and permission.user_id=sqlc.arg(p4) and permission.permission='organization:manage')) as "managementGranted",
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",adapter.status as "connectorStatus",
      project.name as "projectName",coalesce(organization.summary_json->>'name','') as "organizationName",
      (select count(*)::int from connector_remote_resources target where target.project_id=adapter.project_id
        and target.connector_instance_id=adapter.id and target.resource_kind='organization-user' and target.status='active'
        and target.remote_id=any(sqlc.arg(p5)::text[])) as "targetCount",
      coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(sqlc.arg(p6)::text,true),false) as "featureEnabled",
      exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
        and capability.connector_instance_id=adapter.id and capability.capability_code=sqlc.arg(p7) and capability.status='supported'
		and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		and capability.region='cn' and capability.deployment='cn-public-cloud'
        and capability.evidence_level='field-write' and capability.device_model is null and capability.firmware_version is null
        and (capability.expires_at is null or capability.expires_at>now())) as "capabilityVerified",
      approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.resource_type as "approvalResourceType",
      approval.resource_id as "approvalResourceId",approval.action as "approvalAction",approval.status as "approvalStatus",
      coalesce(approval.expires_at>now(),false) as "approvalUnexpired",approval.context_json->>'previewDigest' as "approvalPreviewDigest"
    from projects project join team_members member on member.team_id=project.team_id and member.user_id=sqlc.arg(p4)
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id
      and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    left join project_feature_flags flags on flags.project_id=project.id
    left join connector_remote_resources organization on organization.project_id=adapter.project_id
      and organization.connector_instance_id=adapter.id and organization.resource_kind='organization' and organization.status='active'
    left join approval_requests approval on approval.id=sqlc.narg(p8)::uuid and approval.project_id=project.id
    where project.id=sqlc.arg(p1) and adapter.id=sqlc.arg(p2) and adapter.team_id=sqlc.arg(p3)
) r;

-- name: FHMemberInsert :one
insert into connector_management_write_jobs(
        id,project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,
        feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json)
      values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),'project-member-upsert',sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13))
      on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text,status;

-- name: FHMemberExisting :many
SELECT to_jsonb(r) FROM (
select id::text,status,request_digest as "requestDigest",requested_by_user_id as "requestedByUserId"
          from connector_management_write_jobs where project_id=sqlc.arg(p1) and connector_instance_id=sqlc.arg(p2)
            and action_kind='project-member-upsert' and idempotency_key=sqlc.arg(p3)
) r;

-- name: FHMemberEnqueue :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
      values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.management_write.requested','connector_management_write_job',sqlc.arg(p4),sqlc.arg(p5),8)
      on conflict(event_id) do nothing;

-- name: FHMemberGrant :many
SELECT to_jsonb(r) FROM (
select 1 from project_permissions where project_id=sqlc.arg(p1) and team_id=sqlc.arg(p2)
    and user_id=sqlc.arg(p3) and permission='organization:manage'
) r;

-- name: FHMemberRead :many
SELECT to_jsonb(r) FROM (
select id::text,action_kind as action,capability_code as "capabilityCode",preview_digest as "previewDigest",
      status,attempt_count as "attemptCount",reconciliation_count as "reconciliationCount",last_error_code as "lastErrorCode",
      result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",completed_at as "completedAt",
      created_at as "createdAt",updated_at as "updatedAt"
    from connector_management_write_jobs where id=sqlc.arg(p1)::uuid and project_id=sqlc.arg(p2) and connector_instance_id=sqlc.arg(p3)
) r;
