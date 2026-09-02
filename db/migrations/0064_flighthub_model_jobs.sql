create table connector_model_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  requested_by_user_id integer not null,
  action_kind text not null,
  idempotency_key text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
  reconciliation_name text,
  status text not null default 'queued',
  remote_ids_json jsonb not null default '[]'::jsonb,
  asset_ids_json jsonb not null default '[]'::jsonb,
  progress integer not null default 0,
  stage text not null default 'queued',
  submit_attempt_count integer not null default 0,
  reconciliation_count integer not null default 0,
  last_error_code text,
  submitted_at timestamptz,
  reconciled_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_model_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_model_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_model_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_model_jobs_action_valid
    check(action_kind in('traditional-create','open-start','open-stop')),
  constraint connector_model_jobs_status_valid
    check(status in('queued','reconciling','succeeded','failed','blocked')),
  constraint connector_model_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_model_jobs_digest_valid check(request_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_model_jobs_envelope_object check(jsonb_typeof(request_envelope_json)='object'),
  constraint connector_model_jobs_remote_array check(jsonb_typeof(remote_ids_json)='array'),
  constraint connector_model_jobs_asset_array check(jsonb_typeof(asset_ids_json)='array'),
  constraint connector_model_jobs_progress_valid check(progress between 0 and 100),
  constraint connector_model_jobs_attempts_valid
    check(submit_attempt_count between 0 and 1 and reconciliation_count between 0 and 32),
  constraint connector_model_jobs_completion_valid
    check((status='succeeded')=(completed_at is not null)),
  constraint connector_model_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_model_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_model_jobs_pending_idx
  on connector_model_jobs(connector_instance_id,status,updated_at)
  where status in('queued','reconciling');
