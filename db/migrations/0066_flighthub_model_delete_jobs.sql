create table connector_model_delete_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  target_resource_id bigint not null,
  approval_request_id uuid not null,
  requested_by_user_id integer not null,
  action_kind text not null,
  capability_code text not null,
  feature_flag text not null,
  idempotency_key text not null,
  expected_remote_version text not null,
  preview_digest text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
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
  constraint connector_model_delete_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_model_delete_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_model_delete_jobs_target_project_fk
    foreign key(target_resource_id,project_id) references connector_remote_resources(id,project_id) on delete restrict,
  constraint connector_model_delete_jobs_approval_project_fk
    foreign key(approval_request_id,project_id) references approval_requests(id,project_id) on delete restrict,
  constraint connector_model_delete_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_model_delete_jobs_action_valid
    check(action_kind in('model-delete','model-resource-delete')),
  constraint connector_model_delete_jobs_policy_valid check(
    (action_kind='model-delete' and capability_code='model.delete' and feature_flag='flighthub.model.delete')
    or (action_kind='model-resource-delete' and capability_code='model.resource.delete'
      and feature_flag='flighthub.model-resource.delete')
  ),
  constraint connector_model_delete_jobs_status_valid
    check(status in('queued','executing','succeeded','failed','blocked')),
  constraint connector_model_delete_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_model_delete_jobs_version_valid
    check(length(btrim(expected_remote_version)) between 1 and 512 and expected_remote_version=btrim(expected_remote_version)),
  constraint connector_model_delete_jobs_digest_valid
    check(preview_digest ~ '^[a-f0-9]{64}$' and request_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_model_delete_jobs_envelope_object check(jsonb_typeof(request_envelope_json)='object'),
  constraint connector_model_delete_jobs_result_object check(jsonb_typeof(result_json)='object'),
  constraint connector_model_delete_jobs_attempts_valid
    check(attempt_count between 0 and 1 and reconciliation_count between 0 and 16),
  constraint connector_model_delete_jobs_completion_valid check(
    (status in('succeeded','failed','blocked'))=(completed_at is not null)
    and (status='blocked')=(unknown_at is not null)
  ),
  constraint connector_model_delete_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_model_delete_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create unique index connector_model_delete_jobs_active_target_unique
  on connector_model_delete_jobs(target_resource_id)
  where status in('queued','executing');
--> statement-breakpoint
create index connector_model_delete_jobs_pending_idx
  on connector_model_delete_jobs(connector_instance_id,action_kind,status,updated_at)
  where status in('queued','executing');
--> statement-breakpoint
update connector_definitions
   set manifest_json = jsonb_set(
     manifest_json,
     '{capabilities}',
     coalesce(manifest_json->'capabilities','[]'::jsonb) || '[
       {"code":"model.delete","kind":"action","risk":"critical","featureFlag":"flighthub.model.delete","defaultEnabled":false},
       {"code":"model.resource.delete","kind":"action","risk":"critical","featureFlag":"flighthub.model-resource.delete","defaultEnabled":false}
     ]'::jsonb,
     true
   ),
       updated_at = now()
 where connector_key = 'dji.flighthub2' and version = '1.0.0';

