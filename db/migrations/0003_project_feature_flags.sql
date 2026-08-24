create table project_feature_flags (
  project_id integer primary key references projects(id) on delete cascade,
  device_commands_enabled boolean not null default false,
  operations_overview_enabled boolean not null default false,
  object_storage_enabled boolean not null default false,
  external_algorithms_enabled boolean not null default false,
  automatic_ai_enabled boolean not null default false,
  dependency_health_json jsonb not null default '{}'::jsonb,
  updated_by_user_id integer references users(id) on delete set null,
  updated_at timestamptz not null default now()
);
--> statement-breakpoint
create index project_feature_flags_updated_idx on project_feature_flags(updated_at);
