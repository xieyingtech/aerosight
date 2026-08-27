create table alert_automation_drafts (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  automation_run_id uuid not null,
  perception_event_id uuid not null,
  draft_type text not null,
  status text not null default 'draft',
  title text not null,
  payload_json jsonb not null default '{}'::jsonb,
  evidence_refs_json jsonb not null default '[]'::jsonb,
  created_at timestamptz not null default now(),
  constraint alert_automation_drafts_type_valid check(draft_type in('report','issue','follow-up-task')),
  constraint alert_automation_drafts_status_valid check(status in('draft','discarded','published')),
  constraint alert_automation_drafts_run_type_unique unique(automation_run_id,draft_type),
  constraint alert_automation_drafts_run_project_fk foreign key(automation_run_id,project_id) references alert_automation_runs(id,project_id) on delete cascade,
  constraint alert_automation_drafts_event_project_fk foreign key(perception_event_id,project_id) references perception_events(id,project_id) on delete restrict,
  constraint alert_automation_drafts_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

create index alert_automation_drafts_project_event_idx on alert_automation_drafts(project_id,perception_event_id,created_at desc);
