create table idempotency_records (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  actor_key text not null,
  operation text not null,
  idempotency_key text not null,
  request_hash text not null,
  status text not null default 'processing',
  response_json jsonb,
  error_code text,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  expires_at timestamptz not null default (now() + interval '24 hours'),
  constraint idempotency_records_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint idempotency_records_scope_unique
    unique (project_id, actor_key, operation, idempotency_key),
  constraint idempotency_records_status_valid
    check (status in ('processing', 'completed', 'failed'))
);
--> statement-breakpoint
create index idempotency_records_expiry_idx on idempotency_records(expires_at);
--> statement-breakpoint
create index idempotency_records_project_created_idx on idempotency_records(project_id, created_at desc);
