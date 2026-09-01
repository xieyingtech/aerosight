alter table observations
  add column task_run_id integer,
  add constraint observations_task_run_project_fk
    foreign key(task_run_id,project_id) references task_runs(id,project_id)
    on delete set null (task_run_id);
--> statement-breakpoint
create index observations_task_run_time_idx
  on observations(project_id,task_run_id,captured_at)
  where task_run_id is not null;
