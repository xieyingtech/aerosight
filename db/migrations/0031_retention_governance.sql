create table retention_policies (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  policy_key text not null,
  version integer not null,
  status text not null default 'draft',
  retention_days integer not null,
  derivative_retention_days integer not null,
  is_default boolean not null default false,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint retention_policies_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint retention_policies_created_by_fk foreign key(created_by_user_id) references users(id) on delete set null,
  constraint retention_policies_published_by_fk foreign key(published_by_user_id) references users(id) on delete set null,
  constraint retention_policies_status_valid check(status in('draft','published','retired')),
  constraint retention_policies_duration_valid check(retention_days>0 and derivative_retention_days>0),
  constraint retention_policies_version_valid check(version>0),
  constraint retention_policies_key_version_unique unique(project_id,policy_key,version),
  constraint retention_policies_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create unique index retention_policies_one_default_idx on retention_policies(project_id)
  where status='published' and is_default;
--> statement-breakpoint
create table retention_holds (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  asset_id integer not null,
  reason text not null,
  status text not null default 'active',
  hold_until timestamptz,
  created_by_user_id integer,
  released_by_user_id integer,
  created_at timestamptz not null default now(),
  released_at timestamptz,
  constraint retention_holds_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint retention_holds_asset_project_fk foreign key(asset_id,project_id) references assets(id,project_id) on delete cascade,
  constraint retention_holds_created_by_fk foreign key(created_by_user_id) references users(id) on delete set null,
  constraint retention_holds_released_by_fk foreign key(released_by_user_id) references users(id) on delete set null,
  constraint retention_holds_status_valid check(status in('active','released')),
  constraint retention_holds_release_complete check((status='active' and released_at is null) or (status='released' and released_at is not null)),
  constraint retention_holds_reason_present check(length(trim(reason))>0)
);
--> statement-breakpoint
create unique index retention_holds_one_active_asset_idx on retention_holds(project_id,asset_id)
  where status='active';
--> statement-breakpoint
create table retention_cleanup_runs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  retention_policy_id bigint not null,
  mode text not null default 'dry_run',
  status text not null default 'planned',
  plan_json jsonb not null default '{}'::jsonb,
  candidate_count integer not null default 0,
  deleted_count integer not null default 0,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  error_code text,
  constraint retention_cleanup_runs_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint retention_cleanup_runs_policy_project_fk foreign key(retention_policy_id,project_id) references retention_policies(id,project_id) on delete restrict,
  constraint retention_cleanup_runs_created_by_fk foreign key(created_by_user_id) references users(id) on delete set null,
  constraint retention_cleanup_runs_mode_valid check(mode in('dry_run','execute')),
  constraint retention_cleanup_runs_status_valid check(status in('planned','running','completed','failed')),
  constraint retention_cleanup_runs_counts_valid check(candidate_count>=0 and deleted_count>=0 and deleted_count<=candidate_count),
  constraint retention_cleanup_runs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create table retention_deletion_tombstones (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  cleanup_run_id uuid not null,
  retention_policy_id bigint not null,
  asset_id integer not null,
  storage_key_hash text not null,
  checksum_sha256 text,
  reason_code text not null,
  deleted_at timestamptz not null default now(),
  constraint retention_tombstones_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint retention_tombstones_run_project_fk foreign key(cleanup_run_id,project_id) references retention_cleanup_runs(id,project_id) on delete restrict,
  constraint retention_tombstones_policy_project_fk foreign key(retention_policy_id,project_id) references retention_policies(id,project_id) on delete restrict,
  constraint retention_tombstones_asset_project_fk foreign key(asset_id,project_id) references assets(id,project_id) on delete restrict,
  constraint retention_tombstones_storage_hash_valid check(storage_key_hash ~ '^[a-f0-9]{64}$'),
  constraint retention_tombstones_checksum_valid check(checksum_sha256 is null or checksum_sha256 ~ '^[a-f0-9]{64}$'),
  constraint retention_tombstones_asset_unique unique(asset_id)
);
--> statement-breakpoint
create index retention_cleanup_runs_project_created_idx on retention_cleanup_runs(project_id,created_at desc);
--> statement-breakpoint
create index retention_tombstones_project_deleted_idx on retention_deletion_tombstones(project_id,deleted_at desc);
--> statement-breakpoint
create or replace function protect_published_retention_policy()
returns trigger language plpgsql as $$
begin
  if old.status='published' then
    raise exception 'published retention policy is immutable' using errcode='55000';
  end if;
  return case when tg_op='DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger retention_policies_published_immutable before update or delete on retention_policies
for each row execute function protect_published_retention_policy();
