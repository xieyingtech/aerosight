create table connector_definitions (
  id bigserial primary key,
  connector_key text not null,
  version text not null,
  display_name text not null,
  status text not null default 'active',
  manifest_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_definitions_key_version_unique unique (connector_key, version),
  constraint connector_definitions_status_valid check (status in ('active', 'disabled', 'retired')),
  constraint connector_definitions_manifest_object check (jsonb_typeof(manifest_json) = 'object')
);
--> statement-breakpoint
insert into connector_definitions (connector_key, version, display_name, manifest_json)
values
  ('dji.cloud-api', '1.0.0', 'DJI Cloud API',
   '{"discoveryModes":["subscribe","push"],"protocols":["mqtt","https","websocket"],"compatibleDrivers":["dji.cloud"]}'::jsonb),
  ('simulator.memory', '1.0.0', 'In-memory Simulator',
   '{"discoveryModes":["push"],"protocols":["memory"],"compatibleDrivers":["simulator"]}'::jsonb),
  ('legacy.adapter', '1.0.0', 'Legacy Adapter Compatibility',
   '{"discoveryModes":["manual-import"],"protocols":[],"compatibleDrivers":["*"]}'::jsonb)
on conflict (connector_key, version) do nothing;
--> statement-breakpoint
alter table device_adapters
  add column connector_definition_id bigint,
  add column onboarding_policy text not null default 'review',
  add column discovery_scope_json jsonb not null default '{}'::jsonb,
  add column sync_cursor_json jsonb not null default '{}'::jsonb;
--> statement-breakpoint
update device_adapters adapter
   set connector_definition_id = definition.id
  from connector_definitions definition
 where definition.version = '1.0.0'
   and definition.connector_key = case adapter.adapter_type
     when 'dji' then 'dji.cloud-api'
     when 'simulator' then 'simulator.memory'
     else 'legacy.adapter'
   end;
--> statement-breakpoint
alter table device_adapters
  alter column connector_definition_id set not null,
  add constraint device_adapters_connector_definition_fk
    foreign key (connector_definition_id) references connector_definitions(id) on delete restrict,
  add constraint device_adapters_onboarding_policy_valid
    check (onboarding_policy in ('automatic', 'review', 'observe-only')),
  add constraint device_adapters_discovery_scope_object
    check (jsonb_typeof(discovery_scope_json) = 'object'),
  add constraint device_adapters_sync_cursor_object
    check (jsonb_typeof(sync_cursor_json) = 'object');
--> statement-breakpoint
create or replace function populate_device_adapter_connector_definition()
returns trigger language plpgsql as $$
begin
  if new.connector_definition_id is null then
    select definition.id into new.connector_definition_id
      from connector_definitions definition
     where definition.version = '1.0.0'
       and definition.connector_key = case new.adapter_type
         when 'dji' then 'dji.cloud-api'
         when 'simulator' then 'simulator.memory'
         else 'legacy.adapter'
       end;
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger device_adapters_populate_connector_definition
before insert or update of adapter_type, connector_definition_id
on device_adapters
for each row execute function populate_device_adapter_connector_definition();
--> statement-breakpoint
create index device_adapters_connector_definition_idx
  on device_adapters(connector_definition_id, status);
--> statement-breakpoint
create view connector_instances as
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
       adapter.updated_at
  from device_adapters adapter
  join connector_definitions definition on definition.id = adapter.connector_definition_id;
