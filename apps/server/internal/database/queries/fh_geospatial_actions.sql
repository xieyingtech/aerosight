-- name: FHGeospatialAuthorize :many
SELECT to_jsonb(r) FROM (
select sqlc.arg(p3)::int as "teamId",member.role,
      (member.role in('owner','admin') or exists(select 1 from project_permissions permission
        where permission.project_id=adapter.project_id and permission.team_id=adapter.team_id
          and permission.user_id=member.user_id and permission.permission='mission:operate')) as "hasOperatePermission",
      adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(sqlc.arg(p7)::text,true),false) as "actionEnabled",
      exists(select 1 from connector_capability_snapshots capability
        where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
          and capability.capability_code=sqlc.arg(p6) and capability.status='supported'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
          and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
          and capability.device_model is null and capability.firmware_version is null) as "capabilityFieldVerified",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",
      target.resource_kind as "targetKind",target.status as "targetStatus",target.remote_version as "targetRemoteVersion"
     from device_adapters adapter
     join team_members member on member.team_id=adapter.team_id and member.user_id=sqlc.arg(p4)
     left join project_feature_flags flags on flags.project_id=adapter.project_id
     left join connector_remote_resources target on target.id=sqlc.arg(p5) and target.project_id=adapter.project_id
       and target.connector_instance_id=adapter.id
     where adapter.id=sqlc.arg(p2) and adapter.project_id=sqlc.arg(p1) and adapter.team_id=sqlc.arg(p3)
) r;

-- name: FHGeospatialInsert :one
insert into connector_geospatial_action_jobs(
        id,project_id,team_id,connector_instance_id,target_resource_id,requested_by_user_id,
        action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,request_digest,request_envelope_json
      ) values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),sqlc.arg(p4),sqlc.arg(p5),sqlc.arg(p6),sqlc.arg(p7),sqlc.arg(p8),sqlc.arg(p9),sqlc.arg(p10),sqlc.arg(p11),sqlc.arg(p12),sqlc.arg(p13))
      on conflict do nothing returning id::text,status;

-- name: FHGeospatialExisting :many
SELECT to_jsonb(r) FROM (
select id::text,status,request_digest as "requestDigest",target_resource_id as "targetResourceId",
          expected_remote_version as "expectedRemoteVersion",requested_by_user_id as "requestedByUserId"
         from connector_geospatial_action_jobs
         where project_id=sqlc.arg(p1) and connector_instance_id=sqlc.arg(p2) and action_kind=sqlc.arg(p3) and idempotency_key=sqlc.arg(p4)
) r;

-- name: FHGeospatialEnqueue :exec
insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values(sqlc.arg(p1),sqlc.arg(p2),sqlc.arg(p3),'flighthub.geospatial_action.requested','connector_geospatial_action_job',sqlc.arg(p4),sqlc.arg(p5),8)
       on conflict(event_id) do nothing;

-- name: FHGeospatialRead :many
SELECT to_jsonb(r) FROM (
select id::text,action_kind as action,capability_code as "capabilityCode",target_resource_id as "targetResourceId",
      expected_remote_version as "expectedRemoteVersion",status,attempt_count as "attemptCount",last_error_code as "lastErrorCode",
      result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",completed_at as "completedAt",
      created_at as "createdAt",updated_at as "updatedAt"
     from connector_geospatial_action_jobs where id=sqlc.arg(p1)::uuid and project_id=sqlc.arg(p2) and connector_instance_id=sqlc.arg(p3)
) r;
