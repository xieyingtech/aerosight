create table device_adapters (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  name text not null,
  adapter_type text not null,
  vendor text,
  protocol_version text not null default '1',
  status text not null default 'disabled',
  secret_ref text,
  config_json jsonb not null default '{}'::jsonb,
  capabilities_json jsonb not null default '{}'::jsonb,
  last_health_json jsonb not null default '{}'::jsonb,
  last_checked_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint device_adapters_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_adapters_project_name_unique unique (project_id, name),
  constraint device_adapters_id_project_unique unique (id, project_id),
  constraint device_adapters_status_valid
    check (status in ('disabled', 'connecting', 'connected', 'degraded', 'failed'))
);
--> statement-breakpoint
create index device_adapters_project_status_idx on device_adapters(project_id, status);
--> statement-breakpoint
create unique index devices_id_project_unique on devices(id, project_id);
--> statement-breakpoint
alter table devices add column adapter_id bigint;
--> statement-breakpoint
alter table devices add column device_model text;
--> statement-breakpoint
alter table devices add column firmware_version text;
--> statement-breakpoint
alter table devices add column status_reason text;
--> statement-breakpoint
alter table devices add constraint devices_adapter_project_fk
  foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete set null (adapter_id);
--> statement-breakpoint
alter table device_capabilities add column project_id integer;
--> statement-breakpoint
alter table device_capabilities add column version_number integer not null default 1;
--> statement-breakpoint
alter table device_capabilities add column declared_by_adapter_id bigint;
--> statement-breakpoint
alter table device_capabilities add column updated_at timestamptz not null default now();
--> statement-breakpoint
update device_capabilities capability
set project_id = device.project_id
from devices device
where device.id = capability.device_id;
--> statement-breakpoint
alter table device_capabilities alter column project_id set not null;
--> statement-breakpoint
alter table device_capabilities add constraint device_capabilities_device_project_fk
  foreign key (device_id, project_id) references devices(id, project_id) on delete cascade;
--> statement-breakpoint
alter table device_capabilities add constraint device_capabilities_adapter_project_fk
  foreign key (declared_by_adapter_id, project_id) references device_adapters(id, project_id) on delete set null (declared_by_adapter_id);
--> statement-breakpoint
create table device_external_identities (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  device_id integer,
  external_device_id text not null,
  external_device_type text,
  identity_json jsonb not null default '{}'::jsonb,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  bound_at timestamptz,
  constraint device_external_identities_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_external_identities_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_external_identities_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_external_identities_adapter_external_unique
    unique (adapter_id, external_device_id),
  constraint device_external_identities_device_adapter_unique
    unique (device_id, adapter_id)
);
--> statement-breakpoint
create index device_external_identities_project_idx on device_external_identities(project_id, last_seen_at desc);
--> statement-breakpoint
create table device_connections (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  device_id integer,
  session_key text not null,
  status text not null default 'unknown',
  link_quality double precision,
  status_reason text,
  opened_at timestamptz not null default now(),
  last_heartbeat_at timestamptz,
  closed_at timestamptz,
  metadata_json jsonb not null default '{}'::jsonb,
  constraint device_connections_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_connections_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_connections_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_connections_session_unique unique (adapter_id, session_key),
  constraint device_connections_status_valid check (status in ('online', 'degraded', 'offline', 'unknown'))
);
--> statement-breakpoint
create index device_connections_project_status_idx on device_connections(project_id, status);
--> statement-breakpoint
create index device_connections_device_opened_idx on device_connections(device_id, opened_at desc);
--> statement-breakpoint
create table device_telemetry (
  id bigserial not null,
  project_id integer not null,
  team_id integer not null,
  adapter_id bigint not null,
  device_id integer not null,
  event_id text not null,
  telemetry_type text not null,
  sequence_number bigint,
  captured_at timestamptz not null,
  received_at timestamptz not null default now(),
  payload_json jsonb not null default '{}'::jsonb,
  quality_json jsonb not null default '{}'::jsonb,
  primary key (id, captured_at),
  constraint device_telemetry_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_telemetry_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint device_telemetry_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade
) partition by range (captured_at);
--> statement-breakpoint
create table device_telemetry_default partition of device_telemetry default;
--> statement-breakpoint
create unique index device_telemetry_source_event_unique
  on device_telemetry(adapter_id, event_id, captured_at);
--> statement-breakpoint
create index device_telemetry_project_time_idx on device_telemetry(project_id, captured_at desc);
--> statement-breakpoint
create index device_telemetry_device_time_idx on device_telemetry(device_id, captured_at desc);
