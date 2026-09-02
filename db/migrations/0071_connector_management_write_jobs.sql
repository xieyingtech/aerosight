create table connector_management_write_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  requested_by_user_id integer not null,
  approval_request_id uuid not null,
  action_kind text not null,
  capability_code text not null,
  feature_flag text not null,
  idempotency_key text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
  preview_digest text not null,
  preview_json jsonb not null,
  status text not null default 'queued',
  attempt_count integer not null default 0,
  reconciliation_count integer not null default 0,
  last_error_code text,
  result_json jsonb not null default '{}'::jsonb,
  attempted_at timestamptz,
  unknown_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_management_write_jobs_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_management_write_jobs_connector_project_fk foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_management_write_jobs_requester_member_fk foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_management_write_jobs_approval_project_fk foreign key(approval_request_id,project_id) references approval_requests(id,project_id) on delete restrict,
  constraint connector_management_write_jobs_action_valid check(action_kind='project-member-upsert'),
  constraint connector_management_write_jobs_policy_valid check(capability_code='organization.project-member.write' and feature_flag='flighthub.organization.project-member'),
  constraint connector_management_write_jobs_status_valid check(status in('queued','executing','accepted','succeeded','failed','blocked')),
  constraint connector_management_write_jobs_idempotency_valid check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_management_write_jobs_digest_valid check(request_digest~'^[a-f0-9]{64}$' and preview_digest~'^[a-f0-9]{64}$'),
  constraint connector_management_write_jobs_json_valid check(jsonb_typeof(request_envelope_json)='object' and jsonb_typeof(preview_json)='object' and jsonb_typeof(result_json)='object'),
  constraint connector_management_write_jobs_attempt_valid check(attempt_count between 0 and 1 and reconciliation_count between 0 and 1),
  constraint connector_management_write_jobs_completion_valid check((status in('succeeded','failed','blocked'))=(completed_at is not null) and (status='blocked')=(unknown_at is not null)),
  constraint connector_management_write_jobs_project_idempotency_unique unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_management_write_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_management_write_jobs_pending_idx on connector_management_write_jobs(status,updated_at) where status in('queued','executing','accepted');
--> statement-breakpoint
update connector_definitions set manifest_json=jsonb_set(manifest_json,'{capabilities}',
  (select coalesce(jsonb_agg(value),'[]'::jsonb) from jsonb_array_elements(manifest_json->'capabilities') value where value->>'code'<>'organization.write') || '[
    {"code":"organization.project-member.write","kind":"action","risk":"high","featureFlag":"flighthub.organization.project-member","defaultEnabled":false},
    {"code":"organization.write","kind":"action","risk":"critical","featureFlag":"flighthub.actions","defaultEnabled":false,"availability":"unreleased"}
  ]'::jsonb,true) || '{"futureManagementWritesDefault":"unavailable"}'::jsonb,updated_at=now()
where connector_key='dji.flighthub2' and version='1.0.0';
