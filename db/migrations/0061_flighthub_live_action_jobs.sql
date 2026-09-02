create table connector_live_action_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  device_id integer,
  target_resource_id bigint,
  requested_by_user_id integer not null,
  action_kind text not null,
  capability_code text not null,
  feature_flag text not null,
  idempotency_key text not null,
  request_digest text not null,
  request_envelope_json jsonb not null,
  status text not null default 'queued',
  attempt_count integer not null default 0,
  last_error_code text,
  result_json jsonb not null default '{}'::jsonb,
  attempted_at timestamptz,
  unknown_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_live_action_jobs_project_team_fk
    foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_live_action_jobs_connector_project_fk
    foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_live_action_jobs_device_project_fk
    foreign key(device_id,project_id) references devices(id,project_id) on delete restrict,
  constraint connector_live_action_jobs_target_project_fk
    foreign key(target_resource_id,project_id) references connector_remote_resources(id,project_id) on delete restrict,
  constraint connector_live_action_jobs_requester_member_fk
    foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_live_action_jobs_action_valid
    check(action_kind in('live-quality-set','live-converter-create','live-converter-toggle','live-converter-delete')),
  constraint connector_live_action_jobs_capability_valid check (
    (action_kind='live-quality-set' and capability_code='live.quality.set' and feature_flag='flighthub.live.quality')
    or (action_kind='live-converter-create' and capability_code='live.converter.create' and feature_flag='flighthub.live.converter.create')
    or (action_kind='live-converter-toggle' and capability_code='live.converter.toggle' and feature_flag='flighthub.live.converter.toggle')
    or (action_kind='live-converter-delete' and capability_code='live.converter.delete' and feature_flag='flighthub.live.converter.delete')
  ),
  constraint connector_live_action_jobs_target_shape check (
    (action_kind in('live-quality-set','live-converter-create') and device_id is not null and target_resource_id is null)
    or (action_kind in('live-converter-toggle','live-converter-delete') and device_id is null and target_resource_id is not null)
  ),
  constraint connector_live_action_jobs_status_valid
    check(status in('queued','executing','succeeded','failed','blocked')),
  constraint connector_live_action_jobs_idempotency_valid
    check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_live_action_jobs_digest_valid check(request_digest ~ '^[a-f0-9]{64}$'),
  constraint connector_live_action_jobs_envelope_object check(jsonb_typeof(request_envelope_json)='object'),
  constraint connector_live_action_jobs_result_object check(jsonb_typeof(result_json)='object'),
  constraint connector_live_action_jobs_attempt_valid check(attempt_count between 0 and 1),
  constraint connector_live_action_jobs_completion_valid check(
    (status in('succeeded','failed','blocked'))=(completed_at is not null)
    and (status='blocked')=(unknown_at is not null)
  ),
  constraint connector_live_action_jobs_project_idempotency_unique
    unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_live_action_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_live_action_jobs_pending_idx
  on connector_live_action_jobs(connector_instance_id,action_kind,status,updated_at)
  where status in('queued','executing');
--> statement-breakpoint
update connector_definitions
   set manifest_json = jsonb_set(
     manifest_json,
     '{capabilities}',
     coalesce(manifest_json->'capabilities','[]'::jsonb) || '[
       {"code":"live.quality.set","kind":"action","risk":"high","driverCapability":"stream.video.control","featureFlag":"flighthub.live.quality","defaultEnabled":false},
       {"code":"live.recording.control","kind":"action","risk":"high","featureFlag":"flighthub.live.recording","defaultEnabled":false},
       {"code":"live.share.manage","kind":"action","risk":"high","featureFlag":"flighthub.live.share","defaultEnabled":false},
       {"code":"live.converter.create","kind":"action","risk":"high","featureFlag":"flighthub.live.converter.create","defaultEnabled":false},
       {"code":"live.converter.toggle","kind":"action","risk":"high","featureFlag":"flighthub.live.converter.toggle","defaultEnabled":false},
       {"code":"live.converter.delete","kind":"action","risk":"critical","featureFlag":"flighthub.live.converter.delete","defaultEnabled":false}
     ]'::jsonb,
     true
   ),
       updated_at = now()
 where connector_key = 'dji.flighthub2' and version = '1.0.0';
