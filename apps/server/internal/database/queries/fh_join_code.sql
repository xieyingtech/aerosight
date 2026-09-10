-- name: FHJoinCodeAccess :many
SELECT to_jsonb(r) FROM (
select membership.role,
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",project.team_id::int as "teamId",adapter.status as "connectorStatus",
      adapter.credential_envelope_json as "credentialEnvelope",adapter.discovery_scope_json->>'projectUuid' as "projectUuid",
      adapter.discovery_scope_json->>'organizationUuid' as "organizationUuid",
      exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
        and capability.connector_instance_id=adapter.id and capability.capability_code='organization.read'
		and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		and capability.region='cn' and capability.deployment='cn-public-cloud'
		and capability.status='supported' and capability.evidence_level in('live-read','field-write')
        and (capability.expires_at is null or capability.expires_at>now())) as "managementCapabilityVerified"
    from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=sqlc.arg(p1)
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    where project.id=sqlc.arg(p2) and adapter.id=sqlc.arg(p3)
) r;
