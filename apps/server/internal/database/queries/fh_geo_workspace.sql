-- name: ReadFHGeoAccess :many
SELECT to_jsonb(r) FROM (
select project.id::int as "projectId",project.team_id::int as "teamId",membership.role
        from projects project
        join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(user_id)
       where project.id=sqlc.arg(project_id)
) r;

-- name: ReadFHGeoConnectors :many
SELECT to_jsonb(r) FROM (
select adapter.project_id::int as "projectId",adapter.id::text,adapter.name,adapter.status,
             adapter.last_checked_at as "lastCheckedAt"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
       where adapter.project_id=sqlc.arg(project_id) and adapter.team_id=sqlc.arg(team_id)
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
       order by adapter.updated_at desc limit 50
) r;

-- name: ReadFHGeoSyncStates :many
SELECT to_jsonb(r) FROM (
select state.project_id::int as "projectId",state.connector_instance_id::text as "connectorId",state.status,
             state.attempt_count::int as "attemptCount",state.last_error_code as "lastErrorCode",
             state.last_started_at as "lastStartedAt",state.last_succeeded_at as "lastSucceededAt",
             state.next_attempt_at as "nextAttemptAt"
        from connector_resource_sync_states state
        join device_adapters adapter on adapter.id=state.connector_instance_id and adapter.project_id=state.project_id
       where state.project_id=sqlc.arg(project_id) and state.team_id=sqlc.arg(team_id) and state.resource_kind='geospatial'
       order by state.updated_at desc limit 50
) r;

-- name: ReadFHGeoResources :many
SELECT to_jsonb(r) FROM (
select resource.project_id::int as "projectId",resource.id::text,
             resource.connector_instance_id::text as "connectorId",resource.resource_kind as kind,resource.status,
             resource.remote_version as "remoteVersion",resource.remote_updated_at as "remoteUpdatedAt",
             resource.last_seen_at as "lastSeenAt",resource.missing_at as "missingAt",
             resource.summary_json->>'name' as name,
             case when resource.resource_kind='air-sense-warning'
                    and jsonb_typeof(resource.summary_json->'longitude')='number'
                    and jsonb_typeof(resource.summary_json->'latitude')='number'
                  then jsonb_build_object('type','Point','coordinates',jsonb_build_array(
                    resource.summary_json->'longitude',resource.summary_json->'latitude'))
                  else resource.summary_json->'geometry' end as geometry,
             resource.summary_json->>'coordinateReference' as "coordinateReference",
             resource.summary_json->>'status' as "stateCode",resource.summary_json->>'display' as display,
             resource.summary_json->>'areaType' as "areaType",resource.summary_json->>'percent' as progress,
             resource.summary_json->>'result' as "resultCode",resource.summary_json->>'modelCount' as "modelCount",
             resource.summary_json->'modelNames' as "modelNames",resource.summary_json->>'warningLevel' as "warningLevel",
             resource.summary_json->>'deviceId' as "deviceId",resource.summary_json->>'expiresAt' as "expiresAt",
             resource.summary_json->>'expired' as expired,issue.id::int as "issueId"
        from connector_remote_resources resource
        join device_adapters adapter on adapter.id=resource.connector_instance_id and adapter.project_id=resource.project_id
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        left join perception_events event on resource.resource_kind='air-sense-warning'
          and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
          and event.project_id=resource.project_id
        left join issue_links issue_link on issue_link.project_id=resource.project_id
          and issue_link.link_type='perception_event' and issue_link.target_id=event.id::text
        left join issues issue on issue.id=issue_link.issue_id and issue.project_id=resource.project_id
       where resource.project_id=sqlc.arg(project_id) and resource.team_id=sqlc.arg(team_id)
         and resource.resource_kind in('map-element','flight-area','offline-map','air-sense-warning')
         and definition.connector_key='dji.flighthub2'
       order by coalesce(resource.remote_updated_at,resource.last_seen_at) desc,resource.id desc limit 1000
) r;
