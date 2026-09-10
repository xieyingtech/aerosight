-- name: ReadFHManagementAccess :many
SELECT to_jsonb(r) FROM (
select membership.role,project.team_id::int as "teamId",adapter.project_id::int as "connectorProjectId",
        adapter.team_id::int as "connectorTeamId",adapter.id::text as "connectorId",adapter.name as "connectorName",adapter.status as "connectorStatus",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code='organization.read'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.status='supported' and capability.evidence_level in('live-read','field-write')
          and (capability.expires_at is null or capability.expires_at>now())) as "managementCapabilityVerified"
      from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(user_id)
      join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
      where project.id=sqlc.arg(project_id) and adapter.id=sqlc.arg(connector_id)
) r;

-- name: ReadFHManagementState :many
SELECT to_jsonb(r) FROM (
select status,attempt_count::int as "attemptCount",last_error_code as "lastErrorCode",last_started_at as "lastStartedAt",
        last_succeeded_at as "lastSucceededAt",next_attempt_at as "nextAttemptAt"
      from connector_resource_sync_states where project_id=sqlc.arg(project_id) and team_id=sqlc.arg(team_id) and connector_instance_id=sqlc.arg(connector_id) and resource_kind='organization'
) r;

-- name: ReadFHManagementResources :many
SELECT to_jsonb(r) FROM (
select id::text,connector_instance_id::text as "connectorId",resource_kind as kind,status,summary_json as summary,
        last_seen_at as "lastSeenAt",missing_at as "missingAt"
      from connector_remote_resources where project_id=sqlc.arg(project_id) and team_id=sqlc.arg(team_id) and connector_instance_id=sqlc.arg(connector_id)
        and resource_kind in('organization','organization-user','organization-role','organization-permission','project-user','project-member')
      order by resource_kind,last_seen_at desc,id desc limit 2000
) r;
