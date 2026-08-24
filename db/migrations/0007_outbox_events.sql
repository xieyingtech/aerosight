create table outbox_events (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  event_id text not null,
  event_type text not null,
  aggregate_type text,
  aggregate_id text,
  payload_json jsonb not null default '{}'::jsonb,
  status text not null default 'pending',
  attempts integer not null default 0,
  max_attempts integer not null default 8,
  available_at timestamptz not null default now(),
  locked_by text,
  locked_until timestamptz,
  last_error text,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  constraint outbox_events_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint outbox_events_event_unique unique (event_id),
  constraint outbox_events_status_valid
    check (status in ('pending', 'processing', 'completed', 'dead')),
  constraint outbox_events_attempts_valid
    check (attempts >= 0 and max_attempts > 0)
);
--> statement-breakpoint
create index outbox_events_claim_idx on outbox_events(status, available_at, locked_until, id);
--> statement-breakpoint
create index outbox_events_project_created_idx on outbox_events(project_id, created_at, id);
--> statement-breakpoint
create table outbox_consumptions (
  consumer_name text not null,
  event_id text not null,
  consumed_at timestamptz not null default now(),
  primary key (consumer_name, event_id),
  foreign key (event_id) references outbox_events(event_id) on delete cascade
);
