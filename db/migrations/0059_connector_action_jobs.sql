create table connector_action_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  task_run_id integer not null,
  device_id integer not null,
  wayline_resource_id bigint,
  target_resource_id bigint,
  remote_result_resource_id bigint,
  approval_request_id uuid not null,
  requested_by_user_id integer not null,
  action_kind text not null,
  idempotency_key text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
  status text not null default 'queued',
  dispatch_check_json jsonb not null default '{}'::jsonb,
  attempt_count integer not null default 0,
  reconciliation_count integer not null default 0,
  last_error_code text,
  accepted_at timestamptz,
  reconciled_at timestamptz,
  unknown_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_action_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_action_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_action_jobs_task_run_project_fk
    foreign key(task_run_id,project_id) references task_runs(id,project_id) on delete cascade,
  constraint connector_action_jobs_device_project_fk
    foreign key(device_id,project_id) references devices(id,project_id) on delete restrict,
  constraint connector_action_jobs_wayline_project_fk
    foreign key(wayline_resource_id,project_id) references connector_remote_resources(id,project_id) on delete restrict,
  constraint connector_action_jobs_target_project_fk
    foreign key(target_resource_id,project_id) references connector_remote_resources(id,project_id) on delete restrict,
  constraint connector_action_jobs_result_project_fk
    foreign key(remote_result_resource_id,project_id) references connector_remote_resources(id,project_id) on delete set null (remote_result_resource_id),
  constraint connector_action_jobs_approval_project_fk
    foreign key(approval_request_id,project_id) references approval_requests(id,project_id) on delete restrict,
  constraint connector_action_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_action_jobs_action_valid
    check(action_kind in('flight-task-create','flight-task-status','flight-task-resumption')),
  constraint connector_action_jobs_status_valid
    check(status in('queued','prepared','reconciling','succeeded','failed','blocked')),
  constraint connector_action_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_action_jobs_digest_valid
    check(request_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_action_jobs_envelope_object
    check(jsonb_typeof(request_envelope_json)='object'),
  constraint connector_action_jobs_dispatch_object
    check(jsonb_typeof(dispatch_check_json)='object'),
  constraint connector_action_jobs_attempts_valid
    check(attempt_count between 0 and 1 and reconciliation_count between 0 and 8),
  constraint connector_action_jobs_target_shape
    check(
      (action_kind='flight-task-create' and wayline_resource_id is not null and target_resource_id is null)
      or (action_kind in('flight-task-status','flight-task-resumption') and wayline_resource_id is null and target_resource_id is not null)
    ),
  constraint connector_action_jobs_completion_valid
    check(
      (status='succeeded')=(completed_at is not null)
      and (status<>'succeeded' or remote_result_resource_id is not null)
      and (status='blocked')=(unknown_at is not null)
    ),
  constraint connector_action_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_action_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_action_jobs_pending_idx
  on connector_action_jobs(connector_instance_id,action_kind,status,updated_at)
  where status not in('succeeded','failed','blocked');
