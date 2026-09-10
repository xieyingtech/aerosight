-- name: ReadFHDiagnosticsAccess :many
SELECT to_jsonb(r) FROM (
select adapter.id::text,adapter.name,adapter.status,
             adapter.last_health_json->>'code' as "lastErrorCode",adapter.last_checked_at as "lastCheckedAt"
        from device_adapters adapter
        join connector_definitions definition on definition.id=adapter.connector_definition_id
        join projects project on project.id=adapter.project_id and project.team_id=adapter.team_id
        join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(user_id)
       where adapter.project_id=sqlc.arg(project_id) and adapter.id=sqlc.arg(connector_id)
         and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
) r;

-- name: ReadFHDiagnosticsWatermarks :many
SELECT to_jsonb(r) FROM (
select resource_kind as "resourceKind",status,attempt_count::int as "attemptCount",
               last_error_code as "lastErrorCode",last_started_at as "lastStartedAt",
               last_succeeded_at as "lastSucceededAt",next_attempt_at as "nextAttemptAt"
          from connector_resource_sync_states
         where project_id=sqlc.arg(project_id) and connector_instance_id=sqlc.arg(connector_id)
         order by resource_kind
) r;

-- name: ReadFHDiagnosticsCapabilities :many
SELECT to_jsonb(r) FROM (
select capability_code as "capabilityCode",status,evidence_level as "evidenceLevel",region,deployment,
               device_model as "deviceModel",firmware_version as "firmwareVersion",
               details_json->>'reason' as reason,details_json->>'endpointId' as "endpointId",
               details_json->'layers' as layers,verified_at as "verifiedAt",expires_at as "expiresAt",
               (expires_at is not null and expires_at<=now()) as expired
          from connector_capability_snapshots capability
         where capability.project_id=sqlc.arg(project_id) and capability.connector_instance_id=sqlc.arg(connector_id)
		   and ((account_fingerprint is null and evidence_level in('documented','fixture'))
		     or account_fingerprint=(select discovery_scope_json->>'accountFingerprint' from device_adapters
		       where id=sqlc.arg(connector_id) and project_id=sqlc.arg(project_id)))
         order by capability_code,verified_at desc
         limit 500
) r;
