create table connector_open_model_uploads (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  requested_by_user_id integer not null,
  idempotency_key text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
  resource_uuid_digest text not null,
  status text not null default 'requested',
  credential_envelope_json jsonb,
  credential_expires_at timestamptz,
  callback_digest text,
  callback_envelope_json jsonb,
  callback_attempt_count integer not null default 0,
  reconciliation_count integer not null default 0,
  last_error_code text,
  remote_resource_id bigint,
  asset_id integer,
  credential_issued_at timestamptz,
  callback_attempted_at timestamptz,
  reconciled_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_open_model_uploads_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_open_model_uploads_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_open_model_uploads_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_open_model_uploads_remote_project_fk
    foreign key(remote_resource_id,project_id) references connector_remote_resources(id,project_id) on delete set null (remote_resource_id),
  constraint connector_open_model_uploads_asset_project_fk
    foreign key(asset_id,project_id) references assets(id,project_id) on delete set null (asset_id),
  constraint connector_open_model_uploads_status_valid
    check(status in('requested','credential_ready','callback_pending','reconciling','succeeded','expired','failed','blocked')),
  constraint connector_open_model_uploads_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_open_model_uploads_digest_valid
    check(request_digest ~ '^[a-f0-9]{64}$' and resource_uuid_digest ~ '^[a-f0-9]{64}$'
      and (callback_digest is null or callback_digest ~ '^[a-f0-9]{64}$')),
  constraint connector_open_model_uploads_envelope_object
    check(jsonb_typeof(request_envelope_json)='object'
      and (credential_envelope_json is null or jsonb_typeof(credential_envelope_json)='object')
      and (callback_envelope_json is null or jsonb_typeof(callback_envelope_json)='object')),
  constraint connector_open_model_uploads_callback_pair
    check((callback_envelope_json is null or callback_digest is not null)
      and (status not in('callback_pending','reconciling') or (callback_digest is not null and callback_envelope_json is not null))),
  constraint connector_open_model_uploads_attempts_valid
    check(callback_attempt_count between 0 and 1 and reconciliation_count between 0 and 16),
  constraint connector_open_model_uploads_completion_valid
    check((status='succeeded')=(completed_at is not null)
      and (status<>'succeeded' or (remote_resource_id is not null and asset_id is not null))),
  constraint connector_open_model_uploads_project_idempotency_unique
    unique(project_id,connector_instance_id,idempotency_key),
  constraint connector_open_model_uploads_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_open_model_uploads_pending_idx
  on connector_open_model_uploads(connector_instance_id,status,updated_at)
  where status not in('succeeded','expired','failed','blocked');
