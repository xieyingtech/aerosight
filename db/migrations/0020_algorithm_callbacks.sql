create table algorithm_callback_receipts (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  algorithm_run_id uuid not null,
  provider_id bigint not null,
  callback_id text not null,
  external_job_id text not null,
  payload_hash text not null,
  disposition text not null default 'verified',
  received_at timestamptz not null default now(),
  constraint algorithm_callback_receipts_project_team_fk foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint algorithm_callback_receipts_run_project_fk foreign key (algorithm_run_id, project_id) references algorithm_runs(id, project_id) on delete cascade,
  constraint algorithm_callback_receipts_provider_project_fk foreign key (provider_id, project_id) references algorithm_providers(id, project_id) on delete cascade,
  constraint algorithm_callback_receipts_hash_valid check (payload_hash ~ '^[a-f0-9]{64}$'),
  constraint algorithm_callback_receipts_disposition_valid check (disposition in ('verified','applied')),
  constraint algorithm_callback_receipts_provider_callback_unique unique (provider_id, callback_id)
);
--> statement-breakpoint
create index algorithm_callback_receipts_run_idx on algorithm_callback_receipts(algorithm_run_id, received_at desc);
