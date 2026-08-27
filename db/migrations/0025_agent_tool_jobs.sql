create table agent_tool_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  session_id integer not null,
  requested_by_user_id integer not null,
  tool_name text not null,
  required_permission text not null,
  args_json jsonb not null default '{}'::jsonb,
  status text not null default 'queued',
  context_expires_at timestamptz not null,
  authorization_checked_at timestamptz,
  started_at timestamptz,
  finished_at timestamptz,
  failure_code text,
  result_json jsonb,
  created_at timestamptz not null default now(),
  constraint agent_tool_jobs_status_valid check(status in('queued','running','succeeded','failed')),
  constraint agent_tool_jobs_expiry_valid check(context_expires_at > created_at),
  constraint agent_tool_jobs_session_project_fk foreign key(session_id,project_id) references agent_sessions(id,project_id) on delete cascade,
  constraint agent_tool_jobs_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

create index agent_tool_jobs_claim_idx on agent_tool_jobs(status,created_at) where status='queued';
create index agent_tool_jobs_session_idx on agent_tool_jobs(session_id,created_at desc);
