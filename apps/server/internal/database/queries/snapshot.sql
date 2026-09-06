-- name: SnapshotProjectScope :one
select project.id, project.name, project.team_id as "teamId", membership.role,
              coalesce(flags.dependency_health_json, '{}'::jsonb) as "dependencyHealth"
       from projects project
       join team_members membership on membership.team_id = project.team_id
       left join project_feature_flags flags on flags.project_id = project.id
       where project.id = $1 and membership.user_id = $2;

-- name: SnapshotDevices :many
select to_jsonb(snapshot_row) as item from (
select device.id, device.name, device.type, device.status,
              device.device_type_id::text as "deviceTypeId",
              device_type.type_key as "typeKey", device_type.version as "typeVersion",
              device_type.display_name as "typeName", device_type.category,
              driver.driver_key as "driverKey", driver.version as "driverVersion",
              driver.status as "driverStatus",
              device.status_reason as "statusReason", device.last_seen_at as "lastSeenAt",
              coalesce((select jsonb_agg(jsonb_build_object(
                'code',capability.capability_code,'availability',capability.availability,
                'reason',capability.availability_reason,'risk',capability.risk_level
              ) order by capability.capability_code) from device_capabilities capability
                where capability.project_id=device.project_id and capability.device_id=device.id),'[]') as "rawCapabilities",
              coalesce((select jsonb_agg(jsonb_build_object(
                'stableChannelId',channel.stable_channel_id,'capabilityCode',channel.capability_code,'channelKey',channel.channel_key,
                'displayName',channel.display_name,'dataType',channel.data_type,
                'availability',channel.availability,'availabilityReason',channel.availability_reason,
                'protocol',channel.protocol
              ) order by channel.channel_key) from device_stream_channels channel
                where channel.project_id=device.project_id and channel.device_id=device.id),'[]') as "rawChannels",
              case when pose.observation_id is null then null else json_build_object(
                'longitude', ST_X(pose.standard_position),
                'latitude', ST_Y(pose.standard_position),
                'altitudeMeters', ST_Z(pose.standard_position),
                'capturedAt', pose.captured_at,
                'spatialQuality', pose.spatial_quality,
                'horizontalAccuracyMeters', pose.horizontal_accuracy_m
              ) end as pose
       from devices device
       join device_types device_type on device_type.id = device.device_type_id
       join driver_definitions driver on driver.id = device_type.driver_definition_id
       left join lateral (
         select observation_id, standard_position, captured_at, spatial_quality, horizontal_accuracy_m
         from poses where poses.project_id = $1 and poses.device_id = device.id
         order by captured_at desc limit 1
       ) pose on true
       where device.project_id = $1 order by device.id
) snapshot_row;

-- name: SnapshotDeviceGrants :many
select to_jsonb(snapshot_row) as item from (
select scope_type as "scopeType",device_type_id::text as "deviceTypeId",device_id as "deviceId",
              action_pattern as "actionPattern",effect
         from device_capability_grants
        where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())
) snapshot_row;

-- name: SnapshotActiveTasks :many
select to_jsonb(snapshot_row) as item from (
select run.id, run.status, run.started_at as "startedAt", run.created_at as "createdAt",
              task.id as "taskId", task.name as "taskName", run.input_snapshot_json as input
       from task_runs run join tasks task on task.id = run.task_id and task.project_id = run.project_id
       where run.project_id = $1 and run.status in ('queued', 'blocked', 'ready', 'dispatching', 'running', 'paused', 'canceling')
       order by run.created_at desc
) snapshot_row;

-- name: SnapshotTracks :many
select to_jsonb(snapshot_row) as item from (
select recent.device_id as "deviceId", min(recent.captured_at) as "startedAt",
              max(recent.captured_at) as "endedAt", count(*)::int as "pointCount",
              ST_AsGeoJSON(ST_MakeLine(recent.standard_position order by recent.captured_at))::json as geometry
       from (
         select pose.device_id, pose.captured_at, pose.standard_position
         from poses pose where pose.project_id = $1 and pose.standard_position is not null
         order by pose.captured_at desc limit 5000
       ) recent
       group by recent.device_id having count(*) >= 2
) snapshot_row;

-- name: SnapshotLiveStreams :many
select to_jsonb(snapshot_row) as item from (
select stream.id::text as id, stream.device_id as "deviceId", stream.stream_key as "streamKey",
              stream.source_type as "sourceType", stream.status,
              stream.status_reason as "statusReason", stream.started_at as "startedAt",
              stream.last_active_at as "lastActiveAt", stream.ended_at as "endedAt"
       from live_streams stream
       where stream.project_id = $1
         and stream.status in ('requested', 'starting', 'live', 'degraded', 'stopping')
       order by stream.started_at desc
) snapshot_row;

