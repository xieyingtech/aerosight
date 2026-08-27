alter table agent_sessions add constraint agent_sessions_id_project_unique unique(id,project_id);

create table agent_drafts (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  session_id integer not null,
  created_by_user_id integer not null,
  draft_type text not null,
  status text not null default 'draft',
  title text not null,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint agent_drafts_type_valid check(draft_type in('inspection_task','report','issue')),
  constraint agent_drafts_status_valid check(status in('draft','discarded','published')),
  constraint agent_drafts_id_project_unique unique(id,project_id),
  constraint agent_drafts_session_project_fk foreign key(session_id,project_id) references agent_sessions(id,project_id) on delete cascade,
  constraint agent_drafts_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint agent_drafts_actor_team_fk foreign key(team_id,created_by_user_id) references team_members(team_id,user_id) on delete restrict
);

create table agent_draft_evidence (
  id bigserial primary key,
  project_id integer not null,
  agent_draft_id uuid not null,
  reference_type text not null,
  reference_id text not null,
  reference_version text not null,
  observed_at timestamptz not null,
  quality text not null,
  created_at timestamptz not null default now(),
  constraint agent_draft_evidence_type_valid check(reference_type in('asset','event','detection','track','task_run')),
  constraint agent_draft_evidence_draft_fk foreign key(agent_draft_id,project_id) references agent_drafts(id,project_id) on delete cascade,
  constraint agent_draft_evidence_unique unique(agent_draft_id,reference_type,reference_id,reference_version)
);

create index agent_drafts_project_created_idx on agent_drafts(project_id,created_at desc);
create index agent_drafts_session_created_idx on agent_drafts(session_id,created_at desc);
create index agent_draft_evidence_project_ref_idx on agent_draft_evidence(project_id,reference_type,reference_id);
