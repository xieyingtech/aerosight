-- name: ReadFHFlightOpsAccess :many
SELECT to_jsonb(r) FROM (
select project.id::int as "projectId",project.team_id::int as "teamId",membership.role,
             (membership.role in('owner','admin') or exists(
               select 1 from project_permissions permission
                where permission.project_id=project.id and permission.team_id=project.team_id
                  and permission.user_id=membership.user_id and permission.permission='mission:operate'
             )) as "canOperate"
        from projects project
        join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(user_id)
       where project.id=sqlc.arg(project_id)
) r;

-- name: ReadFHFlightOpsConnectors :many
SELECT to_jsonb(r) FROM (
select adapter.id::text,adapter.name,adapter.status,adapter.last_checked_at as "lastCheckedAt",
             coalesce(flags.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb,false) as "actionEnabled",
             exists(select 1 from connector_capability_snapshots capability
               where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
                 and capability.capability_code='flight.execute' and capability.status='supported'
				 and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
				 and capability.region='cn' and capability.deployment='cn-public-cloud'
				 and capability.evidence_level='field-write'
				 and exists(select 1 from device_external_identities accepted_identity
				   join devices accepted_device on accepted_device.id=accepted_identity.device_id
				     and accepted_device.project_id=accepted_identity.project_id
				   where accepted_identity.project_id=adapter.project_id and accepted_identity.adapter_id=adapter.id
				     and accepted_identity.discovery_status='managed' and accepted_device.device_model=capability.device_model
				     and accepted_device.firmware_version=capability.firmware_version)
                 and (capability.expires_at is null or capability.expires_at>now())) as "actionVerified"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join project_feature_flags flags on flags.project_id=adapter.project_id
       where adapter.project_id=sqlc.arg(project_id) and adapter.team_id=sqlc.arg(team_id)
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
       order by adapter.updated_at desc limit 50
) r;

-- name: ReadFHFlightOpsWaylines :many
SELECT to_jsonb(r) FROM (
select resource.id::text,resource.connector_instance_id::text as "connectorId",task.id::int as "taskId",
             resource.summary_json->>'name' as name,resource.status,
             resource.summary_json->>'deviceModelKey' as "deviceModelKey",
             resource.summary_json->'templateTypes' as "templateTypes",
             resource.summary_json->>'payloadCount' as "payloadCount",
             resource.summary_json->>'sizeBytes' as "sizeBytes",
             resource.remote_updated_at as "remoteUpdatedAt",resource.last_seen_at as "lastSeenAt"
        from connector_remote_resources resource
        join device_adapters adapter on adapter.id=resource.connector_instance_id and adapter.project_id=resource.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join tasks task on resource.canonical_target_type='task'
          and resource.canonical_target_id=task.id::text and task.project_id=resource.project_id
       where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id) and resource.resource_kind='wayline'
         and definition.connector_key='dji.flighthub2'
       order by resource.remote_updated_at desc nulls last,resource.id desc limit 250
) r;

-- name: ReadFHFlightOpsTaskRuns :many
SELECT to_jsonb(r) FROM (
select resource.id::text,resource.connector_instance_id::text as "connectorId",run.id::int as "taskRunId",
             task.id::int as "taskId",task.name as "taskName",device.id::int as "deviceId",device.name as "deviceName",
             run.status,run.state_reason as "stateReason",resource.summary_json->>'taskType' as "taskType",
             resource.summary_json->>'mediaUploadStatus' as "mediaUploadStatus",
             resource.summary_json->>'resumableStatus' as "resumableStatus",
             case when resource.summary_json ? 'breakPointResume' then (resource.summary_json->>'breakPointResume')::boolean end as "breakPointResume",
             resource.summary_json->>'currentWaypoint' as "currentWaypoint",
             resource.summary_json->>'totalWaypoints' as "totalWaypoints",
             resource.summary_json->>'exceptionCount' as "exceptionCount",
             run.started_at as "startedAt",run.finished_at as "finishedAt",resource.remote_updated_at as "remoteUpdatedAt"
        from connector_remote_resources resource
        join task_runs run on resource.canonical_target_type='task_run'
          and resource.canonical_target_id=run.id::text and run.project_id=resource.project_id
        join tasks task on task.id=run.task_id and task.project_id=run.project_id
        left join devices device on device.id=run.selected_device_id and device.project_id=run.project_id
       where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id) and resource.resource_kind='flight-task'
       order by coalesce(resource.remote_updated_at,run.created_at) desc limit 250
) r;