-- name: SnapshotRealtimeChannels :many
select to_jsonb(snapshot_row) as item from (
select channel.stable_channel_id as "stableChannelId",channel.device_id as "deviceId",
              channel.channel_key as "channelKey",channel.display_name as "displayName",
              channel.data_type as "dataType",channel.schema_json as schema,channel.unit,
              channel.quality_json as quality,channel.availability,channel.availability_reason as "availabilityReason",
              telemetry.payload_json as "latestPayload",telemetry.captured_at as "latestCapturedAt",
              telemetry.quality_json as "latestQuality"
         from device_stream_channels channel
         left join device_latest_telemetry telemetry
           on telemetry.project_id=channel.project_id and telemetry.device_id=channel.device_id
        where channel.project_id=$1 order by channel.device_id,channel.channel_key
) snapshot_row;

-- name: SnapshotDiagnostics :many
select to_jsonb(snapshot_row) as item from (
select 'command:'||command.id::text as id,command.device_id as "deviceId",'command'::text as kind,
              case when command.status in ('unknown','timed_out','nacked') then 'error' else 'warning' end as severity,
              device.name||' · '||command.capability_code as title,
              coalesce(command.result_json->>'reason',command.result_json->>'errorCode',command.status) as reason,
              command.status,coalesce(command.completed_at,command.created_at) as "occurredAt"
         from device_commands command join devices device on device.id=command.device_id and device.project_id=command.project_id
        where command.project_id=$1 and command.status in ('nacked','timed_out','unknown')
       union all
       select 'connection:'||adapter.id::text,null::integer,'connection',case when adapter.status='failed' then 'error' else 'warning' end,
              adapter.name,coalesce(adapter.last_health_json->>'code',adapter.status),adapter.status,
              coalesce(adapter.last_checked_at,adapter.updated_at)
         from device_adapters adapter where adapter.project_id=$1 and adapter.status in ('failed','degraded')
       union all
       select 'stream:'||stream.id::text,stream.device_id,'stream',case when stream.status='failed' then 'error' else 'warning' end,
              device.name||' · '||stream.stream_key,coalesce(stream.status_reason,stream.status),stream.status,
              coalesce(stream.ended_at,stream.updated_at)
         from live_streams stream join devices device on device.id=stream.device_id and device.project_id=stream.project_id
        where stream.project_id=$1 and stream.status in ('failed','degraded','starting')
       order by "occurredAt" desc limit 50
) snapshot_row;

-- name: SnapshotMedia :many
select to_jsonb(snapshot_row) as item from (
select asset.id, asset.kind, asset.mime_type as "mimeType", asset.device_id as "deviceId",
              asset.task_run_id as "taskRunId", asset.captured_at as "capturedAt",
              asset.created_at as "createdAt", asset.metadata_json as metadata
       from assets asset where asset.project_id = $1 and asset.status = 'available'
       order by coalesce(asset.captured_at, asset.created_at) desc limit 500
) snapshot_row;

-- name: SnapshotSuspectedConstruction :many
select to_jsonb(snapshot_row) as item from (
select group_row.id::text as id, group_row.project_id as "projectId", '疑似违建' as label,
              group_row.status, group_row.location_quality as "locationQuality",
              group_row.last_detected_at as "capturedAt", ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry
       from detection_groups group_row where group_row.project_id=$1 and group_row.status='active'
       order by group_row.last_detected_at desc limit 500
) snapshot_row;

-- name: SnapshotAlerts :many
select to_jsonb(snapshot_row) as item from (
select event.id,event.project_id as "projectId",'疑似违建' as title,event.status,event.severity,
              event.last_detected_at as "updatedAt",ST_AsGeoJSON(group_row.geographic_geometry)::json as geometry
       from perception_events event join detection_groups group_row on group_row.id=event.detection_group_id and group_row.project_id=event.project_id
       where event.project_id=$1 and event.status in('open','acknowledged','investigating')
       order by event.last_detected_at desc limit 500
) snapshot_row;

-- name: SnapshotIssues :many
select to_jsonb(snapshot_row) as item from (
select issue.id,issue.project_id as "projectId",issue.number,issue.title,issue.status,issue.priority,
              issue.last_seen_at as "updatedAt",ST_AsGeoJSON(located.geometry)::json as geometry
       from issues issue left join lateral (
         select detection.geographic_geometry as geometry
         from issue_links link join detections detection
           on detection.project_id=link.project_id
          and detection.id=case when link.target_id~'^[0-9]+$' then link.target_id::bigint end
         where link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='detection'
           and detection.geographic_geometry is not null
         order by detection.captured_at desc,detection.id desc limit 1
       ) located on true
       where issue.project_id=$1 and issue.status<>'closed'
       order by issue.last_seen_at desc limit 500
) snapshot_row;
