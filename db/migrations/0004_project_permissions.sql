create unique index projects_id_team_unique on projects(id, team_id);
--> statement-breakpoint
create table project_permissions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  user_id integer not null,
  permission text not null,
  granted_by_user_id integer references users(id) on delete set null,
  created_at timestamptz not null default now(),
  constraint project_permissions_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint project_permissions_team_member_fk
    foreign key (team_id, user_id) references team_members(team_id, user_id) on delete cascade,
  constraint project_permissions_unique unique (project_id, user_id, permission)
);
--> statement-breakpoint
create index project_permissions_user_project_idx on project_permissions(user_id, project_id);
--> statement-breakpoint
create index project_permissions_project_permission_idx on project_permissions(project_id, permission);
