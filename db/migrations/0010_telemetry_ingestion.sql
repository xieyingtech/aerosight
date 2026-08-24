create table telemetry_event_dedup (
  adapter_id bigint not null,
  event_id text not null,
  project_id integer not null,
  captured_at timestamptz not null,
  received_at timestamptz not null default now(),
  primary key (adapter_id, event_id),
  constraint telemetry_event_dedup_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade
);
--> statement-breakpoint
create index telemetry_event_dedup_received_idx on telemetry_event_dedup(received_at);
--> statement-breakpoint
create table device_latest_telemetry (
  device_id integer primary key,
  project_id integer not null,
  adapter_id bigint not null,
  event_id text not null,
  telemetry_type text not null,
  sequence_number bigint,
  captured_at timestamptz not null,
  received_at timestamptz not null,
  payload_json jsonb not null default '{}'::jsonb,
  quality_json jsonb not null default '{}'::jsonb,
  updated_at timestamptz not null default now(),
  constraint device_latest_telemetry_device_project_fk
    foreign key (device_id, project_id) references devices(id, project_id) on delete cascade,
  constraint device_latest_telemetry_adapter_project_fk
    foreign key (adapter_id, project_id) references device_adapters(id, project_id) on delete cascade
);
--> statement-breakpoint
create index device_latest_telemetry_project_time_idx on device_latest_telemetry(project_id, captured_at desc);
