alter table issues
  add column state_version integer not null default 0,
  add constraint issues_state_version_nonnegative check(state_version >= 0);
--> statement-breakpoint
alter table issue_events add column client_key text;
--> statement-breakpoint
create unique index issue_events_client_key_unique
  on issue_events(project_id,issue_id,client_key) where client_key is not null;
--> statement-breakpoint
alter table agents add constraint agents_id_project_unique unique(id,project_id);
--> statement-breakpoint
create table issue_assignees (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  issue_id integer not null,
  assignee_type text not null,
  user_id integer,
  agent_id integer,
  assigned_by_user_id integer not null,
  active boolean not null default true,
  created_at timestamptz not null default now(),
  removed_at timestamptz,
  constraint issue_assignees_type_valid check(assignee_type in('user','agent')),
  constraint issue_assignees_subject_valid check(
    (assignee_type='user' and user_id is not null and agent_id is null) or
    (assignee_type='agent' and agent_id is not null and user_id is null)
  ),
  constraint issue_assignees_active_time_valid check(active=(removed_at is null)),
  constraint issue_assignees_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint issue_assignees_issue_project_fk foreign key(issue_id,project_id) references issues(id,project_id) on delete cascade,
  constraint issue_assignees_user_team_fk foreign key(team_id,user_id) references team_members(team_id,user_id) on delete cascade,
  constraint issue_assignees_agent_project_fk foreign key(agent_id,project_id) references agents(id,project_id) on delete cascade,
  constraint issue_assignees_actor_fk foreign key(assigned_by_user_id) references users(id) on delete restrict
);
--> statement-breakpoint
create unique index issue_assignees_active_user_unique on issue_assignees(issue_id,user_id) where active and user_id is not null;
--> statement-breakpoint
create unique index issue_assignees_active_agent_unique on issue_assignees(issue_id,agent_id) where active and agent_id is not null;
--> statement-breakpoint
create index issue_assignees_project_issue_idx on issue_assignees(project_id,issue_id) where active;
--> statement-breakpoint
insert into project_permissions(project_id,team_id,user_id,permission)
select project_id,team_id,user_id,'issue:handle' from project_permissions where permission='event:handle'
on conflict(project_id,user_id,permission) do nothing;
