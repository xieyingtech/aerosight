-- name: FHDeviceAdminAuthorize :many
SELECT to_jsonb(r) FROM (
select sqlc.arg(p3)::int as "teamId",member.role,adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",
    adapter.status as "connectorStatus",coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(sqlc.arg(p7)::text,true),false) as "featureEnabled",
    exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
      and capability.connector_instance_id=adapter.id and capability.capability_code=sqlc.arg(p6) and capability.status='supported'
	  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
	  and capability.region='cn' and capability.deployment='cn-public-cloud'
      and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
      and ((sqlc.narg(p5)::int is null and capability.device_model is null and capability.firmware_version is null)
        or (sqlc.narg(p5)::int is not null and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version))) as "capabilityVerified",
    device.project_id as "deviceProjectId",identity.id is not null as "identityPresent",coalesce(device.status='online',false) as "deviceOnline",
    coalesce(latest.captured_at>now()-interval '30 seconds' and latest.captured_at<=now()+interval '1 second',false) as "stateFresh",
    approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.resource_type as "approvalResourceType",
    approval.resource_id as "approvalResourceId",approval.action as "approvalAction",approval.status as "approvalStatus",
    coalesce(approval.expires_at>now(),false) as "approvalUnexpired"
   from device_adapters adapter join team_members member on member.team_id=adapter.team_id and member.user_id=sqlc.arg(p4)
   left join project_feature_flags flags on flags.project_id=adapter.project_id
   left join devices device on device.id=sqlc.narg(p5) and device.project_id=adapter.project_id
   left join device_external_identities identity on identity.project_id=adapter.project_id and identity.adapter_id=adapter.id and identity.device_id=device.id
   left join device_latest_telemetry latest on latest.project_id=adapter.project_id and latest.adapter_id=adapter.id and latest.device_id=device.id
   left join approval_requests approval on approval.id=sqlc.arg(p8)::uuid and approval.project_id=adapter.project_id
   where adapter.id=sqlc.arg(p2) and adapter.project_id=sqlc.arg(p1) and adapter.team_id=sqlc.arg(p3)
) r;

-- name: FHDeviceAdminInsert :one
insert into connector_device_admin_jobs(
      id,project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,
      capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json
    ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13)) on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text,status;

-- name: FHDeviceAdminExisting :many
SELECT to_jsonb(r) FROM (
select id::text,status,request_digest as "requestDigest",requested_by_user_id as "requestedByUserId" from connector_device_admin_jobs where project_id=sqlc.arg(p1) and connector_instance_id=sqlc.arg(p2) and action_kind=sqlc.arg(p3) and idempotency_key=sqlc.arg(p4)
) r;

-- name: FHDeviceAdminEnqueue :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.device_admin.requested','connector_device_admin_job',sqlc.arg(p4),sqlc.arg(p5),8) on conflict(event_id) do nothing;

-- name: FHDeviceAdminRead :many
SELECT to_jsonb(r) FROM (
select action_kind as action,status,attempt_count as "attemptCount",last_error_code as "lastErrorCode",result_json as result,result_envelope_json as "resultEnvelope" from connector_device_admin_jobs where id=sqlc.arg(p1)::uuid and project_id=sqlc.arg(p2) and connector_instance_id=sqlc.arg(p3)
) r;
