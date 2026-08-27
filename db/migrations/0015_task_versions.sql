alter table tasks
  add column team_id integer,
  add constraint tasks_id_project_unique unique (id, project_id);
--> statement-breakpoint
update tasks task set team_id = project.team_id
  from projects project where project.id = task.project_id;
--> statement-breakpoint
alter table tasks
  alter column team_id set not null,
  add constraint tasks_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade;
--> statement-breakpoint
create table task_versions (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  task_id integer not null,
  version integer not null,
  status text not null default 'draft',
  definition_json jsonb not null default '{}'::jsonb,
  script text not null,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint task_versions_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint task_versions_task_project_fk
    foreign key (task_id, project_id) references tasks(id, project_id) on delete cascade,
  constraint task_versions_created_by_fk foreign key (created_by_user_id) references users(id) on delete set null,
  constraint task_versions_published_by_fk foreign key (published_by_user_id) references users(id) on delete set null,
  constraint task_versions_status_valid check (status in ('draft', 'published', 'retired')),
  constraint task_versions_version_positive check (version > 0),
  constraint task_versions_task_version_unique unique (task_id, version),
  constraint task_versions_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index task_versions_one_draft_idx on task_versions(task_id) where status = 'draft';
--> statement-breakpoint
create table task_steps (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  task_version_id bigint not null,
  position integer not null,
  step_key text not null,
  name text not null,
  capability_code text,
  action text not null,
  parameters_json jsonb not null default '{}'::jsonb,
  failure_policy_json jsonb not null default '{}'::jsonb,
  media_requirements_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  constraint task_steps_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint task_steps_version_project_fk
    foreign key (task_version_id, project_id) references task_versions(id, project_id) on delete cascade,
  constraint task_steps_position_positive check (position > 0),
  constraint task_steps_version_position_unique unique (task_version_id, position),
  constraint task_steps_version_key_unique unique (task_version_id, step_key)
);
--> statement-breakpoint
insert into task_versions (
  project_id, team_id, task_id, version, status, definition_json, script,
  created_by_user_id, published_by_user_id, created_at, published_at
)
select task.project_id, task.team_id, task.id, 1, 'published',
       jsonb_build_object(
         'name', task.name, 'description', task.description, 'triggerType', task.trigger_type,
         'requiredCapabilityCode', task.required_capability_code,
         'targetSelector', task.target_selector_json, 'schedule', task.schedule,
         'eventRule', task.event_rule_json, 'legacy', true
       ),
       task.script, task.created_by_user_id, task.created_by_user_id,
       task.created_at at time zone 'UTC', task.created_at at time zone 'UTC'
  from tasks task;
--> statement-breakpoint
alter table tasks add column current_published_version_id bigint;
--> statement-breakpoint
update tasks task set current_published_version_id = version.id
  from task_versions version where version.task_id = task.id and version.version = 1;
--> statement-breakpoint
alter table tasks add constraint tasks_current_version_project_fk
  foreign key (current_published_version_id, project_id)
  references task_versions(id, project_id) on delete set null (current_published_version_id);
--> statement-breakpoint
alter table task_runs add column task_version_id bigint;
--> statement-breakpoint
update task_runs run set task_version_id = task.current_published_version_id
  from tasks task where task.id = run.task_id and task.project_id = run.project_id;
--> statement-breakpoint
alter table task_runs add constraint task_runs_version_project_fk
  foreign key (task_version_id, project_id)
  references task_versions(id, project_id) on delete restrict;
--> statement-breakpoint
create or replace function protect_published_task_version()
returns trigger language plpgsql as $$
begin
  if old.status in ('published', 'retired') then
    raise exception 'published task versions are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger task_versions_published_immutable
before update or delete on task_versions
for each row execute function protect_published_task_version();
--> statement-breakpoint
create or replace function protect_published_task_step()
returns trigger language plpgsql as $$
begin
  if exists (select 1 from task_versions where id = old.task_version_id and status in ('published', 'retired')) then
    raise exception 'published task steps are immutable' using errcode = '55000';
  end if;
  return case when tg_op = 'DELETE' then old else new end;
end;
$$;
--> statement-breakpoint
create trigger task_steps_published_immutable
before update or delete on task_steps
for each row execute function protect_published_task_step();
--> statement-breakpoint
create index task_versions_project_status_idx on task_versions(project_id, status, created_at desc);
--> statement-breakpoint
create index task_steps_version_position_idx on task_steps(task_version_id, position);