-- name: ReadFHFlightOpsTracks :many
SELECT to_jsonb(r) FROM (
select run.id::int as id,observation.adapter_id::text as "connectorId",run.id::int as "taskRunId",
             task.name as "taskName",device.id::int as "deviceId",device.name as "deviceName",
             count(*)::int as "pointCount",min(observation.captured_at) as "firstCapturedAt",
             max(observation.captured_at) as "lastCapturedAt"
        from observations observation
        join task_runs run on run.id=observation.task_run_id and run.project_id=observation.project_id
        join tasks task on task.id=run.task_id and task.project_id=run.project_id
        join devices device on device.id=observation.device_id and device.project_id=observation.project_id
        join device_adapters adapter on adapter.id=observation.adapter_id and adapter.project_id=observation.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
       where observation.project_id=sqlc.arg(project_id) and observation.team_id=sqlc.arg(team_id) and observation.task_run_id is not null
         and definition.connector_key='dji.flighthub2'
       group by run.id,observation.adapter_id,task.name,device.id,device.name
       order by max(observation.captured_at) desc limit 250
) r;

-- name: ReadFHFlightOpsAssets :many
SELECT to_jsonb(r) FROM (
select resource.id::text,resource.connector_instance_id::text as "connectorId",asset.id::int as "assetId",
             asset.task_run_id::int as "taskRunId",asset.device_id::int as "deviceId",resource.summary_json->>'name' as name,
             asset.kind,asset.mime_type as "mimeType",asset.status,
             resource.summary_json->>'fileType' as "fileType",resource.summary_json->>'suffix' as suffix,
             coalesce(resource.summary_json->>'sizeBytes',asset.size_bytes::text) as "sizeBytes",
             resource.summary_json->>'contentType' as "contentType",resource.summary_json->>'progress' as progress,
             resource.summary_json->'fileTypes' as "fileTypes",resource.summary_json->>'failedReasonCode' as "failedReasonCode",
             asset.captured_at as "capturedAt",asset.created_at as "createdAt"
        from connector_remote_resources resource
        join assets asset on resource.canonical_target_type='asset'
          and resource.canonical_target_id=asset.id::text and asset.project_id=resource.project_id
       where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id) and resource.resource_kind=sqlc.arg(resource_kind)
       order by coalesce(asset.captured_at,asset.created_at) desc limit 250
) r;

-- name: ReadFHFlightOpsAlerts :many
SELECT to_jsonb(r) FROM (
select resource.id::text,resource.connector_instance_id::text as "connectorId",resource.resource_kind as kind,
             coalesce(run.id,(resource.summary_json->>'taskRunId')::int)::int as "taskRunId",
             event.id::text as "perceptionEventId",issue.id::int as "issueId",
             coalesce(event.title,issue.title,resource.summary_json->>'taskName',resource.summary_json->>'label') as title,
             event.severity,event.status,resource.summary_json->>'alertCount' as "alertCount",
             resource.summary_json->>'confidence' as confidence,
             case when resource.summary_json ? 'hasMedia' then (resource.summary_json->>'hasMedia')::boolean end as "hasMedia",
             coalesce(resource.remote_updated_at,event.last_detected_at) as "occurredAt",resource.last_seen_at as "lastSeenAt"
        from connector_remote_resources resource
        left join task_runs run on resource.canonical_target_type='task_run'
          and resource.canonical_target_id=run.id::text and run.project_id=resource.project_id
        left join perception_events event on resource.canonical_target_type='perception_event'
          and resource.canonical_target_id=event.id::text and event.project_id=resource.project_id
        left join issue_links issue_link on issue_link.project_id=resource.project_id
          and issue_link.link_type='perception_event' and issue_link.target_id=event.id::text
        left join issues issue on issue.id=issue_link.issue_id and issue.project_id=resource.project_id
       where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id) and resource.resource_kind in('flight-alert','ai-alert')
       order by coalesce(resource.remote_updated_at,resource.last_seen_at) desc limit 250
) r;

-- name: ReadFHFlightOpsActions :many
SELECT to_jsonb(r) FROM (
select job.id::text,job.connector_instance_id::text as "connectorId",job.task_run_id::int as "taskRunId",
             job.device_id::int as "deviceId",job.wayline_resource_id::text as "waylineResourceId",
             job.target_resource_id::text as "targetResourceId",job.remote_result_resource_id::text as "resultResourceId",
             job.action_kind as action,job.status,job.attempt_count::int as "attemptCount",
             job.reconciliation_count::int as "reconciliationCount",job.last_error_code as "lastErrorCode",
             job.accepted_at as "acceptedAt",job.reconciled_at as "reconciledAt",job.unknown_at as "unknownAt",
             job.completed_at as "completedAt",job.created_at as "createdAt",job.updated_at as "updatedAt"
        from connector_action_jobs job
       where job.project_id=sqlc.arg(project_id) and job.team_id=sqlc.arg(team_id)
       order by job.updated_at desc limit 250
) r;
