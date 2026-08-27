alter table assets
  add column team_id integer,
  add column logical_key text,
  add column version integer not null default 1,
  add column status text not null default 'available',
  add column object_version text,
  add column checksum_sha256 text,
  add column available_at timestamptz,
  add column failed_at timestamptz,
  add column failure_code text,
  add column retention_hold_until timestamptz,
  add column legal_hold boolean not null default false,
  add column retention_reason text,
  add column deleted_at timestamptz,
  add column supersedes_asset_id integer;
--> statement-breakpoint
update assets asset
   set team_id = project.team_id,
       logical_key = 'legacy/' || asset.id::text,
       checksum_sha256 = case
         when asset.checksum ~ '^[A-Fa-f0-9]{64}$' then lower(asset.checksum)
         else null
       end,
       available_at = coalesce(asset.created_at at time zone 'UTC', now())
  from projects project
 where project.id = asset.project_id;
--> statement-breakpoint
alter table assets
  alter column team_id set not null,
  alter column logical_key set not null,
  add constraint assets_status_valid check (status in ('pending', 'available', 'failed', 'deleted')),
  add constraint assets_version_positive check (version > 0),
  add constraint assets_checksum_sha256_valid check (checksum_sha256 is null or checksum_sha256 ~ '^[a-f0-9]{64}$'),
  add constraint assets_id_project_unique unique (id, project_id),
  add constraint assets_project_logical_version_unique unique (project_id, logical_key, version),
  add constraint assets_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  add constraint assets_supersedes_project_fk
    foreign key (supersedes_asset_id, project_id) references assets(id, project_id) on delete set null (supersedes_asset_id);
--> statement-breakpoint
alter table task_runs add constraint task_runs_id_project_unique unique (id, project_id);
--> statement-breakpoint
alter table issues add constraint issues_id_project_unique unique (id, project_id);
--> statement-breakpoint
alter table assets
  add constraint assets_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete set null (device_id),
  add constraint assets_task_run_project_fk
    foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete set null (task_run_id),
  add constraint assets_issue_project_fk
    foreign key (issue_id, project_id) references issues(id, project_id) on delete set null (issue_id);
--> statement-breakpoint
create table asset_upload_intents (
  id uuid primary key,
  project_id integer not null,
  team_id integer not null,
  actor_user_id integer,
  logical_key text not null,
  object_key text not null,
  file_name text not null,
  kind text not null,
  mime_type text not null,
  expected_size_bytes bigint not null,
  expected_checksum_sha256 text not null,
  device_id integer,
  task_run_id integer,
  issue_id integer,
  status text not null default 'pending',
  asset_id integer,
  failure_code text,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint asset_upload_intents_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint asset_upload_intents_actor_fk
    foreign key (actor_user_id) references users(id) on delete set null,
  constraint asset_upload_intents_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete set null (device_id),
  constraint asset_upload_intents_task_run_project_fk
    foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete set null (task_run_id),
  constraint asset_upload_intents_issue_project_fk
    foreign key (issue_id, project_id) references issues(id, project_id) on delete set null (issue_id),
  constraint asset_upload_intents_asset_project_fk
    foreign key (asset_id, project_id) references assets(id, project_id) on delete set null (asset_id),
  constraint asset_upload_intents_status_valid
    check (status in ('pending', 'completed', 'failed', 'expired')),
  constraint asset_upload_intents_size_valid check (expected_size_bytes >= 0),
  constraint asset_upload_intents_checksum_valid check (expected_checksum_sha256 ~ '^[a-f0-9]{64}$'),
  constraint asset_upload_intents_project_object_unique unique (project_id, object_key)
);
--> statement-breakpoint
create table asset_derivatives (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  source_asset_id integer not null,
  derived_asset_id integer not null,
  derivative_type text not null,
  generator text not null,
  generator_version text,
  parameters_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  constraint asset_derivatives_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint asset_derivatives_source_project_fk
    foreign key (source_asset_id, project_id) references assets(id, project_id) on delete cascade,
  constraint asset_derivatives_derived_project_fk
    foreign key (derived_asset_id, project_id) references assets(id, project_id) on delete cascade,
  constraint asset_derivatives_not_self check (source_asset_id <> derived_asset_id),
  constraint asset_derivatives_unique unique (source_asset_id, derived_asset_id, derivative_type)
);
--> statement-breakpoint
create table evidence_links (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  target_type text not null,
  target_id text not null,
  asset_id integer not null,
  asset_version integer not null,
  asset_checksum_sha256 text not null,
  start_offset_ms bigint,
  end_offset_ms bigint,
  is_published boolean not null default false,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  constraint evidence_links_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint evidence_links_asset_project_fk
    foreign key (asset_id, project_id) references assets(id, project_id) on delete restrict,
  constraint evidence_links_created_by_fk
    foreign key (created_by_user_id) references users(id) on delete set null,
  constraint evidence_links_target_type_valid
    check (target_type in ('detection', 'track', 'event', 'report', 'issue', 'task_run')),
  constraint evidence_links_offsets_valid check (
    (start_offset_ms is null and end_offset_ms is null) or
    (start_offset_ms is not null and start_offset_ms >= 0 and end_offset_ms is not null and end_offset_ms > start_offset_ms)
  ),
  constraint evidence_links_version_positive check (asset_version > 0),
  constraint evidence_links_checksum_valid check (asset_checksum_sha256 ~ '^[a-f0-9]{64}$'),
  constraint evidence_links_unique
    unique (project_id, target_type, target_id, asset_id, start_offset_ms, end_offset_ms)
);
--> statement-breakpoint
create or replace function protect_published_evidence_link()
returns trigger language plpgsql as $$
begin
  if old.is_published then
    raise exception 'published evidence links are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger evidence_links_published_immutable
before update or delete on evidence_links
for each row execute function protect_published_evidence_link();
--> statement-breakpoint
create index assets_project_status_created_idx on assets(project_id, status, created_at desc);
--> statement-breakpoint
create index assets_retention_idx on assets(project_id, retention_hold_until) where status = 'available';
--> statement-breakpoint
create index asset_upload_intents_expiry_idx on asset_upload_intents(status, expires_at);
--> statement-breakpoint
create index asset_derivatives_source_idx on asset_derivatives(project_id, source_asset_id);
--> statement-breakpoint
create index evidence_links_target_idx on evidence_links(project_id, target_type, target_id);
