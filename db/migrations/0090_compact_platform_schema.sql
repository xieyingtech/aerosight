-- Compact physical storage; preserve historical data and business boundaries.

alter table audit_events alter column project_id drop not null, alter column team_id drop not null,
 add column scope text not null default 'project', add column actor_system text,
 add column legacy_platform_id bigint unique, add column details_json jsonb not null default '{}',
 drop constraint audit_events_actor_present,
 add constraint audit_events_actor_present check(actor_user_id is not null or actor_agent_id is not null or actor_system is not null),
 add constraint audit_events_scope_valid check((scope='project' and project_id is not null and team_id is not null) or (scope='platform' and project_id is null and team_id is null));

select setval('audit_events_id_seq',greatest(coalesce((select max(id) from audit_events),0)+1,1),false);

insert into audit_events(scope,actor_user_id,request_id,action,resource_type,resource_id,input_hash,result_hash,status,created_at,completed_at,legacy_platform_id)
 select 'platform',actor_user_id,request_id,action,resource_type,resource_id,input_hash,result_hash,status,created_at,completed_at,id from platform_audit_events order by id;

drop table platform_audit_events;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','retention_deletion_tombstones',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from retention_deletion_tombstones old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','retention_holds',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from retention_holds old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','retention_cleanup_runs',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from retention_cleanup_runs old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','retention_policies',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from retention_policies old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','alert_automation_drafts',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from alert_automation_drafts old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','alert_automation_runs',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from alert_automation_runs old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','alert_automation_policy_versions',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from alert_automation_policy_versions old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','alert_automation_policies',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from alert_automation_policies old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','sensor_calibrations',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from sensor_calibrations old join projects project on project.id=old.project_id;

insert into audit_events(project_id,team_id,request_id,actor_system,action,resource_type,resource_id,input_hash,details_json,status,completed_at)
 select old.project_id,project.team_id,'schema-0090','schema-migration','schema.archive','safety_policy_versions',old.id::text,'schema-0090',to_jsonb(old),'completed',now()
 from safety_policy_versions old join projects project on project.id=old.project_id;

alter table projects drop constraint projects_current_safety_policy_project_fk;

comment on column projects.current_safety_policy_version_id is 'Historical snapshot ID; retired record recoverable from audit_events schema.archive';

alter table task_runs drop constraint task_runs_policy_project_fk;

comment on column task_runs.safety_policy_version_id is 'Historical snapshot ID; retired record recoverable from audit_events schema.archive';

alter table connector_control_sessions drop constraint connector_control_sessions_policy_project_fk;

comment on column connector_control_sessions.safety_policy_version_id is 'Historical snapshot ID; retired record recoverable from audit_events schema.archive';

alter table observations drop constraint observations_calibration_project_fk;

comment on column observations.calibration_id is 'Historical snapshot ID; retired record recoverable from audit_events schema.archive';

drop table retention_deletion_tombstones;

drop table retention_holds;

drop table retention_cleanup_runs;

drop table retention_policies;

drop table alert_automation_drafts;

drop table alert_automation_runs;

alter table alert_automation_policies drop constraint alert_automation_policies_current_version_project_fk;

drop table alert_automation_policy_versions;

drop table alert_automation_policies;

drop table sensor_calibrations;

drop table safety_policy_versions;

