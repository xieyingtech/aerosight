create table alert_automation_policies (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  name text not null,
  current_published_version_id bigint,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint alert_automation_policies_project_name_unique unique(project_id,name),
  constraint alert_automation_policies_id_project_unique unique(id,project_id),
  constraint alert_automation_policies_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

create table alert_automation_policy_versions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  alert_automation_policy_id bigint not null,
  event_rule_version_id bigint,
  version integer not null,
  status text not null default 'draft',
  mode text not null default 'manual',
  config_json jsonb not null default '{}'::jsonb,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint alert_automation_policy_versions_status_valid check(status in('draft','published','retired')),
  constraint alert_automation_policy_versions_mode_valid check(mode in('manual','agent-on-demand','agent-auto-draft','follow-up-draft')),
  constraint alert_automation_policy_versions_version_positive check(version>0),
  constraint alert_automation_policy_versions_policy_version_unique unique(alert_automation_policy_id,version),
  constraint alert_automation_policy_versions_id_project_unique unique(id,project_id),
  constraint alert_automation_policy_versions_policy_project_fk foreign key(alert_automation_policy_id,project_id) references alert_automation_policies(id,project_id) on delete cascade,
  constraint alert_automation_policy_versions_rule_project_fk foreign key(event_rule_version_id,project_id) references event_rule_versions(id,project_id) on delete restrict,
  constraint alert_automation_policy_versions_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

alter table alert_automation_policies add constraint alert_automation_policies_current_version_project_fk
  foreign key(current_published_version_id,project_id) references alert_automation_policy_versions(id,project_id) on delete set null;

create table alert_automation_runs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  policy_version_id bigint not null,
  perception_event_id uuid not null,
  trigger_reason text not null,
  status text not null default 'queued',
  input_scope_json jsonb not null default '{}'::jsonb,
  output_refs_json jsonb not null default '[]'::jsonb,
  failure_code text,
  failure_message text,
  queued_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  constraint alert_automation_runs_status_valid check(status in('queued','running','succeeded','failed','canceled')),
  constraint alert_automation_runs_id_project_unique unique(id,project_id),
  constraint alert_automation_runs_policy_project_fk foreign key(policy_version_id,project_id) references alert_automation_policy_versions(id,project_id) on delete restrict,
  constraint alert_automation_runs_event_project_fk foreign key(perception_event_id,project_id) references perception_events(id,project_id) on delete restrict,
  constraint alert_automation_runs_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

create unique index alert_automation_policy_versions_one_draft_idx on alert_automation_policy_versions(alert_automation_policy_id) where status='draft';
create index alert_automation_policy_versions_project_status_idx on alert_automation_policy_versions(project_id,status);
create index alert_automation_runs_claim_idx on alert_automation_runs(status,queued_at) where status='queued';
create index alert_automation_runs_project_event_idx on alert_automation_runs(project_id,perception_event_id,created_at desc);
