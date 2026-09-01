alter table device_adapters
  add column external_scope_key text,
  add constraint device_adapters_external_scope_key_normalized
    check (
      external_scope_key is null
      or (
        length(external_scope_key) between 1 and 512
        and external_scope_key = btrim(external_scope_key)
      )
    );
--> statement-breakpoint
create unique index device_adapters_connector_external_scope_unique
  on device_adapters(project_id, connector_definition_id, external_scope_key)
  where external_scope_key is not null;
--> statement-breakpoint
create or replace view connector_instances as
select adapter.id,
       adapter.project_id,
       adapter.team_id,
       adapter.name,
       adapter.connector_definition_id,
       definition.connector_key,
       definition.version as connector_version,
       adapter.adapter_type as legacy_adapter_type,
       adapter.vendor,
       adapter.protocol_version,
       adapter.status,
       adapter.secret_ref,
       adapter.config_json,
       adapter.capabilities_json,
       adapter.network_profile_id,
       adapter.onboarding_policy,
       adapter.discovery_scope_json,
       adapter.sync_cursor_json,
       adapter.last_health_json,
       adapter.last_checked_at,
       adapter.created_at,
       adapter.updated_at,
       adapter.external_scope_key,
       adapter.credential_envelope_json
  from device_adapters adapter
  join connector_definitions definition on definition.id = adapter.connector_definition_id;
