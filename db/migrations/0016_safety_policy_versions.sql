create table safety_policy_versions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  version integer not null,
  status text not null default 'draft',
  project_boundary geometry(Polygon, 4326),
  restricted_areas geometry(MultiPolygon, 4326),
  max_altitude_meters double precision not null,
  max_speed_meters_per_second double precision not null,
  minimum_battery_percent double precision not null,
  allowed_windows_json jsonb not null default '[]'::jsonb,
  required_compliance_json jsonb not null default '[]'::jsonb,
  optional_compliance_json jsonb not null default '[]'::jsonb,
  exemptions_json jsonb not null default '[]'::jsonb,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint safety_policy_versions_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint safety_policy_versions_created_by_fk foreign key (created_by_user_id) references users(id) on delete set null,
  constraint safety_policy_versions_published_by_fk foreign key (published_by_user_id) references users(id) on delete set null,
  constraint safety_policy_versions_status_valid check (status in ('draft', 'published')),
  constraint safety_policy_versions_limits_valid check (
    max_altitude_meters > 0 and max_speed_meters_per_second > 0
    and minimum_battery_percent between 0 and 100
  ),
  constraint safety_policy_versions_project_version_unique unique (project_id, version),
  constraint safety_policy_versions_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index safety_policy_versions_one_draft_idx
  on safety_policy_versions(project_id) where status = 'draft';
--> statement-breakpoint
create index safety_policy_versions_boundary_gist on safety_policy_versions using gist(project_boundary);
--> statement-breakpoint
create index safety_policy_versions_restricted_gist on safety_policy_versions using gist(restricted_areas);
--> statement-breakpoint
alter table projects add column current_safety_policy_version_id bigint;
--> statement-breakpoint
alter table projects add constraint projects_current_safety_policy_project_fk
  foreign key (current_safety_policy_version_id, id)
  references safety_policy_versions(id, project_id) on delete set null (current_safety_policy_version_id);
--> statement-breakpoint
create or replace function protect_published_safety_policy_version()
returns trigger language plpgsql as $$
begin
  if old.status = 'published' then
    raise exception 'published safety policy versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger safety_policy_versions_published_immutable
before update or delete on safety_policy_versions
for each row execute function protect_published_safety_policy_version();
