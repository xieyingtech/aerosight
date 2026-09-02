create table connector_device_admin_jobs (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  connector_instance_id bigint not null,
  device_id integer,
  requested_by_user_id integer not null,
  approval_request_id uuid not null,
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
  result_envelope_json jsonb,
  attempted_at timestamptz,
  unknown_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint connector_device_admin_jobs_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade,
  constraint connector_device_admin_jobs_connector_project_fk foreign key(connector_instance_id,project_id) references device_adapters(id,project_id) on delete cascade,
  constraint connector_device_admin_jobs_device_project_fk foreign key(device_id,project_id) references devices(id,project_id) on delete restrict,
  constraint connector_device_admin_jobs_requester_member_fk foreign key(team_id,requested_by_user_id) references team_members(team_id,user_id) on delete restrict,
  constraint connector_device_admin_jobs_approval_project_fk foreign key(approval_request_id,project_id) references approval_requests(id,project_id) on delete restrict,
  constraint connector_device_admin_jobs_action_valid check(action_kind in('rtk-calibrate','relay-pair','active-project-update','sn-decrypt')),
  constraint connector_device_admin_jobs_policy_valid check(
    (action_kind='rtk-calibrate' and capability_code='device.rtk.calibrate' and feature_flag='flighthub.rtk.calibrate') or
    (action_kind='relay-pair' and capability_code='device.relay.pair' and feature_flag='flighthub.relay.pair') or
    (action_kind='active-project-update' and capability_code='device.active-project.update' and feature_flag='flighthub.device-migration') or
    (action_kind='sn-decrypt' and capability_code='security.sn.decrypt' and feature_flag='flighthub.sn-decrypt')
  ),
  constraint connector_device_admin_jobs_target_valid check((action_kind='sn-decrypt')=(device_id is null)),
  constraint connector_device_admin_jobs_status_valid check(status in('queued','executing','accepted','succeeded','failed','blocked')),
  constraint connector_device_admin_jobs_idempotency_valid check(length(btrim(idempotency_key)) between 8 and 200 and idempotency_key=btrim(idempotency_key)),
  constraint connector_device_admin_jobs_digest_valid check(request_digest~'^[a-f0-9]{64}$'),
  constraint connector_device_admin_jobs_envelopes_valid check(jsonb_typeof(request_envelope_json)='object' and (result_envelope_json is null or jsonb_typeof(result_envelope_json)='object')),
  constraint connector_device_admin_jobs_result_valid check(jsonb_typeof(result_json)='object'),
  constraint connector_device_admin_jobs_attempt_valid check(attempt_count between 0 and 1),
  constraint connector_device_admin_jobs_completion_valid check((status in('succeeded','failed','blocked'))=(completed_at is not null) and (status='blocked')=(unknown_at is not null)),
  constraint connector_device_admin_jobs_project_idempotency_unique unique(project_id,connector_instance_id,action_kind,idempotency_key),
  constraint connector_device_admin_jobs_id_project_unique unique(id,project_id)
);
--> statement-breakpoint
create index connector_device_admin_jobs_pending_idx on connector_device_admin_jobs(status,updated_at) where status in('queued','executing','accepted');
--> statement-breakpoint
update driver_definitions set manifest_json=jsonb_set(manifest_json,'{capabilities}',manifest_json->'capabilities'||'[
  {"code":"rtk.calibrate","kind":"command","risk":"critical","inputSchema":{"type":"object"}},
  {"code":"relay.pair","kind":"command","risk":"critical","inputSchema":{"type":"object"}},
  {"code":"device.active-project.update","kind":"command","risk":"critical","inputSchema":{"type":"object"}}
]'::jsonb,true),updated_at=now() where driver_key='dji.cloud' and version='1.0.0';
--> statement-breakpoint
update device_types set capability_profile_json=capability_profile_json||'{"rtk.calibrate":{"enabled":true},"device.active-project.update":{"enabled":true}}'::jsonb,updated_at=now()
 where type_key in('dji.dock2','dji.dock3');
--> statement-breakpoint
update device_types set capability_profile_json=capability_profile_json||'{"relay.pair":{"enabled":true}}'::jsonb,updated_at=now() where type_key='dji.dock3';
--> statement-breakpoint
insert into device_capabilities(device_id,project_id,capability_code,version,declared_by_adapter_id,params_schema_json,input_schema_json,output_schema_json,risk_level,source_json)
select device.id,device.project_id,capability->>'code',driver.version,route.connector_instance_id,coalesce(capability->'inputSchema','{}'),coalesce(capability->'inputSchema','{}'),coalesce(capability->'outputSchema','{}'),coalesce(capability->>'risk','low'),jsonb_build_object('driver',driver.driver_key,'typeKey',device_type.type_key)
from devices device join device_types device_type on device_type.id=device.device_type_id join driver_definitions driver on driver.id=device_type.driver_definition_id
join lateral(select binding.connector_instance_id from device_connector_bindings binding join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id join connector_definitions definition on definition.id=adapter.connector_definition_id where binding.project_id=device.project_id and binding.device_id=device.id and binding.status='active' and definition.connector_key='dji.flighthub2' and definition.version='1.0.0' order by binding.priority desc,binding.connector_instance_id limit 1) route on true
cross join lateral jsonb_array_elements(case when jsonb_typeof(driver.manifest_json->'capabilities')='array' then driver.manifest_json->'capabilities' else '[]'::jsonb end) capability
where capability->>'code' in('rtk.calibrate','relay.pair','device.active-project.update') and device_type.capability_profile_json ? (capability->>'code')
on conflict(device_id,capability_code) do update set declared_by_adapter_id=excluded.declared_by_adapter_id,params_schema_json=excluded.params_schema_json,input_schema_json=excluded.input_schema_json,output_schema_json=excluded.output_schema_json,risk_level=excluded.risk_level,availability='available',availability_reason=null,source_json=excluded.source_json,updated_at=now();
--> statement-breakpoint
update connector_definitions set manifest_json=jsonb_set(manifest_json,'{capabilities}',manifest_json->'capabilities'||'[
  {"code":"device.rtk.calibrate","kind":"action","risk":"critical","driverCapability":"rtk.calibrate","featureFlag":"flighthub.rtk.calibrate","defaultEnabled":false},
  {"code":"device.relay.pair","kind":"action","risk":"critical","driverCapability":"relay.pair","featureFlag":"flighthub.relay.pair","defaultEnabled":false},
  {"code":"device.active-project.update","kind":"action","risk":"critical","driverCapability":"device.active-project.update","featureFlag":"flighthub.device-migration","defaultEnabled":false},
  {"code":"security.sn.decrypt","kind":"action","risk":"high","featureFlag":"flighthub.sn-decrypt","defaultEnabled":false}
]'::jsonb,true),updated_at=now() where connector_key='dji.flighthub2' and version='1.0.0';
