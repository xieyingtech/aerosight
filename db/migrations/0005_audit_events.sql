create table audit_events (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  request_id text not null,
  idempotency_key text,
  actor_user_id integer references users(id) on delete set null,
  actor_agent_id integer references agents(id) on delete set null,
  action text not null,
  resource_type text not null,
  resource_id text,
  input_hash text not null,
  policy_result_json jsonb not null default '{}'::jsonb,
  result_hash text,
  status text not null default 'accepted',
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint audit_events_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint audit_events_actor_present
    check (actor_user_id is not null or actor_agent_id is not null),
  constraint audit_events_status_valid
    check (status in ('accepted', 'completed'))
);
--> statement-breakpoint
create index audit_events_project_created_idx on audit_events(project_id, created_at desc);
--> statement-breakpoint
create index audit_events_request_idx on audit_events(request_id);
--> statement-breakpoint
create index audit_events_resource_idx on audit_events(project_id, resource_type, resource_id);
