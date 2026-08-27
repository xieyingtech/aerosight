create table driver_definitions (
  id bigserial primary key,
  driver_key text not null,
  version text not null,
  display_name text not null,
  status text not null default 'active',
  manifest_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint driver_definitions_key_version_unique unique (driver_key, version),
  constraint driver_definitions_status_valid check (status in ('active', 'disabled', 'retired')),
  constraint driver_definitions_manifest_object check (jsonb_typeof(manifest_json) = 'object')
);
--> statement-breakpoint
create table device_types (
  id bigserial primary key,
  type_key text not null,
  version integer not null,
  display_name text not null,
  category text not null,
  vendor text,
  model text,
  driver_definition_id bigint not null references driver_definitions(id) on delete restrict,
  driver_version_constraint text not null default '*',
  capability_profile_json jsonb not null default '{}'::jsonb,
  status text not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint device_types_key_version_unique unique (type_key, version),
  constraint device_types_version_positive check (version > 0),
  constraint device_types_status_valid check (status in ('active', 'retired')),
  constraint device_types_capability_profile_object check (jsonb_typeof(capability_profile_json) = 'object')
);
--> statement-breakpoint
insert into driver_definitions (driver_key, version, display_name, manifest_json)
values (
  'legacy.static',
  '1.0.0',
  'Legacy static device driver',
  '{"protocols":[],"capabilities":{"state.read":{"risk":"low"}},"streamHandlers":[],"commandHandlers":[]}'::jsonb
)
on conflict (driver_key, version) do nothing;
--> statement-breakpoint
insert into device_types (
  type_key, version, display_name, category, driver_definition_id,
  driver_version_constraint, capability_profile_json
)
select 'legacy.device', 1, 'Legacy device', 'unknown', driver.id, '=1.0.0',
       '{"capabilities":{"state.read":{}}}'::jsonb
  from driver_definitions driver
 where driver.driver_key = 'legacy.static' and driver.version = '1.0.0'
on conflict (type_key, version) do nothing;
--> statement-breakpoint
alter table devices add column device_type_id bigint;
--> statement-breakpoint
update devices
   set device_type_id = (
     select id from device_types where type_key = 'legacy.device' and version = 1
   )
 where device_type_id is null;
--> statement-breakpoint
alter table devices
  alter column device_type_id set not null,
  add constraint devices_device_type_fk foreign key (device_type_id) references device_types(id) on delete restrict;
--> statement-breakpoint
alter table devices drop constraint devices_connectivity_status_valid;
--> statement-breakpoint
alter table devices add constraint devices_connectivity_status_valid
  check (status in ('online', 'degraded', 'offline', 'unknown', 'unavailable'));
