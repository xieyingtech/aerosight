create table live_streams (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  device_id integer not null,
  task_run_id integer,
  adapter_id bigint,
  stream_key text not null,
  source_type text not null,
  status text not null default 'starting',
  playback_ref text,
  playback_locator_expires_at timestamptz,
  status_reason text,
  started_by_user_id integer,
  started_at timestamptz not null default now(),
  last_active_at timestamptz,
  ended_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint live_streams_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint live_streams_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint live_streams_task_run_project_fk
    foreign key (task_run_id, project_id) references task_runs(id, project_id) on delete set null (task_run_id),
  constraint live_streams_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete set null (adapter_id),
  constraint live_streams_started_by_fk
    foreign key (started_by_user_id) references users(id) on delete set null,
  constraint live_streams_status_valid
    check (status in ('starting', 'live', 'degraded', 'failed', 'stopping', 'stopped')),
  constraint live_streams_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create unique index live_streams_one_active_device_key_idx
  on live_streams(project_id, device_id, stream_key)
  where status in ('starting', 'live', 'degraded', 'stopping');
--> statement-breakpoint
create index live_streams_project_status_idx on live_streams(project_id, status, started_at desc);
--> statement-breakpoint
create index live_streams_device_started_idx on live_streams(device_id, started_at desc);
