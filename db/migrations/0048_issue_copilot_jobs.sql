alter table issue_events add constraint issue_events_id_project_unique unique(id,project_id);
--> statement-breakpoint
alter table agent_sessions
  add constraint agent_sessions_issue_project_fk foreign key(issue_id,project_id) references issues(id,project_id) on delete set null(issue_id),
  add constraint agent_sessions_agent_project_fk foreign key(agent_id,project_id) references agents(id,project_id) on delete set null(agent_id);
--> statement-breakpoint
alter table agent_tool_jobs
  add column issue_id integer,
  add column trigger_issue_event_id integer,
  add column trigger_type text,
  add column idempotency_key text,
  add constraint agent_tool_jobs_issue_project_fk foreign key(issue_id,project_id) references issues(id,project_id) on delete cascade,
  add constraint agent_tool_jobs_trigger_event_project_fk foreign key(trigger_issue_event_id,project_id) references issue_events(id,project_id) on delete cascade,
  add constraint agent_tool_jobs_trigger_type_valid check(trigger_type is null or trigger_type in('issue_mention','issue_assignment','task_step','chat'));
--> statement-breakpoint
create unique index agent_tool_jobs_project_idempotency_unique
  on agent_tool_jobs(project_id,idempotency_key) where idempotency_key is not null;
--> statement-breakpoint
create index agent_tool_jobs_issue_created_idx on agent_tool_jobs(issue_id,created_at desc) where issue_id is not null;
