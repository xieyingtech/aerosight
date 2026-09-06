-- name: ListFlightHubConnections :many
select to_jsonb(row_data) from (
select instance.id::text as id, instance.name, instance.status,
            instance.discovery_scope_json->>'projectUuid' as "projectUuid",
            instance.discovery_scope_json->>'projectName' as "projectName",
            instance.last_health_json->>'code' as "lastErrorCode",
            instance.last_checked_at as "lastValidatedAt",
            latest.finished_at as "lastSyncAt", latest.status as "lastSyncStatus",
            coalesce(inventory.discovered_count,0)::int as "discoveredCount",
            coalesce(inventory.managed_count,0)::int as "managedCount",
            coalesce(inventory.missing_count,0)::int as "missingCount",
            instance.created_at as "createdAt", instance.updated_at as "updatedAt"
       from connector_instances instance
       left join lateral (
         select run.status, run.finished_at
           from connector_sync_runs run
          where run.project_id=instance.project_id and run.connector_instance_id=instance.id
          order by run.created_at desc limit 1
       ) latest on true
       left join lateral (
         select count(*) filter (where identity.discovery_status='discovered') as discovered_count,
                count(*) filter (where identity.discovery_status='managed') as managed_count,
                count(*) filter (where identity.discovery_status='missing') as missing_count
           from device_external_identities identity
          where identity.project_id=instance.project_id and identity.adapter_id=instance.id
       ) inventory on true
      where instance.project_id=$1 and instance.connector_key='dji.flighthub2'
      order by instance.created_at
) row_data;

-- name: ListFlightHubIdentities :many
select to_jsonb(row_data) from (
select identity.id::text as id, identity.adapter_id::text as "connectorId", instance.name as "connectorName",
              identity.external_device_id as "externalDeviceId",
              identity.external_device_type as "externalDeviceType",
              identity.identity_json#>>'{attributes,serialNumber}' as "serialNumber",
              nullif(identity.identity_json#>>'{attributes,callsign}','') as callsign,
              nullif(identity.identity_json->>'parentExternalId','') as "parentExternalId",
              identity.discovery_status as "discoveryStatus", identity.source_version as "sourceVersion",
              identity.device_id as "deviceId", identity.last_seen_at as "lastSeenAt"
         from device_external_identities identity
         join connector_instances instance on instance.id=identity.adapter_id and instance.project_id=identity.project_id
        where identity.project_id=$1 and instance.connector_key='dji.flighthub2'
        order by identity.last_seen_at desc, identity.id desc limit 200
) row_data;

-- name: ListFlightHubSyncRuns :many
select to_jsonb(row_data) from (
select run.id::text as id, run.connector_instance_id::text as "connectorId", instance.name as "connectorName",
              run.status, run.discovered_count::int as "discoveredCount",
              run.managed_count::int as "managedCount", run.missing_count::int as "missingCount",
              run.error_code as "errorCode", run.started_at as "startedAt",
              run.finished_at as "finishedAt", run.created_at as "createdAt"
         from connector_sync_runs run
         join connector_instances instance on instance.id=run.connector_instance_id and instance.project_id=run.project_id
        where run.project_id=$1 and instance.connector_key='dji.flighthub2'
        order by run.created_at desc limit 100
) row_data;

