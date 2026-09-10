-- name: ReadFHModelsAccess :many
SELECT to_jsonb(r) FROM (
select project.id::int as "projectId",project.team_id::int as "teamId",membership.role,
        (membership.role in('owner','admin') or exists(select 1 from project_permissions permission
          where permission.project_id=project.id and permission.team_id=project.team_id
            and permission.user_id=sqlc.arg(user_id) and permission.permission='mission:operate')) as "canOperate"
      from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(user_id)
      where project.id=sqlc.arg(project_id)
) r;

-- name: ReadFHModelsConnectors :many
SELECT to_jsonb(r) FROM (
select adapter.project_id::int as "projectId",adapter.id::text,adapter.name,adapter.status,
        adapter.last_checked_at as "lastCheckedAt" from device_adapters adapter
      join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=sqlc.arg(project_id) and adapter.team_id=sqlc.arg(team_id) and definition.connector_key='dji.flighthub2'
) r;

-- name: ReadFHModelsActions :many
SELECT to_jsonb(r) FROM (
select adapter.project_id::int as "projectId",adapter.id::text as "connectorId",policy.action,
        coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(policy.flag,true),false) as "flagEnabled",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code=policy.capability
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
          and capability.status='supported' and capability.evidence_level='field-write'
          and (capability.expires_at is null or capability.expires_at>now())
          and capability.device_model is null and capability.firmware_version is null) as "capabilityVerified"
      from device_adapters adapter cross join (SELECT 'traditional-create'::text AS action,'model.write'::text AS capability,'model.write'::text AS flag UNION ALL SELECT 'open-start'::text AS action,'model.write'::text AS capability,'model.write'::text AS flag UNION ALL SELECT 'open-stop'::text AS action,'model.write'::text AS capability,'model.write'::text AS flag UNION ALL SELECT 'open-upload'::text AS action,'model.write'::text AS capability,'model.write'::text AS flag UNION ALL SELECT 'model-delete'::text AS action,'model.delete'::text AS capability,'flighthub.model.delete'::text AS flag UNION ALL SELECT 'model-resource-delete'::text AS action,'model.resource.delete'::text AS capability,'flighthub.model-resource.delete'::text AS flag) policy
      left join project_feature_flags flags on flags.project_id=adapter.project_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id
      where adapter.project_id=sqlc.arg(project_id) and adapter.team_id=sqlc.arg(team_id) and definition.connector_key='dji.flighthub2'
) r;

-- name: ReadFHModelsSync :many
SELECT to_jsonb(r) FROM (
select state.project_id::int as "projectId",state.connector_instance_id::text as "connectorId",state.status,
        state.last_error_code as "lastErrorCode",state.last_succeeded_at as "lastSucceededAt",state.next_attempt_at as "nextAttemptAt"
      from connector_resource_sync_states state where state.project_id=sqlc.arg(project_id) and state.team_id=sqlc.arg(team_id) and state.resource_kind='models'
) r;

-- name: ReadFHModelsResources :many
SELECT to_jsonb(r) FROM (
select resource.project_id::int as "projectId",resource.id::text,resource.connector_instance_id::text as "connectorId",
        resource.resource_kind as kind,resource.status,resource.summary_json->>'name' as name,
        resource.summary_json->>'fileType' as "fileType",resource.summary_json->>'showOnMap' as "showOnMap",
        resource.summary_json->>'sizeBytes' as "sizeBytes",resource.summary_json->>'modelType' as "modelType",
        resource.summary_json->>'modelStatus' as "modelStatus",
        resource.summary_json->>'reconstructionProgress' as "reconstructionProgress",
        resource.summary_json->>'errorCode' as "errorCode",resource.summary_json->>'zipStatus' as "zipStatus",
        resource.summary_json->>'zipProgress' as "zipProgress",resource.summary_json->>'resourceStatus' as "resourceStatus",
        resource.summary_json->>'fileCount' as "fileCount",asset.id::text as "assetId",asset.kind as "assetKind",
        asset.status as "assetStatus",asset.failure_code as "assetFailureCode",
        resource.remote_updated_at as "remoteUpdatedAt",resource.last_seen_at as "lastSeenAt"
      from connector_remote_resources resource left join assets asset on resource.canonical_target_type='asset'
        and asset.project_id=resource.project_id and asset.id::text=resource.canonical_target_id
      where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id) and resource.resource_kind in('model','model-resource')
      order by resource.last_seen_at desc,resource.id desc
) r;

-- name: ReadFHModelsJobs :many
SELECT to_jsonb(r) FROM (
select job.project_id::int as "projectId",job.id::text,job.connector_instance_id::text as "connectorId",
        'reconstruction'::text as "jobType",job.action_kind as action,job.status,job.progress,job.stage,
        job.submit_attempt_count as "attemptCount",job.reconciliation_count as "reconciliationCount",
        job.last_error_code as "lastErrorCode",job.asset_ids_json as "assetIds",job.created_at as "createdAt",job.updated_at as "updatedAt"
      from connector_model_jobs job where job.project_id=sqlc.arg(project_id) and job.team_id=sqlc.arg(team_id)
      union all select upload.project_id::int,upload.id::text,upload.connector_instance_id::text,'upload',
        'open-upload',upload.status,null,null,upload.callback_attempt_count,upload.reconciliation_count,
        upload.last_error_code,case when upload.asset_id is null then '[]'::jsonb else jsonb_build_array(upload.asset_id) end,
        upload.created_at,upload.updated_at from connector_open_model_uploads upload where upload.project_id=sqlc.arg(project_id) and upload.team_id=sqlc.arg(team_id)
      union all select deletion.project_id::int,deletion.id::text,deletion.connector_instance_id::text,'deletion',
        deletion.action_kind,deletion.status,null,null,deletion.attempt_count,deletion.reconciliation_count,
        deletion.last_error_code,'[]'::jsonb,deletion.created_at,deletion.updated_at
        from connector_model_delete_jobs deletion where deletion.project_id=sqlc.arg(project_id) and deletion.team_id=sqlc.arg(team_id)
      order by "updatedAt" desc
) r;
