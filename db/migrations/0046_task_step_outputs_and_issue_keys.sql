alter table task_run_steps
  add column input_snapshot_json jsonb not null default '{}'::jsonb,
  add column output_snapshot_json jsonb not null default '{}'::jsonb,
  add column condition_result_json jsonb,
  add column execution_key text,
  add constraint task_run_steps_input_object check (jsonb_typeof(input_snapshot_json)='object'),
  add constraint task_run_steps_output_object check (jsonb_typeof(output_snapshot_json)='object'),
  add constraint task_run_steps_condition_object check (condition_result_json is null or jsonb_typeof(condition_result_json)='object'),
  add constraint task_run_steps_project_execution_unique unique(project_id,execution_key);
--> statement-breakpoint
alter table algorithm_runs
  add column task_run_step_id bigint,
  add constraint algorithm_runs_task_step_project_fk
    foreign key(task_run_step_id,project_id) references task_run_steps(id,project_id) on delete set null(task_run_step_id);
--> statement-breakpoint
create index algorithm_runs_task_step_idx on algorithm_runs(task_run_step_id) where task_run_step_id is not null;
--> statement-breakpoint
alter table issues
  add column task_version_id bigint,
  add column condition_scope_key text,
  add column business_object_key text,
  add column occurrence_count integer not null default 1,
  add column first_seen_at timestamptz not null default now(),
  add column last_seen_at timestamptz not null default now(),
  add column labels_json jsonb not null default '[]'::jsonb,
  add constraint issues_task_version_project_fk
    foreign key(task_version_id,project_id) references task_versions(id,project_id) on delete restrict,
  add constraint issues_occurrence_positive check(occurrence_count > 0),
  add constraint issues_labels_array check(jsonb_typeof(labels_json)='array');
--> statement-breakpoint
alter table issues add constraint issues_task_run_project_fk
  foreign key(task_run_id,project_id) references task_runs(id,project_id) on delete set null(task_run_id);
--> statement-breakpoint
alter table issue_links add constraint issue_links_issue_project_fk
  foreign key(issue_id,project_id) references issues(id,project_id) on delete cascade;
--> statement-breakpoint
alter table issue_events add constraint issue_events_issue_project_fk
  foreign key(issue_id,project_id) references issues(id,project_id) on delete cascade;
--> statement-breakpoint
create unique index issues_task_business_unique
  on issues(project_id,task_version_id,condition_scope_key,business_object_key)
  where task_version_id is not null and condition_scope_key is not null and business_object_key is not null;