--> statement-breakpoint
create index devices_project_type_idx on devices(project_id, device_type_id);
--> statement-breakpoint
create table device_relationships (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  from_device_id integer not null,
  to_device_id integer not null,
  relation_type text not null,
  source_type text not null default 'manual',
  valid_from timestamptz not null default now(),
  valid_until timestamptz,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  constraint device_relationships_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_relationships_from_project_fk
    foreign key (from_device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_relationships_to_project_fk
    foreign key (to_device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_relationships_not_self check (from_device_id <> to_device_id),
  constraint device_relationships_valid_range check (valid_until is null or valid_until > valid_from),
  constraint device_relationships_source_valid check (source_type in ('driver', 'discovery', 'manual', 'migration')),
  constraint device_relationships_unique unique (project_id, from_device_id, to_device_id, relation_type, valid_from),
  constraint device_relationships_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index device_relationships_from_idx on device_relationships(from_device_id, relation_type, valid_from desc);
--> statement-breakpoint
create index device_relationships_to_idx on device_relationships(to_device_id, relation_type, valid_from desc);
--> statement-breakpoint
alter table device_capabilities
  add column device_type_id bigint,
  add column driver_definition_id bigint,
  add column availability text not null default 'available',
  add column availability_reason text,
  add column input_schema_json jsonb not null default '{}'::jsonb,
  add column output_schema_json jsonb not null default '{}'::jsonb,
  add column risk_level text not null default 'low',
  add column source_json jsonb not null default '{}'::jsonb;
--> statement-breakpoint
update device_capabilities capability
   set device_type_id = device.device_type_id
  from devices device
 where device.id = capability.device_id;
--> statement-breakpoint
update device_capabilities capability
   set driver_definition_id = device_type.driver_definition_id
  from device_types device_type
 where device_type.id = capability.device_type_id;
--> statement-breakpoint
create or replace function populate_device_capability_type_driver()
returns trigger language plpgsql as $$
begin
  if new.device_type_id is null or new.driver_definition_id is null then
    select device.device_type_id, device_type.driver_definition_id
      into new.device_type_id, new.driver_definition_id
      from devices device
      join device_types device_type on device_type.id = device.device_type_id
     where device.id = new.device_id and device.project_id = new.project_id;
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger device_capabilities_populate_type_driver
before insert or update of device_id, project_id, device_type_id, driver_definition_id
on device_capabilities
for each row execute function populate_device_capability_type_driver();
--> statement-breakpoint
alter table device_capabilities
  alter column device_type_id set not null,
  alter column driver_definition_id set not null,
  add constraint device_capabilities_device_type_fk
    foreign key (device_type_id) references device_types(id) on delete restrict,
  add constraint device_capabilities_driver_fk
    foreign key (driver_definition_id) references driver_definitions(id) on delete restrict,
  add constraint device_capabilities_availability_valid
    check (availability in ('available', 'degraded', 'unavailable')),
  add constraint device_capabilities_risk_valid
    check (risk_level in ('low', 'medium', 'high', 'critical'));
--> statement-breakpoint
create index device_capabilities_type_code_idx on device_capabilities(device_type_id, capability_code);
--> statement-breakpoint
create table device_network_profiles (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  name text not null,
  mode text not null,
  mqtt_endpoint text,
  api_public_base_url text,
  websocket_public_url text,
  media_ingest_base_url text,
  media_playback_base_url text,
  tls_required boolean not null default false,
  secret_ref text,
  status text not null default 'unverified',
  config_json jsonb not null default '{}'::jsonb,
  last_validation_json jsonb not null default '{}'::jsonb,
  last_validated_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint device_network_profiles_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_network_profiles_mode_valid check (mode in ('lan', 'public')),
  constraint device_network_profiles_status_valid check (status in ('unverified', 'valid', 'invalid', 'degraded')),
  constraint device_network_profiles_public_tls check (mode <> 'public' or tls_required),
  constraint device_network_profiles_project_name_unique unique (project_id, name),
  constraint device_network_profiles_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
alter table device_adapters add column network_profile_id bigint;
--> statement-breakpoint
alter table device_adapters add constraint device_adapters_network_profile_project_fk
  foreign key (network_profile_id, project_id)
  references device_network_profiles(id, project_id) on delete set null (network_profile_id);
--> statement-breakpoint
create table device_stream_channels (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  device_id integer not null,
  capability_code text not null,
  channel_key text not null,
  display_name text not null,
  data_type text not null,
  schema_json jsonb not null default '{}'::jsonb,
  unit text,
  protocol text,
  quality_json jsonb not null default '{}'::jsonb,
  availability text not null default 'available',
  availability_reason text,
  source_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint device_stream_channels_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_stream_channels_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_stream_channels_capability_fk
    foreign key (device_id, capability_code)
    references device_capabilities(device_id, capability_code) on delete cascade,
  constraint device_stream_channels_data_type_valid
    check (data_type in ('video', 'audio', 'telemetry', 'sensor', 'events')),
  constraint device_stream_channels_availability_valid
    check (availability in ('available', 'degraded', 'unavailable')),
  constraint device_stream_channels_device_key_unique unique (device_id, channel_key),
  constraint device_stream_channels_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index device_stream_channels_project_type_idx on device_stream_channels(project_id, data_type, availability);
--> statement-breakpoint
create table device_capability_grants (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  user_id integer not null,
  scope_type text not null,
  device_type_id bigint,
  device_id integer,
  action_pattern text not null,
  effect text not null default 'allow',
  granted_by_user_id integer,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  constraint device_capability_grants_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_capability_grants_team_member_fk
    foreign key (team_id, user_id) references team_members(team_id, user_id) on delete cascade,
  constraint device_capability_grants_type_fk
    foreign key (device_type_id) references device_types(id) on delete cascade,
  constraint device_capability_grants_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_capability_grants_granter_fk
    foreign key (granted_by_user_id) references users(id) on delete set null,
  constraint device_capability_grants_scope_valid check (
    (scope_type = 'project' and device_type_id is null and device_id is null) or
    (scope_type = 'device_type' and device_type_id is not null and device_id is null) or
    (scope_type = 'device' and device_type_id is null and device_id is not null)
  ),
  constraint device_capability_grants_effect_valid check (effect in ('allow', 'deny')),
  constraint device_capability_grants_action_nonempty check (length(trim(action_pattern)) > 0)
);
--> statement-breakpoint
create unique index device_capability_grants_unique
  on device_capability_grants(project_id, user_id, scope_type, device_type_id, device_id, action_pattern)
  nulls not distinct;
--> statement-breakpoint
create index device_capability_grants_lookup_idx
  on device_capability_grants(project_id, user_id, action_pattern, expires_at);
--> statement-breakpoint
alter table live_streams
  add column stream_channel_id bigint,
  add column ingest_ref text,
  add column lease_expires_at timestamptz;
--> statement-breakpoint
alter table live_streams add constraint live_streams_channel_project_fk
  foreign key (stream_channel_id, project_id)
  references device_stream_channels(id, project_id) on delete set null (stream_channel_id);
--> statement-breakpoint
alter table algorithm_definition_versions
  add column output_schema_json jsonb not null default '{}'::jsonb,
  add column display_metadata_json jsonb not null default '{}'::jsonb;
--> statement-breakpoint
create index driver_definitions_status_idx on driver_definitions(status, driver_key);
--> statement-breakpoint
create index device_types_driver_idx on device_types(driver_definition_id, status, type_key);