--> statement-breakpoint
create table connector_sync_runs (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  discovery_mode text not null,
  status text not null default 'pending',
  scope_json jsonb not null default '{}'::jsonb,
  cursor_before_json jsonb not null default '{}'::jsonb,
  cursor_after_json jsonb not null default '{}'::jsonb,
  discovered_count integer not null default 0,
  managed_count integer not null default 0,
  missing_count integer not null default 0,
  error_code text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  constraint connector_sync_runs_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_sync_runs_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_sync_runs_mode_valid
    check (discovery_mode in ('push', 'poll', 'subscribe', 'manual-import')),
  constraint connector_sync_runs_status_valid
    check (status in ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
  constraint connector_sync_runs_counts_nonnegative
    check (discovered_count >= 0 and managed_count >= 0 and missing_count >= 0),
  constraint connector_sync_runs_time_valid
    check (finished_at is null or (started_at is not null and finished_at >= started_at)),
  constraint connector_sync_runs_scope_object check (jsonb_typeof(scope_json) = 'object'),
  constraint connector_sync_runs_cursor_before_object check (jsonb_typeof(cursor_before_json) = 'object'),
  constraint connector_sync_runs_cursor_after_object check (jsonb_typeof(cursor_after_json) = 'object'),
  constraint connector_sync_runs_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_sync_runs_connector_created_idx
  on connector_sync_runs(connector_instance_id, created_at desc);
--> statement-breakpoint
alter table device_external_identities
  add column discovery_status text not null default 'discovered',
  add column suggested_device_type_id bigint,
  add column match_confidence double precision,
  add column source_version text,
  add column last_sync_run_id bigint;
--> statement-breakpoint
update device_external_identities
   set discovery_status = case when device_id is null then 'discovered' else 'managed' end;
--> statement-breakpoint
alter table device_external_identities
  add constraint device_external_identities_status_valid
    check (discovery_status in ('discovered', 'managed', 'ignored', 'conflicted', 'missing')),
  add constraint device_external_identities_match_confidence_valid
    check (match_confidence is null or (match_confidence >= 0 and match_confidence <= 1)),
  add constraint device_external_identities_suggested_type_fk
    foreign key (suggested_device_type_id) references device_types(id) on delete set null,
  add constraint device_external_identities_sync_run_project_fk
    foreign key (last_sync_run_id, project_id)
    references connector_sync_runs(id, project_id) on delete set null (last_sync_run_id),
  add constraint device_external_identities_connector_identity_project_unique
    unique (id, adapter_id, project_id);
--> statement-breakpoint
insert into device_external_identities (
  project_id, team_id, adapter_id, device_id, external_device_id,
  external_device_type, identity_json, discovery_status, bound_at
)
select device.project_id, project.team_id, device.adapter_id, device.id,
       'migration:device:' || device.id::text, device.type,
       jsonb_build_object('source', 'connector-migration', 'legacyDeviceId', device.id),
       'managed', now()
  from devices device
  join projects project on project.id = device.project_id
 where device.adapter_id is not null
   and not exists (
     select 1 from device_external_identities identity
      where identity.project_id = device.project_id
        and identity.adapter_id = device.adapter_id
        and identity.device_id = device.id
   );
--> statement-breakpoint
create table device_connector_bindings (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  device_id integer not null,
  connector_instance_id bigint not null,
  external_identity_id bigint not null,
  route_role text not null default 'direct',
  priority integer not null default 100,
  status text not null default 'active',
  bound_at timestamptz not null default now(),
  unbound_at timestamptz,
  metadata_json jsonb not null default '{}'::jsonb,
  constraint device_connector_bindings_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_connector_bindings_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_connector_bindings_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_connector_bindings_identity_connector_project_fk
    foreign key (external_identity_id, connector_instance_id, project_id)
    references device_external_identities(id, adapter_id, project_id) on delete cascade,
  constraint device_connector_bindings_route_role_valid
    check (route_role in ('direct', 'gateway', 'inherited')),
  constraint device_connector_bindings_priority_nonnegative check (priority >= 0),
  constraint device_connector_bindings_status_valid
    check (status in ('active', 'standby', 'disabled', 'conflicted')),
  constraint device_connector_bindings_time_valid
    check (unbound_at is null or unbound_at >= bound_at),
  constraint device_connector_bindings_metadata_object check (jsonb_typeof(metadata_json) = 'object'),
  constraint device_connector_bindings_identity_unique unique (external_identity_id),
  constraint device_connector_bindings_device_connector_unique unique (device_id, connector_instance_id),
  constraint device_connector_bindings_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
insert into device_connector_bindings (
  project_id, team_id, device_id, connector_instance_id, external_identity_id,
  route_role, priority, status, bound_at, metadata_json
)
select identity.project_id, identity.team_id, identity.device_id, identity.adapter_id, identity.id,
       'direct', 100, 'active', coalesce(identity.bound_at, now()),
       jsonb_build_object('source', 'connector-migration')
  from device_external_identities identity
 where identity.device_id is not null
on conflict (device_id, connector_instance_id) do nothing;
--> statement-breakpoint
create index device_connector_bindings_device_route_idx
  on device_connector_bindings(device_id, status, priority);
--> statement-breakpoint
create index device_connector_bindings_connector_idx
  on device_connector_bindings(connector_instance_id, status);
--> statement-breakpoint
create index device_external_identities_discovery_idx
  on device_external_identities(project_id, discovery_status, last_seen_at desc);
