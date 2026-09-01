create table connector_object_upload_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  operation_kind text not null,
  source_asset_id integer not null,
  requested_by_user_id integer not null,
  idempotency_key text not null,
  requested_name text not null,
  reconciliation_name text not null,
  status text not null default 'queued',
  object_key_digest text,
  object_key_envelope_json jsonb,
  notification_attempt_count integer not null default 0,
  reconciliation_miss_count integer not null default 0,
  last_error_code text,
  remote_resource_id bigint,
  uploaded_at timestamptz,
  notification_attempted_at timestamptz,
  reconciled_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_object_upload_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_object_upload_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_object_upload_jobs_asset_project_fk
    foreign key(source_asset_id,project_id) references assets(id,project_id) on delete restrict,
  constraint connector_object_upload_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_object_upload_jobs_remote_project_fk
    foreign key(remote_resource_id,project_id) references connector_remote_resources(id,project_id) on delete set null (remote_resource_id),
  constraint connector_object_upload_jobs_status_valid
    check(status in('queued','uploading','notifying','reconciling','succeeded','failed')),
  constraint connector_object_upload_jobs_operation_kind_valid
    check(operation_kind ~ '^[a-z][a-z0-9-]{0,63}$'),
  constraint connector_object_upload_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_object_upload_jobs_name_valid
    check(length(btrim(requested_name)) between 1 and 200 and requested_name=btrim(requested_name)
      and length(btrim(reconciliation_name)) between 1 and 240 and reconciliation_name=btrim(reconciliation_name)),
  constraint connector_object_upload_jobs_digest_valid
    check(object_key_digest is null or object_key_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_object_upload_jobs_envelope_object
    check(object_key_envelope_json is null or jsonb_typeof(object_key_envelope_json)='object'),
  constraint connector_object_upload_jobs_attempts_valid
    check(notification_attempt_count between 0 and 2 and reconciliation_miss_count between 0 and 8),
  constraint connector_object_upload_jobs_upload_checkpoint
    check((object_key_digest is null)=(object_key_envelope_json is null)
      and (uploaded_at is null)=(object_key_envelope_json is null)),
  constraint connector_object_upload_jobs_completion_valid
    check((status='succeeded')=(completed_at is not null)
      and (status<>'succeeded' or remote_resource_id is not null)),
  constraint connector_object_upload_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,operation_kind,idempotency_key),
  constraint connector_object_upload_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_object_upload_jobs_pending_idx
  on connector_object_upload_jobs(connector_instance_id,operation_kind,status,updated_at)
  where status not in('succeeded','failed');
