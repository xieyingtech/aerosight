-- name: FHControlLockDevice :many
SELECT to_jsonb(r) FROM (
select id from devices where project_id=sqlc.arg(p1) and id=sqlc.arg(p2) for update
) r;

-- name: FHControlExisting :many
SELECT to_jsonb(r) FROM (
select id::text,status,holder_user_id as "holderUserId",connector_instance_id::text as "connectorInstanceId",
        approval_request_id::text as "approvalRequestId",safety_policy_version_id::text as "safetyPolicyVersionId",
        controls_json=sqlc.arg(p4)::jsonb as "controlsMatch" from connector_control_sessions
       where project_id=sqlc.arg(p1) and device_id=sqlc.arg(p2) and idempotency_key=sqlc.arg(p3)
) r;

-- name: FHControlAuthorize :many
SELECT to_jsonb(r) FROM (
select adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",
        device.project_id as "deviceProjectId",adapter.status as "connectorStatus",
        coalesce(flags.flighthub_action_flags_json @> '{"device.control":true}'::jsonb,false) as "featureEnabled",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code='device.control'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
          and capability.status='supported' and capability.evidence_level='field-write'
          and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
        device.status='online' as "deviceOnline",latest.captured_at as "stateCapturedAt",
        project.current_safety_policy_version_id::text as "currentSafetyPolicyVersionId",
        approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",
        approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",
        approval.action as "approvalAction",approval.status as "approvalStatus",
        coalesce(approval.expires_at>now(),false) as "approvalUnexpired",
        (select count(*)::int from connector_control_sessions session where session.project_id=device.project_id
          and session.device_id=device.id and session.status in('requested','acquiring','active','releasing')) as "conflictingSessionCount"
       from device_adapters adapter
       join projects project on project.id=adapter.project_id and project.team_id=adapter.team_id
       join devices device on device.project_id=adapter.project_id and device.id=sqlc.arg(p2)
       join device_external_identities identity on identity.project_id=adapter.project_id and identity.adapter_id=adapter.id
         and identity.device_id=device.id
       left join project_feature_flags flags on flags.project_id=adapter.project_id
       left join device_latest_telemetry latest on latest.project_id=device.project_id and latest.device_id=device.id and latest.adapter_id=adapter.id
       left join approval_requests approval on approval.id::text=sqlc.arg(p4) and approval.project_id=adapter.project_id
       where adapter.project_id=sqlc.arg(p1) and adapter.id=sqlc.arg(p3)
) r;

-- name: FHControlInsert :one
insert into connector_control_sessions(
        id,project_id,team_id,connector_instance_id,device_id,holder_user_id,approval_request_id,
        safety_policy_version_id,idempotency_key,controls_json,last_heartbeat_at,lease_expires_at,
        absolute_expires_at,operation_window_started_at
      ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7)::uuid,sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10)::jsonb,sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13),sqlc.arg(p11)) returning id::text,status;

-- name: FHControlEnqueueAcquire :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.control_session.reconcile','connector_control_session',sqlc.arg(p4),sqlc.arg(p5),8)
       on conflict(event_id) do nothing;

-- name: FHControlHeartbeat :one
update connector_control_sessions set last_heartbeat_at=sqlc.arg(p3),
      lease_expires_at=least(absolute_expires_at,sqlc.arg(p3)+(sqlc.arg(p4)*interval '1 millisecond')),updated_at=now()
     where project_id=sqlc.arg(p1) and id=sqlc.arg(p2)::uuid and device_id=sqlc.arg(p7) and holder_user_id=sqlc.arg(p5) and status='active'
       and lease_expires_at>sqlc.arg(p3) and absolute_expires_at>sqlc.arg(p3)
       and last_heartbeat_at<=sqlc.arg(p3)-(sqlc.arg(p6)*interval '1 millisecond')
     returning id::text,status,lease_expires_at as "leaseExpiresAt";

-- name: FHControlClaim :one
update connector_control_sessions set
      operation_count=case when operation_window_started_at<=now()-interval '1 second' then 1 else operation_count+1 end,
      operation_window_started_at=case when operation_window_started_at<=now()-interval '1 second' then now() else operation_window_started_at end,
      last_operation_at=now(),updated_at=now()
     where project_id=sqlc.arg(p1) and id=sqlc.arg(p2)::uuid and device_id=sqlc.arg(p3) and holder_user_id=sqlc.arg(p4) and status='active'
       and lease_expires_at>now() and absolute_expires_at>now()
       and (operation_window_started_at<=now()-interval '1 second' or operation_count<sqlc.arg(p5))
     returning id;

-- name: FHControlLockSession :many
SELECT to_jsonb(r) FROM (
select status,holder_user_id as "holderUserId" from connector_control_sessions
       where project_id=sqlc.arg(p1) and device_id=sqlc.arg(p3) and id=sqlc.arg(p2)::uuid for update
) r;

-- name: FHControlRelease :exec
update connector_control_sessions set status='releasing',release_requested_at=coalesce(release_requested_at,now()),updated_at=now()
      where project_id=sqlc.arg(p1) and id=sqlc.arg(p2)::uuid;

-- name: FHControlEnqueueRelease :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.control_session.reconcile','connector_control_session',sqlc.arg(p4),sqlc.arg(p5),8)
       on conflict(event_id) do nothing;
