create table connector_control_sessions (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  device_id integer not null,
  holder_user_id integer not null,
  approval_request_id uuid not null,
  safety_policy_version_id bigint not null,
  idempotency_key text not null,
  controls_json jsonb not null,
  status text not null default 'requested',
  acquire_attempt_count integer not null default 0,
  release_attempt_count integer not null default 0,
  last_heartbeat_at timestamptz not null,
  lease_expires_at timestamptz not null,
  absolute_expires_at timestamptz not null,
  last_operation_at timestamptz,
  operation_window_started_at timestamptz not null,
  operation_count integer not null default 0,
  failure_code text,
  acquired_at timestamptz,
  release_requested_at timestamptz,
  released_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_control_sessions_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_control_sessions_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_control_sessions_device_project_fk
    foreign key(device_id,project_id) references devices(id,project_id) on delete restrict,
  constraint connector_control_sessions_holder_member_fk
    foreign key(team_id,holder_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_control_sessions_approval_project_fk
    foreign key(approval_request_id,project_id) references approval_requests(id,project_id) on delete restrict,
  constraint connector_control_sessions_policy_project_fk
    foreign key(safety_policy_version_id,project_id) references safety_policy_versions(id,project_id) on delete restrict,
  constraint connector_control_sessions_status_valid
    check(status in('requested','acquiring','active','releasing','released','failed','expired')),
  constraint connector_control_sessions_controls_object
    check(jsonb_typeof(controls_json)='object'),
  constraint connector_control_sessions_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_control_sessions_attempts_valid
    check(acquire_attempt_count between 0 and 1 and release_attempt_count between 0 and 1),
  constraint connector_control_sessions_operation_count_valid
    check(operation_count between 0 and 30),
  constraint connector_control_sessions_lease_valid
    check(lease_expires_at<=absolute_expires_at and absolute_expires_at<=created_at+interval '10 minutes'),
  constraint connector_control_sessions_terminal_valid check (
    (status in('released','failed','expired'))=(released_at is not null)
  ),
  constraint connector_control_sessions_project_idempotency_unique
    unique(project_id,device_id,idempotency_key),
  constraint connector_control_sessions_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create unique index connector_control_sessions_device_exclusive_idx
  on connector_control_sessions(project_id,device_id)
  where status in('requested','acquiring','active','releasing');
--> statement-breakpoint
create index connector_control_sessions_reconcile_idx
  on connector_control_sessions(status,lease_expires_at)
  where status in('requested','acquiring','active','releasing');
