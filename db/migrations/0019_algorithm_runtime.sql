create table algorithm_providers (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  name text not null,
  provider_type text not null,
  base_url text not null,
  secret_ref text,
  auth_type text not null default 'none',
  allowed_headers_json jsonb not null default '[]'::jsonb,
  timeout_seconds integer not null default 30,
  concurrency_limit integer not null default 1,
  rate_limit_per_minute integer not null default 60,
  status text not null default 'disabled',
  health_json jsonb not null default '{}'::jsonb,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint algorithm_providers_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_providers_creator_fk foreign key (created_by_user_id) references users(id) on delete set null,
  constraint algorithm_providers_type_valid check (provider_type in ('http-json','kserve-v2','ogc-processes','ai-sdk')),
  constraint algorithm_providers_auth_valid check (auth_type in ('none','bearer','api-key-header','basic','signed')),
  constraint algorithm_providers_status_valid check (status in ('disabled','testing','active','degraded','failed')),
  constraint algorithm_providers_limits_valid check (timeout_seconds between 1 and 3600 and concurrency_limit > 0 and rate_limit_per_minute > 0),
  constraint algorithm_providers_project_name_unique unique (project_id, name),
  constraint algorithm_providers_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table algorithm_definitions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  provider_id bigint not null,
  name text not null,
  capability_code text not null,
  description text,
  current_published_version_id bigint,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint algorithm_definitions_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_definitions_provider_project_fk foreign key (provider_id, project_id) references algorithm_providers(id, project_id) on delete cascade,
  constraint algorithm_definitions_creator_fk foreign key (created_by_user_id) references users(id) on delete set null,
  constraint algorithm_definitions_project_name_unique unique (project_id, name),
  constraint algorithm_definitions_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table algorithm_definition_versions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  algorithm_definition_id bigint not null,
  version integer not null,
  status text not null default 'draft',
  execution_mode text not null,
  model_or_process text not null,
  input_requirements_json jsonb not null default '{}'::jsonb,
  parameters_schema_json jsonb not null default '{}'::jsonb,
  protocol_config_json jsonb not null default '{}'::jsonb,
  output_mapping_json jsonb not null default '{}'::jsonb,
  label_mapping_json jsonb not null default '{}'::jsonb,
  publish_threshold double precision not null default 0,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint algorithm_definition_versions_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_definition_versions_definition_project_fk foreign key (algorithm_definition_id, project_id) references algorithm_definitions(id, project_id) on delete cascade,
  constraint algorithm_definition_versions_creator_fk foreign key (created_by_user_id) references users(id) on delete set null,
  constraint algorithm_definition_versions_publisher_fk foreign key (published_by_user_id) references users(id) on delete set null,
  constraint algorithm_definition_versions_status_valid check (status in ('draft','published','retired')),
  constraint algorithm_definition_versions_mode_valid check (execution_mode in ('synchronous','asynchronous','callback')),
  constraint algorithm_definition_versions_threshold_valid check (publish_threshold between 0 and 1),
  constraint algorithm_definition_versions_version_valid check (version > 0),
  constraint algorithm_definition_versions_definition_version_unique unique (algorithm_definition_id, version),
  constraint algorithm_definition_versions_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
alter table algorithm_definitions add constraint algorithm_definitions_current_version_project_fk
  foreign key (current_published_version_id, project_id)
  references algorithm_definition_versions(id, project_id) on delete set null (current_published_version_id);
--> statement-breakpoint
create unique index algorithm_definition_versions_one_draft_idx
  on algorithm_definition_versions(algorithm_definition_id) where status = 'draft';
--> statement-breakpoint
create table algorithm_runs (
  id uuid primary key,
  project_id integer not null,
  team_id integer not null,
  algorithm_definition_version_id bigint not null,
  input_asset_id integer not null,
  task_run_id integer,
  device_id integer,
  idempotency_key text not null,
  status text not null default 'queued',
  parameters_json jsonb not null default '{}'::jsonb,
  input_snapshot_json jsonb not null default '{}'::jsonb,
  external_job_id text,
  callback_token_hash text,
  raw_result_object_key text,
  raw_result_checksum_sha256 text,
  canonical_result_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  error_code text,
  error_message text,
  constraint algorithm_runs_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_runs_version_project_fk foreign key (algorithm_definition_version_id, project_id) references algorithm_definition_versions(id, project_id) on delete restrict,
  constraint algorithm_runs_asset_project_fk foreign key (input_asset_id, project_id) references assets(id, project_id) on delete restrict,
  constraint algorithm_runs_task_run_project_fk foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete set null (task_run_id),
  constraint algorithm_runs_device_project_fk foreign key (device_id, project_id) references devices(id, project_id) on delete set null (device_id),
  constraint algorithm_runs_status_valid check (status in ('queued','running','polling','waiting_callback','succeeded','failed','canceled','timed_out')),
  constraint algorithm_runs_checksum_valid check (raw_result_checksum_sha256 is null or raw_result_checksum_sha256 ~ '^[a-f0-9]{64}$'),
  constraint algorithm_runs_project_idempotency_unique unique (project_id, idempotency_key),
  constraint algorithm_runs_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table algorithm_run_attempts (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  algorithm_run_id uuid not null,
  attempt integer not null,
  status text not null,
  request_hash text not null,
  response_status integer,
  external_job_id text,
  duration_ms integer,
  error_category text,
  billing_json jsonb not null default '{}'::jsonb,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  constraint algorithm_run_attempts_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_run_attempts_run_project_fk foreign key (algorithm_run_id, project_id) references algorithm_runs(id, project_id) on delete cascade,
  constraint algorithm_run_attempts_status_valid check (status in ('running','succeeded','failed','timed_out','rate_limited')),
  constraint algorithm_run_attempts_attempt_valid check (attempt > 0 and (duration_ms is null or duration_ms >= 0)),
  constraint algorithm_run_attempts_run_attempt_unique unique (algorithm_run_id, attempt)
);
--> statement-breakpoint
create or replace function protect_published_algorithm_definition_version()
returns trigger language plpgsql as $$
begin
  if old.status in ('published','retired') then
    raise exception 'published algorithm definition versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger algorithm_definition_versions_published_immutable
before update or delete on algorithm_definition_versions
for each row execute function protect_published_algorithm_definition_version();
--> statement-breakpoint
create index algorithm_providers_project_status_idx on algorithm_providers(project_id, status);
--> statement-breakpoint
create index algorithm_definitions_project_provider_idx on algorithm_definitions(project_id, provider_id);
--> statement-breakpoint
create index algorithm_definition_versions_project_status_idx on algorithm_definition_versions(project_id, status);
--> statement-breakpoint
create index algorithm_runs_claim_idx on algorithm_runs(status, created_at);
--> statement-breakpoint
create index algorithm_runs_project_created_idx on algorithm_runs(project_id, created_at desc);
--> statement-breakpoint
create index algorithm_run_attempts_run_idx on algorithm_run_attempts(algorithm_run_id, attempt);
