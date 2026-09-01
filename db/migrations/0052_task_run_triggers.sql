alter table task_runs
  add column trigger_key text,
  add constraint task_runs_trigger_key_normalized
    check (trigger_key is null or (length(trigger_key) between 3 and 512 and trigger_key = btrim(trigger_key)));
--> statement-breakpoint
create unique index task_runs_trigger_key_unique
  on task_runs(project_id, task_version_id, trigger_key)
  where task_version_id is not null and trigger_key is not null;
--> statement-breakpoint
create index task_runs_active_concurrency_idx
  on task_runs(project_id, task_version_id, status)
  where status in ('queued','blocked','ready','dispatching','running','paused','canceling');
