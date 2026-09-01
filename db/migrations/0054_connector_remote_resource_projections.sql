create table connector_resource_sync_states (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  resource_kind text not null,
  status text not null default 'idle',
  cursor_json jsonb not null default '{}'::jsonb,
  attempt_count integer not null default 0,
  last_error_code text,
  last_started_at timestamptz,
  last_succeeded_at timestamptz,
  next_attempt_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_resource_sync_states_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_resource_sync_states_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_resource_sync_states_kind_valid
    check (resource_kind in (
      'inventory','device-state','health','active-operations','waylines',
      'flight-tasks','flight-artifacts','live','geospatial','models','organization'
    )),
  constraint connector_resource_sync_states_status_valid
    check (status in ('idle','running','backoff','failed','disabled')),
  constraint connector_resource_sync_states_attempt_nonnegative check (attempt_count >= 0),
  constraint connector_resource_sync_states_cursor_object check (jsonb_typeof(cursor_json) = 'object'),
  constraint connector_resource_sync_states_time_valid
    check (last_succeeded_at is null or last_started_at is null or last_succeeded_at >= last_started_at),
  constraint connector_resource_sync_states_unique
    unique (project_id, connector_instance_id, resource_kind),
  constraint connector_resource_sync_states_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_resource_sync_states_due_idx
  on connector_resource_sync_states(connector_instance_id, status, next_attempt_at);
--> statement-breakpoint
create table connector_remote_resources (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  resource_kind text not null,
  remote_id text not null,
  remote_version text,
  remote_updated_at timestamptz,
  status text not null default 'active',
  summary_json jsonb not null default '{}'::jsonb,
  canonical_target_type text,
  canonical_target_id text,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  missing_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_remote_resources_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_remote_resources_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_remote_resources_kind_valid
    check (resource_kind in (
      'wayline','flight-task','flight-media','flight-record','flight-alert','ai-alert',
      'map-element','flight-area','offline-map','air-sense-warning','model','model-resource',
      'live-share','stream-converter','recording','hms','topology','auto-record',
      'organization-user','organization-role','organization-permission'
    )),
  constraint connector_remote_resources_status_valid
    check (status in ('active','missing','deleted','failed')),
  constraint connector_remote_resources_remote_id_valid
    check (length(btrim(remote_id)) between 1 and 512 and remote_id = btrim(remote_id)),
  constraint connector_remote_resources_summary_object check (jsonb_typeof(summary_json) = 'object'),
  constraint connector_remote_resources_canonical_pair
    check ((canonical_target_type is null) = (canonical_target_id is null)),
  constraint connector_remote_resources_missing_time
    check ((status = 'missing') = (missing_at is not null) or status in ('deleted','failed')),
  constraint connector_remote_resources_unique
    unique (project_id, connector_instance_id, resource_kind, remote_id),
  constraint connector_remote_resources_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create index connector_remote_resources_lookup_idx
  on connector_remote_resources(project_id, resource_kind, status, last_seen_at desc);
--> statement-breakpoint
create index connector_remote_resources_canonical_idx
  on connector_remote_resources(project_id, canonical_target_type, canonical_target_id)
  where canonical_target_type is not null;
--> statement-breakpoint
create table connector_capability_snapshots (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  capability_code text not null,
  status text not null,
  evidence_level text not null,
  region text not null,
  deployment text not null,
  device_model text,
  firmware_version text,
  details_json jsonb not null default '{}'::jsonb,
  verified_at timestamptz not null,
  expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_capability_snapshots_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint connector_capability_snapshots_connector_project_fk
    foreign key (connector_instance_id, project_id) references device_adapters(id, project_id) on delete cascade,
  constraint connector_capability_snapshots_status_valid
    check (status in ('supported','empty','forbidden','not_applicable','unverified','degraded','failed')),
  constraint connector_capability_snapshots_evidence_valid
    check (evidence_level in ('documented','fixture','live-read','field-write')),
  constraint connector_capability_snapshots_identity_valid
    check (
      length(btrim(capability_code)) between 1 and 256
      and capability_code = btrim(capability_code)
      and length(btrim(region)) between 1 and 64
      and length(btrim(deployment)) between 1 and 128
    ),
  constraint connector_capability_snapshots_details_object check (jsonb_typeof(details_json) = 'object'),
  constraint connector_capability_snapshots_time_valid check (expires_at is null or expires_at > verified_at),
  constraint connector_capability_snapshots_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index connector_capability_snapshots_identity_unique
  on connector_capability_snapshots(
    project_id, connector_instance_id, capability_code, region, deployment, device_model, firmware_version
  ) nulls not distinct;
--> statement-breakpoint
create index connector_capability_snapshots_effective_idx
  on connector_capability_snapshots(connector_instance_id, status, expires_at);
