alter table task_runs
  add column team_id integer,
  add column selected_device_id integer,
  add column safety_policy_version_id bigint,
  add column approval_request_id uuid,
  add column state_version integer not null default 0,
  add column current_step_position integer,
  add column preflight_snapshot_json jsonb not null default '{}'::jsonb,
  add column state_reason text;
--> statement-breakpoint
update task_runs run set team_id = project.team_id from projects project where project.id = run.project_id;
--> statement-breakpoint
alter table task_runs
  alter column team_id set not null,
  add constraint task_runs_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  add constraint task_runs_device_project_fk foreign key (selected_device_id, project_id) references devices(id, project_id) on delete set null (selected_device_id),
  add constraint task_runs_policy_project_fk foreign key (safety_policy_version_id, project_id) references safety_policy_versions(id, project_id) on delete restrict,
  add constraint task_runs_approval_project_fk foreign key (approval_request_id, project_id) references approval_requests(id, project_id) on delete restrict,
  add constraint task_runs_status_valid check (status in ('queued','blocked','ready','dispatching','running','paused','succeeded','failed','canceling','canceled')),
  add constraint task_runs_state_version_valid check (state_version >= 0),
  add constraint task_runs_current_step_valid check (current_step_position is null or current_step_position > 0);
--> statement-breakpoint
alter table task_steps add constraint task_steps_id_project_unique unique (id, project_id);
--> statement-breakpoint
create table task_run_steps (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  task_run_id integer not null,
  task_step_id bigint not null,
  position integer not null,
  status text not null default 'pending',
  attempt_count integer not null default 0,
  result_json jsonb not null default '{}'::jsonb,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  constraint task_run_steps_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint task_run_steps_run_project_fk foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete cascade,
  constraint task_run_steps_step_project_fk foreign key (task_step_id, project_id) references task_steps(id, project_id) on delete restrict,
  constraint task_run_steps_status_valid check (status in ('pending','dispatching','running','succeeded','failed','skipped','paused')),
  constraint task_run_steps_position_valid check (position > 0 and attempt_count >= 0),
  constraint task_run_steps_run_position_unique unique (task_run_id, position),
  constraint task_run_steps_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table device_commands (
  id uuid primary key,
  project_id integer not null,
  team_id integer not null,
  task_run_id integer not null,
  task_run_step_id bigint,
  device_id integer not null,
  command_key text not null,
  idempotency_key text not null,
  capability_code text not null,
  parameters_json jsonb not null default '{}'::jsonb,
  safety_context_json jsonb not null default '{}'::jsonb,
  status text not null default 'pending',
  priority integer not null default 0,
  deadline_at timestamptz not null,
  requested_by_user_id integer,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  result_json jsonb not null default '{}'::jsonb,
  constraint device_commands_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint device_commands_run_project_fk foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete cascade,
  constraint device_commands_run_step_project_fk foreign key (task_run_step_id, project_id) references task_run_steps(id, project_id) on delete cascade,
  constraint device_commands_device_project_fk foreign key (device_id, project_id) references devices(id, project_id) on delete restrict,
  constraint device_commands_requester_fk foreign key (requested_by_user_id) references users(id) on delete set null,
  constraint device_commands_status_valid check (status in ('pending','dispatchable','sent','acknowledged','nacked','timed_out','canceled','unknown')),
  constraint device_commands_priority_valid check (priority between 0 and 100),
  constraint device_commands_device_idempotency_unique unique (device_id, idempotency_key),
  constraint device_commands_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table command_attempts (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  command_id uuid not null,
  adapter_id bigint not null,
  attempt integer not null,
  status text not null,
  sent_at timestamptz not null default now(),
  acknowledged_at timestamptz,
  error_code text,
  result_json jsonb not null default '{}'::jsonb,
  constraint command_attempts_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint command_attempts_command_project_fk foreign key (command_id, project_id) references device_commands(id, project_id) on delete cascade,
  constraint command_attempts_adapter_project_fk foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete restrict,
  constraint command_attempts_status_valid check (status in ('sent','acknowledged','nacked','timed_out','transport_error')),
  constraint command_attempts_attempt_valid check (attempt > 0),
  constraint command_attempts_command_attempt_unique unique (command_id, attempt)
);
--> statement-breakpoint
create index task_run_steps_run_status_idx on task_run_steps(task_run_id, status, position);
--> statement-breakpoint
create index device_commands_dispatch_idx on device_commands(status, priority desc, deadline_at);
--> statement-breakpoint
create index device_commands_run_created_idx on device_commands(task_run_id, created_at);
--> statement-breakpoint
create index command_attempts_command_idx on command_attempts(command_id, attempt);