create table connector_jobs(
 job_type text not null check(job_type in('flight','live','geospatial','model','model-delete','object-upload','device-admin','management')),
 id uuid default gen_random_uuid() not null,
 project_id integer not null,
 team_id integer not null,
 connector_instance_id bigint not null,
 task_run_id integer,
 device_id integer,
 wayline_resource_id bigint,
 target_resource_id bigint,
 remote_result_resource_id bigint,
 approval_request_id uuid,
 requested_by_user_id integer not null,
 action_kind text,
 idempotency_key text not null,
 request_digest text,
 request_envelope_json jsonb,
 status text default 'queued'::text not null,
 dispatch_check_json jsonb default '{}'::jsonb,
 attempt_count integer default 0,
 reconciliation_count integer default 0,
 last_error_code text,
 accepted_at timestamp with time zone,
 reconciled_at timestamp with time zone,
 unknown_at timestamp with time zone,
 completed_at timestamp with time zone,
 created_at timestamp with time zone default now() not null,
 updated_at timestamp with time zone default now() not null,
 capability_code text,
 feature_flag text,
 result_json jsonb default '{}'::jsonb,
 attempted_at timestamp with time zone,
 expected_remote_version text,
 reconciliation_name text,
 remote_ids_json jsonb default '[]'::jsonb,
 asset_ids_json jsonb default '[]'::jsonb,
 progress integer default 0,
 stage text default 'queued'::text,
 submit_attempt_count integer default 0,
 submitted_at timestamp with time zone,
 preview_digest text,
 operation_kind text,
 source_asset_id integer,
 requested_name text,
 object_key_digest text,
 object_key_envelope_json jsonb,
 notification_attempt_count integer default 0,
 reconciliation_miss_count integer default 0,
 remote_resource_id bigint,
 uploaded_at timestamp with time zone,
 notification_attempted_at timestamp with time zone,
 result_envelope_json jsonb,
 preview_json jsonb,
 primary key(id),
 unique(id,project_id),
 business_run_id integer,
 business_step_id bigint,
 business_bound_at timestamptz,
 constraint connector_jobs_business_binding_valid check((business_run_id is null)=(business_step_id is null) and (business_step_id is null or (job_type='flight' and action_kind='flight-task-create' and business_run_id<>task_run_id))),
 foreign key(business_step_id,business_run_id,project_id) references task_run_steps(id,task_run_id,project_id),
 constraint jobs_flight_required check(job_type<>'flight' or (task_run_id is not null and device_id is not null and approval_request_id is not null and action_kind is not null and request_digest is not null and request_envelope_json is not null and dispatch_check_json is not null and attempt_count is not null and reconciliation_count is not null)),
 constraint connector_action_jobs_action_valid check(job_type<>'flight' or ((action_kind = ANY (ARRAY['flight-task-create'::text, 'flight-task-status'::text, 'flight-task-resumption'::text])))),
 FOREIGN KEY (approval_request_id, project_id) REFERENCES approval_requests(id, project_id) ON DELETE RESTRICT,
 constraint connector_action_jobs_attempts_valid check(job_type<>'flight' or ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 8))))),
 constraint connector_action_jobs_completion_valid check(job_type<>'flight' or ((((status = 'succeeded'::text) = (completed_at IS NOT NULL)) AND ((status <> 'succeeded'::text) OR (remote_result_resource_id IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 FOREIGN KEY (connector_instance_id, project_id) REFERENCES device_adapters(id, project_id) ON DELETE CASCADE,
 FOREIGN KEY (device_id, project_id) REFERENCES devices(id, project_id) ON DELETE RESTRICT,
 constraint connector_action_jobs_digest_valid check(job_type<>'flight' or ((request_digest ~ '^[a-f0-9]{64}$'::text))),
 constraint connector_action_jobs_dispatch_object check(job_type<>'flight' or ((jsonb_typeof(dispatch_check_json) = 'object'::text))),
 constraint connector_action_jobs_envelope_object check(job_type<>'flight' or ((jsonb_typeof(request_envelope_json) = 'object'::text))),
 UNIQUE (id, project_id),
 constraint connector_action_jobs_idempotency_valid check(job_type<>'flight' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 UNIQUE (id, project_id, team_id, connector_instance_id, task_run_id, action_kind),
 UNIQUE (job_type, project_id, connector_instance_id, action_kind, idempotency_key),
 FOREIGN KEY (project_id, team_id) REFERENCES projects(id, team_id) ON DELETE CASCADE,
 FOREIGN KEY (requested_by_user_id) REFERENCES users(id) ON DELETE RESTRICT,
 FOREIGN KEY (remote_result_resource_id, project_id) REFERENCES connector_remote_resources(id, project_id) ON DELETE SET NULL (remote_result_resource_id),
 constraint connector_action_jobs_status_valid check(job_type<>'flight' or ((status = ANY (ARRAY['queued'::text, 'prepared'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 FOREIGN KEY (target_resource_id, project_id) REFERENCES connector_remote_resources(id, project_id) ON DELETE RESTRICT,
 constraint connector_action_jobs_target_shape check(job_type<>'flight' or ((((action_kind = 'flight-task-create'::text) AND (wayline_resource_id IS NOT NULL) AND (target_resource_id IS NULL)) OR ((action_kind = ANY (ARRAY['flight-task-status'::text, 'flight-task-resumption'::text])) AND (wayline_resource_id IS NULL) AND (target_resource_id IS NOT NULL))))),
 FOREIGN KEY (task_run_id, project_id) REFERENCES task_runs(id, project_id) ON DELETE CASCADE,
 FOREIGN KEY (wayline_resource_id, project_id) REFERENCES connector_remote_resources(id, project_id) ON DELETE RESTRICT,
 constraint jobs_live_required check(job_type<>'live' or (action_kind is not null and capability_code is not null and feature_flag is not null and request_digest is not null and request_envelope_json is not null and attempt_count is not null and result_json is not null)),
 constraint connector_live_action_jobs_action_valid check(job_type<>'live' or ((action_kind = ANY (ARRAY['live-quality-set'::text, 'live-converter-create'::text, 'live-converter-toggle'::text, 'live-converter-delete'::text])))),
 constraint connector_live_action_jobs_attempt_valid check(job_type<>'live' or (((attempt_count >= 0) AND (attempt_count <= 1)))),
 constraint connector_live_action_jobs_capability_valid check(job_type<>'live' or ((((action_kind = 'live-quality-set'::text) AND (capability_code = 'live.quality.set'::text) AND (feature_flag = 'flighthub.live.quality'::text)) OR ((action_kind = 'live-converter-create'::text) AND (capability_code = 'live.converter.create'::text) AND (feature_flag = 'flighthub.live.converter.create'::text)) OR ((action_kind = 'live-converter-toggle'::text) AND (capability_code = 'live.converter.toggle'::text) AND (feature_flag = 'flighthub.live.converter.toggle'::text)) OR ((action_kind = 'live-converter-delete'::text) AND (capability_code = 'live.converter.delete'::text) AND (feature_flag = 'flighthub.live.converter.delete'::text))))),
 constraint connector_live_action_jobs_completion_valid check(job_type<>'live' or ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 constraint connector_live_action_jobs_digest_valid check(job_type<>'live' or ((request_digest ~ '^[a-f0-9]{64}$'::text))),
 constraint connector_live_action_jobs_envelope_object check(job_type<>'live' or ((jsonb_typeof(request_envelope_json) = 'object'::text))),
 constraint connector_live_action_jobs_idempotency_valid check(job_type<>'live' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_live_action_jobs_result_object check(job_type<>'live' or ((jsonb_typeof(result_json) = 'object'::text))),
 constraint connector_live_action_jobs_status_valid check(job_type<>'live' or ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 constraint connector_live_action_jobs_target_shape check(job_type<>'live' or ((((action_kind = ANY (ARRAY['live-quality-set'::text, 'live-converter-create'::text])) AND (device_id IS NOT NULL) AND (target_resource_id IS NULL)) OR ((action_kind = ANY (ARRAY['live-converter-toggle'::text, 'live-converter-delete'::text])) AND (device_id IS NULL) AND (target_resource_id IS NOT NULL))))),
 constraint jobs_geospatial_required check(job_type<>'geospatial' or (action_kind is not null and capability_code is not null and feature_flag is not null and request_digest is not null and request_envelope_json is not null and attempt_count is not null and result_json is not null)),
 constraint connector_geospatial_action_jobs_action_valid check(job_type<>'geospatial' or ((action_kind = ANY (ARRAY['map-element-create'::text, 'map-element-update'::text, 'map-element-delete'::text])))),
 constraint connector_geospatial_action_jobs_attempt_valid check(job_type<>'geospatial' or (((attempt_count >= 0) AND (attempt_count <= 1)))),
 constraint connector_geospatial_action_jobs_capability_valid check(job_type<>'geospatial' or ((((action_kind = ANY (ARRAY['map-element-create'::text, 'map-element-update'::text])) AND (capability_code = 'geospatial.write'::text) AND (feature_flag = 'flighthub.actions'::text)) OR ((action_kind = 'map-element-delete'::text) AND (capability_code = 'geospatial.element.delete'::text) AND (feature_flag = 'flighthub.geospatial.delete'::text))))),
 constraint connector_geospatial_action_jobs_completion_valid check(job_type<>'geospatial' or ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 constraint connector_geospatial_action_jobs_digest_valid check(job_type<>'geospatial' or ((request_digest ~ '^[a-f0-9]{64}$'::text))),
 constraint connector_geospatial_action_jobs_envelope_object check(job_type<>'geospatial' or ((jsonb_typeof(request_envelope_json) = 'object'::text))),
 constraint connector_geospatial_action_jobs_idempotency_valid check(job_type<>'geospatial' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_geospatial_action_jobs_result_object check(job_type<>'geospatial' or ((jsonb_typeof(result_json) = 'object'::text))),
 constraint connector_geospatial_action_jobs_status_valid check(job_type<>'geospatial' or ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 constraint connector_geospatial_action_jobs_target_shape check(job_type<>'geospatial' or ((((action_kind = 'map-element-create'::text) AND (target_resource_id IS NULL) AND (expected_remote_version IS NULL)) OR ((action_kind = ANY (ARRAY['map-element-update'::text, 'map-element-delete'::text])) AND (target_resource_id IS NOT NULL) AND ((length(btrim(expected_remote_version)) >= 1) AND (length(btrim(expected_remote_version)) <= 512)) AND (expected_remote_version = btrim(expected_remote_version)))))),
 constraint jobs_model_required check(job_type<>'model' or (action_kind is not null and request_digest is not null and request_envelope_json is not null and remote_ids_json is not null and asset_ids_json is not null and progress is not null and stage is not null and submit_attempt_count is not null and reconciliation_count is not null)),
 constraint connector_model_jobs_action_valid check(job_type<>'model' or ((action_kind = ANY (ARRAY['traditional-create'::text, 'open-start'::text, 'open-stop'::text])))),
 constraint connector_model_jobs_asset_array check(job_type<>'model' or ((jsonb_typeof(asset_ids_json) = 'array'::text))),
 constraint connector_model_jobs_attempts_valid check(job_type<>'model' or ((((submit_attempt_count >= 0) AND (submit_attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 32))))),
 constraint connector_model_jobs_completion_valid check(job_type<>'model' or (((status = 'succeeded'::text) = (completed_at IS NOT NULL)))),
 constraint connector_model_jobs_digest_valid check(job_type<>'model' or ((request_digest ~ '^[a-f0-9]{64}$'::text))),
 constraint connector_model_jobs_envelope_object check(job_type<>'model' or ((jsonb_typeof(request_envelope_json) = 'object'::text))),
 constraint connector_model_jobs_idempotency_valid check(job_type<>'model' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_model_jobs_progress_valid check(job_type<>'model' or (((progress >= 0) AND (progress <= 100)))),
 constraint connector_model_jobs_remote_array check(job_type<>'model' or ((jsonb_typeof(remote_ids_json) = 'array'::text))),
 constraint connector_model_jobs_status_valid check(job_type<>'model' or ((status = ANY (ARRAY['queued'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 constraint jobs_model_delete_required check(job_type<>'model-delete' or (target_resource_id is not null and approval_request_id is not null and action_kind is not null and capability_code is not null and feature_flag is not null and expected_remote_version is not null and preview_digest is not null and request_digest is not null and request_envelope_json is not null and attempt_count is not null and reconciliation_count is not null and result_json is not null)),
 constraint connector_model_delete_jobs_action_valid check(job_type<>'model-delete' or ((action_kind = ANY (ARRAY['model-delete'::text, 'model-resource-delete'::text])))),
 constraint connector_model_delete_jobs_attempts_valid check(job_type<>'model-delete' or ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 16))))),
 constraint connector_model_delete_jobs_completion_valid check(job_type<>'model-delete' or ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 constraint connector_model_delete_jobs_digest_valid check(job_type<>'model-delete' or (((preview_digest ~ '^[a-f0-9]{64}$'::text) AND (request_digest ~ '^[a-f0-9]{64}$'::text)))),
 constraint connector_model_delete_jobs_envelope_object check(job_type<>'model-delete' or ((jsonb_typeof(request_envelope_json) = 'object'::text))),
 constraint connector_model_delete_jobs_idempotency_valid check(job_type<>'model-delete' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_model_delete_jobs_policy_valid check(job_type<>'model-delete' or ((((action_kind = 'model-delete'::text) AND (capability_code = 'model.delete'::text) AND (feature_flag = 'flighthub.model.delete'::text)) OR ((action_kind = 'model-resource-delete'::text) AND (capability_code = 'model.resource.delete'::text) AND (feature_flag = 'flighthub.model-resource.delete'::text))))),
 constraint connector_model_delete_jobs_result_object check(job_type<>'model-delete' or ((jsonb_typeof(result_json) = 'object'::text))),
 constraint connector_model_delete_jobs_status_valid check(job_type<>'model-delete' or ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 constraint connector_model_delete_jobs_version_valid check(job_type<>'model-delete' or ((((length(btrim(expected_remote_version)) >= 1) AND (length(btrim(expected_remote_version)) <= 512)) AND (expected_remote_version = btrim(expected_remote_version))))),
 constraint jobs_object_upload_required check(job_type<>'object-upload' or (operation_kind is not null and source_asset_id is not null and requested_name is not null and reconciliation_name is not null and notification_attempt_count is not null and reconciliation_miss_count is not null)),
 FOREIGN KEY (source_asset_id, project_id) REFERENCES assets(id, project_id) ON DELETE RESTRICT,
 constraint connector_object_upload_jobs_attempts_valid check(job_type<>'object-upload' or ((((notification_attempt_count >= 0) AND (notification_attempt_count <= 2)) AND ((reconciliation_miss_count >= 0) AND (reconciliation_miss_count <= 8))))),
 constraint connector_object_upload_jobs_completion_valid check(job_type<>'object-upload' or ((((status = 'succeeded'::text) = (completed_at IS NOT NULL)) AND ((status <> 'succeeded'::text) OR (remote_resource_id IS NOT NULL))))),
 constraint connector_object_upload_jobs_digest_valid check(job_type<>'object-upload' or (((object_key_digest IS NULL) OR (object_key_digest ~ '^[a-f0-9]{64}$'::text)))),
 constraint connector_object_upload_jobs_envelope_object check(job_type<>'object-upload' or (((object_key_envelope_json IS NULL) OR (jsonb_typeof(object_key_envelope_json) = 'object'::text)))),
 constraint connector_object_upload_jobs_idempotency_valid check(job_type<>'object-upload' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_object_upload_jobs_name_valid check(job_type<>'object-upload' or ((((length(btrim(requested_name)) >= 1) AND (length(btrim(requested_name)) <= 200)) AND (requested_name = btrim(requested_name)) AND ((length(btrim(reconciliation_name)) >= 1) AND (length(btrim(reconciliation_name)) <= 240)) AND (reconciliation_name = btrim(reconciliation_name))))),
 constraint connector_object_upload_jobs_operation_kind_valid check(job_type<>'object-upload' or ((operation_kind ~ '^[a-z][a-z0-9-]{0,63}$'::text))),
 UNIQUE (job_type, project_id, connector_instance_id, operation_kind, idempotency_key),
 FOREIGN KEY (remote_resource_id, project_id) REFERENCES connector_remote_resources(id, project_id) ON DELETE SET NULL (remote_resource_id),
 constraint connector_object_upload_jobs_status_valid check(job_type<>'object-upload' or ((status = ANY (ARRAY['queued'::text, 'uploading'::text, 'notifying'::text, 'reconciling'::text, 'succeeded'::text, 'failed'::text])))),
 constraint connector_object_upload_jobs_upload_checkpoint check(job_type<>'object-upload' or ((((object_key_digest IS NULL) = (object_key_envelope_json IS NULL)) AND ((uploaded_at IS NULL) = (object_key_envelope_json IS NULL))))),
 constraint jobs_device_admin_required check(job_type<>'device-admin' or (approval_request_id is not null and action_kind is not null and capability_code is not null and feature_flag is not null and request_digest is not null and request_envelope_json is not null and attempt_count is not null and result_json is not null)),
 constraint connector_device_admin_jobs_action_valid check(job_type<>'device-admin' or ((action_kind = ANY (ARRAY['rtk-calibrate'::text, 'relay-pair'::text, 'active-project-update'::text, 'sn-decrypt'::text])))),
 constraint connector_device_admin_jobs_attempt_valid check(job_type<>'device-admin' or (((attempt_count >= 0) AND (attempt_count <= 1)))),
 constraint connector_device_admin_jobs_completion_valid check(job_type<>'device-admin' or ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 constraint connector_device_admin_jobs_digest_valid check(job_type<>'device-admin' or ((request_digest ~ '^[a-f0-9]{64}$'::text))),
 constraint connector_device_admin_jobs_envelopes_valid check(job_type<>'device-admin' or (((jsonb_typeof(request_envelope_json) = 'object'::text) AND ((result_envelope_json IS NULL) OR (jsonb_typeof(result_envelope_json) = 'object'::text))))),
 constraint connector_device_admin_jobs_idempotency_valid check(job_type<>'device-admin' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_device_admin_jobs_policy_valid check(job_type<>'device-admin' or ((((action_kind = 'rtk-calibrate'::text) AND (capability_code = 'device.rtk.calibrate'::text) AND (feature_flag = 'flighthub.rtk.calibrate'::text)) OR ((action_kind = 'relay-pair'::text) AND (capability_code = 'device.relay.pair'::text) AND (feature_flag = 'flighthub.relay.pair'::text)) OR ((action_kind = 'active-project-update'::text) AND (capability_code = 'device.active-project.update'::text) AND (feature_flag = 'flighthub.device-migration'::text)) OR ((action_kind = 'sn-decrypt'::text) AND (capability_code = 'security.sn.decrypt'::text) AND (feature_flag = 'flighthub.sn-decrypt'::text))))),
 constraint connector_device_admin_jobs_result_valid check(job_type<>'device-admin' or ((jsonb_typeof(result_json) = 'object'::text))),
 constraint connector_device_admin_jobs_status_valid check(job_type<>'device-admin' or ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text])))),
 constraint connector_device_admin_jobs_target_valid check(job_type<>'device-admin' or (((action_kind = 'sn-decrypt'::text) = (device_id IS NULL)))),
 constraint jobs_management_required check(job_type<>'management' or (approval_request_id is not null and action_kind is not null and capability_code is not null and feature_flag is not null and request_digest is not null and request_envelope_json is not null and preview_digest is not null and preview_json is not null and attempt_count is not null and reconciliation_count is not null and result_json is not null)),
 constraint connector_management_write_jobs_action_valid check(job_type<>'management' or ((action_kind = 'project-member-upsert'::text))),
 constraint connector_management_write_jobs_attempt_valid check(job_type<>'management' or ((((attempt_count >= 0) AND (attempt_count <= 1)) AND ((reconciliation_count >= 0) AND (reconciliation_count <= 1))))),
 constraint connector_management_write_jobs_completion_valid check(job_type<>'management' or ((((status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'blocked'::text])) = (completed_at IS NOT NULL)) AND ((status = 'blocked'::text) = (unknown_at IS NOT NULL))))),
 constraint connector_management_write_jobs_digest_valid check(job_type<>'management' or (((request_digest ~ '^[a-f0-9]{64}$'::text) AND (preview_digest ~ '^[a-f0-9]{64}$'::text)))),
 constraint connector_management_write_jobs_idempotency_valid check(job_type<>'management' or ((((length(btrim(idempotency_key)) >= 8) AND (length(btrim(idempotency_key)) <= 200)) AND (idempotency_key = btrim(idempotency_key))))),
 constraint connector_management_write_jobs_json_valid check(job_type<>'management' or (((jsonb_typeof(request_envelope_json) = 'object'::text) AND (jsonb_typeof(preview_json) = 'object'::text) AND (jsonb_typeof(result_json) = 'object'::text)))),
 constraint connector_management_write_jobs_policy_valid check(job_type<>'management' or (((capability_code = 'organization.project-member.write'::text) AND (feature_flag = 'flighthub.organization.project-member'::text)))),
 constraint connector_management_write_jobs_status_valid check(job_type<>'management' or ((status = ANY (ARRAY['queued'::text, 'executing'::text, 'accepted'::text, 'succeeded'::text, 'failed'::text, 'blocked'::text]))))
);

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,target_resource_id,remote_result_resource_id,approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,status,dispatch_check_json,attempt_count,reconciliation_count,last_error_code,accepted_at,reconciled_at,unknown_at,completed_at,created_at,updated_at) select 'flight',id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,target_resource_id,remote_result_resource_id,approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,status,dispatch_check_json,attempt_count,reconciliation_count,last_error_code,accepted_at,reconciled_at,unknown_at,completed_at,created_at,updated_at from connector_action_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,device_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at) select 'live',id,project_id,team_id,connector_instance_id,device_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_live_action_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at) select 'geospatial',id,project_id,team_id,connector_instance_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_geospatial_action_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,reconciliation_name,status,remote_ids_json,asset_ids_json,progress,stage,submit_attempt_count,reconciliation_count,last_error_code,submitted_at,reconciled_at,completed_at,created_at,updated_at) select 'model',id,project_id,team_id,connector_instance_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,reconciliation_name,status,remote_ids_json,asset_ids_json,progress,stage,submit_attempt_count,reconciliation_count,last_error_code,submitted_at,reconciled_at,completed_at,created_at,updated_at from connector_model_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,target_resource_id,approval_request_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,preview_digest,request_digest,request_envelope_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at) select 'model-delete',id,project_id,team_id,connector_instance_id,target_resource_id,approval_request_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,preview_digest,request_digest,request_envelope_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_model_delete_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,operation_kind,source_asset_id,requested_by_user_id,idempotency_key,requested_name,reconciliation_name,status,object_key_digest,object_key_envelope_json,notification_attempt_count,reconciliation_miss_count,last_error_code,remote_resource_id,uploaded_at,notification_attempted_at,reconciled_at,completed_at,created_at,updated_at) select 'object-upload',id,project_id,team_id,connector_instance_id,operation_kind,source_asset_id,requested_by_user_id,idempotency_key,requested_name,reconciliation_name,status,object_key_digest,object_key_envelope_json,notification_attempt_count,reconciliation_miss_count,last_error_code,remote_resource_id,uploaded_at,notification_attempted_at,reconciled_at,completed_at,created_at,updated_at from connector_object_upload_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,result_envelope_json,attempted_at,unknown_at,completed_at,created_at,updated_at) select 'device-admin',id,project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,result_envelope_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_device_admin_jobs;

insert into connector_jobs(job_type,id,project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at) select 'management',id,project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_management_write_jobs;

update connector_jobs job set business_run_id=b.business_run_id,business_step_id=b.business_step_id,business_bound_at=b.created_at
 from inspection_flight_bindings b where job.id=b.action_job_id and job.project_id=b.project_id and job.job_type='flight';

drop table inspection_flight_bindings;

create unique index connector_jobs_business_step_unique on connector_jobs(project_id,business_step_id) where business_step_id is not null;

create unique index connector_jobs_business_flight_unique on connector_jobs(project_id,task_run_id) where business_step_id is not null;

create index connector_jobs_pending_idx on connector_jobs(job_type,status,updated_at) where status not in('succeeded','failed','blocked');

drop table connector_action_jobs;

create view connector_action_jobs as select id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,target_resource_id,remote_result_resource_id,approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,status,dispatch_check_json,attempt_count,reconciliation_count,last_error_code,accepted_at,reconciled_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='flight' with cascaded check option;

drop table connector_live_action_jobs;

create view connector_live_action_jobs as select id,project_id,team_id,connector_instance_id,device_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='live' with cascaded check option;

drop table connector_geospatial_action_jobs;

create view connector_geospatial_action_jobs as select id,project_id,team_id,connector_instance_id,target_resource_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='geospatial' with cascaded check option;

drop table connector_model_jobs;

create view connector_model_jobs as select id,project_id,team_id,connector_instance_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json,reconciliation_name,status,remote_ids_json,asset_ids_json,progress,stage,submit_attempt_count,reconciliation_count,last_error_code,submitted_at,reconciled_at,completed_at,created_at,updated_at from connector_jobs where job_type='model' with cascaded check option;

drop table connector_model_delete_jobs;

create view connector_model_delete_jobs as select id,project_id,team_id,connector_instance_id,target_resource_id,approval_request_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,preview_digest,request_digest,request_envelope_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='model-delete' with cascaded check option;

drop table connector_object_upload_jobs;

create view connector_object_upload_jobs as select id,project_id,team_id,connector_instance_id,operation_kind,source_asset_id,requested_by_user_id,idempotency_key,requested_name,reconciliation_name,status,object_key_digest,object_key_envelope_json,notification_attempt_count,reconciliation_miss_count,last_error_code,remote_resource_id,uploaded_at,notification_attempted_at,reconciled_at,completed_at,created_at,updated_at from connector_jobs where job_type='object-upload' with cascaded check option;

drop table connector_device_admin_jobs;

create view connector_device_admin_jobs as select id,project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,status,attempt_count,last_error_code,result_json,result_envelope_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='device-admin' with cascaded check option;

drop table connector_management_write_jobs;

create view connector_management_write_jobs as select id,project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json,status,attempt_count,reconciliation_count,last_error_code,result_json,attempted_at,unknown_at,completed_at,created_at,updated_at from connector_jobs where job_type='management' with cascaded check option;

alter table detections add column group_id bigint, add column grouped_at timestamptz;

update detections d set group_id=m.detection_group_id,grouped_at=m.added_at from detection_group_members m where d.id=m.detection_id and d.project_id=m.project_id;

alter table detections add constraint detections_group_project_fk foreign key(group_id,project_id) references detection_groups(id,project_id) on delete set null(group_id);

create index detections_group_idx on detections(project_id,group_id) where group_id is not null;

drop table detection_group_members;

alter table device_adapters add column task_managed_alerts boolean not null default false;

update device_adapters a set task_managed_alerts=p.task_managed_alerts from inspection_connector_policies p where a.id=p.connector_instance_id and a.project_id=p.project_id;

drop table inspection_connector_policies;

alter table connector_remote_resources add column inspection_flight_id text, add column inspection_evidence_json jsonb, add constraint connector_resources_inspection_shape check((inspection_flight_id is null)=(inspection_evidence_json is null) and (inspection_flight_id is null or length(inspection_flight_id)>0) and (inspection_evidence_json is null or jsonb_typeof(inspection_evidence_json)='object'));

update connector_remote_resources r set inspection_flight_id=s.remote_flight_id,inspection_evidence_json=s.evidence_json from inspection_alert_sources s where r.id=s.remote_resource_id and r.project_id=s.project_id and r.connector_instance_id=s.connector_instance_id;

create index connector_resources_inspection_idx on connector_remote_resources(project_id,connector_instance_id,inspection_flight_id) where inspection_flight_id is not null;

drop table inspection_alert_sources;

alter table issue_links add column source_key text, add column assessment_id uuid, add constraint issue_links_assessment_project_fk foreign key(assessment_id,project_id) references inspection_assessments(id,project_id), add constraint issue_links_source_shape check((source_key is null)=(assessment_id is null) and (source_key is null or (link_type='inspection_source' and target_id=assessment_id::text)));

create unique index issue_links_source_unique on issue_links(project_id,source_key) where source_key is not null;

-- Multiple source keys may belong to the same issue and assessment. Ordinary
-- links deduplicate targets; source links deduplicate the original source key.
drop index issue_links_issue_target_unique;
create unique index issue_links_issue_target_unique on issue_links(issue_id,link_type,target_id) where source_key is null;

insert into issue_links(project_id,issue_id,link_type,target_id,source_key,assessment_id,created_at) select project_id,issue_id,'inspection_source',assessment_id::text,source_key,assessment_id,created_at from inspection_issue_sources;

drop table inspection_issue_sources;

alter table agent_drafts add column evidence_refs_json jsonb not null default '[]', add constraint agent_drafts_evidence_array check(jsonb_typeof(evidence_refs_json)='array');

update agent_drafts d set evidence_refs_json=(select jsonb_agg(jsonb_build_object('type',e.reference_type,'id',e.reference_id,'version',e.reference_version,'observedAt',e.observed_at,'quality',e.quality,'createdAt',e.created_at) order by e.id) from agent_draft_evidence e where e.agent_draft_id=d.id and e.project_id=d.project_id) where exists(select 1 from agent_draft_evidence e where e.agent_draft_id=d.id and e.project_id=d.project_id);

drop table agent_draft_evidence;

alter table approval_requests drop constraint approval_requests_requester_member_fk, add constraint approval_requests_requester_user_fk foreign key(requested_by_user_id) references users(id) on delete restrict;

alter table approvals drop constraint approvals_approver_member_fk, add constraint approvals_approver_user_fk foreign key(approver_user_id) references users(id) on delete restrict;

alter table agent_drafts drop constraint agent_drafts_actor_team_fk, add constraint agent_drafts_actor_user_fk foreign key(created_by_user_id) references users(id) on delete restrict;

alter table connector_open_model_uploads drop constraint connector_open_model_uploads_requester_member_fk, add constraint connector_open_model_uploads_requester_user_fk foreign key(requested_by_user_id) references users(id) on delete restrict;

alter table connector_control_sessions drop constraint connector_control_sessions_holder_member_fk,
 add constraint connector_control_sessions_holder_user_fk foreign key(holder_user_id) references users(id) on delete restrict;

-- A business workflow's ownership must survive late flight ACKs and cannot be reassigned.
create function preserve_connector_job_identity() returns trigger language plpgsql as $$
begin
 if row(new.id,new.project_id,new.team_id,new.connector_instance_id,new.job_type)
    is distinct from row(old.id,old.project_id,old.team_id,old.connector_instance_id,old.job_type) then
  raise exception 'CONNECTOR_JOB_IDENTITY_IMMUTABLE';
 end if;
 if old.business_step_id is not null and
    row(new.business_run_id,new.business_step_id,new.task_run_id,new.business_bound_at)
    is distinct from row(old.business_run_id,old.business_step_id,old.task_run_id,old.business_bound_at) then
  raise exception 'INSPECTION_FLIGHT_BINDING_IMMUTABLE';
 end if;
 return new;
end;
$$;

create trigger connector_jobs_preserve_identity before update on connector_jobs
 for each row execute function preserve_connector_job_identity();
