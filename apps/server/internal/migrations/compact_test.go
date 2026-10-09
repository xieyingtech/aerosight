package migrations

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"aerosight/server/internal/testdb"
)

// Seed the previous production schema, rather than constructing the new shape.
const compactUpgradeFixture = `
insert into users(id,name) values(1001,'historical operator'),(1002,'approver');
insert into teams(id,name) values(1001,'upgrade team');
insert into team_members(team_id,user_id,role) values(1001,1001,'member'),(1001,1002,'admin');
insert into projects(id,team_id,name) values(1001,1001,'upgrade'),(1002,1001,'foreign project');
insert into device_adapters(id,project_id,team_id,name,adapter_type,connector_definition_id)
 values(1001,1001,1001,'adapter','dji-flighthub2',(select id from connector_definitions where connector_key='dji.flighthub2' limit 1));
insert into devices(id,project_id,adapter_id,name,type,device_type_id)
 values(1001,1001,1001,'drone','drone',(select id from device_types where type_key='dji.dock2' limit 1));
insert into assets(id,project_id,team_id,kind,storage_key,logical_key) values(1001,1001,1001,'image','projects/1001/image.jpg','image');
insert into assets(id,project_id,team_id,kind,storage_key,logical_key) values(1002,1002,1001,'image','projects/1002/foreign.jpg','foreign');
insert into safety_policy_versions(id,project_id,team_id,version,max_altitude_meters,max_speed_meters_per_second,minimum_battery_percent) values(1001,1001,1001,1,100,10,20);
update projects set current_safety_policy_version_id=1001 where id=1001;
insert into sensor_calibrations(project_id,team_id,device_id,sensor_key,version,valid_from) values(1001,1001,1001,'camera',1,now());
insert into retention_holds(project_id,team_id,asset_id,reason) values(1001,1001,1001,'preserve original reason');
insert into tasks(id,project_id,team_id,name,trigger_type,script) values(1001,1001,1001,'upgrade','manual','{}');
insert into task_versions(id,project_id,team_id,task_id,version,script) values(1001,1001,1001,1001,1,'{}');
insert into task_steps(id,project_id,team_id,task_version_id,position,step_key,name,action) values(1001,1001,1001,1001,1,'observe','observe','inspection.observe');
insert into task_runs(id,project_id,team_id,task_id,task_version_id,trigger_source,safety_policy_version_id) values(1001,1001,1001,1001,1001,'manual',1001),(1002,1001,1001,1001,1001,'manual',null);
insert into task_run_steps(id,project_id,team_id,task_run_id,task_step_id,position) values(1001,1001,1001,1002,1001,1);
insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,expires_at)
 values('00000000-0000-4000-8000-000000001001',1001,1001,'task_run','1001','flight',1001,now()+interval '1 hour');
insert into approvals(project_id,team_id,approval_request_id,approver_user_id,decision,reason) values(1001,1001,'00000000-0000-4000-8000-000000001001',1002,'approved','upgrade approval');
insert into connector_remote_resources(id,project_id,team_id,connector_instance_id,resource_kind,remote_id)
 values(1001,1001,1001,1001,'wayline','wayline'),(1002,1001,1001,1001,'model','model'),(1003,1001,1001,1001,'ai-alert','alert');
insert into inspection_alert_sources(project_id,connector_instance_id,remote_resource_id,remote_flight_id,evidence_json) values(1001,1001,1003,'private-flight','{"reason":"original evidence"}');
insert into inspection_connector_policies(project_id,team_id,connector_instance_id,task_managed_alerts) values(1001,1001,1001,true);
insert into connector_action_jobs(project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json)
 values(1001,1001,1001,1001,1001,1001,'00000000-0000-4000-8000-000000001001',1001,'flight-task-create','upgrade-key',repeat('a',64),'{}');
insert into inspection_flight_bindings(project_id,team_id,business_run_id,business_step_id,connector_instance_id,flight_run_id,action_job_id)
 select 1001,1001,1002,1001,1001,1001,id from connector_action_jobs;
insert into connector_live_action_jobs(project_id,team_id,connector_instance_id,device_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json)
 values(1001,1001,1001,1001,1001,'live-quality-set','live.quality.set','flighthub.live.quality','upgrade-key',repeat('a',64),'{}');
insert into connector_geospatial_action_jobs(project_id,team_id,connector_instance_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json)
 values(1001,1001,1001,1001,'map-element-create','geospatial.write','flighthub.actions','upgrade-key',repeat('a',64),'{}');
insert into connector_model_jobs(project_id,team_id,connector_instance_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json)
 values(1001,1001,1001,1001,'traditional-create','upgrade-key',repeat('a',64),'{}');
insert into connector_model_delete_jobs(project_id,team_id,connector_instance_id,target_resource_id,approval_request_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,preview_digest,request_digest,request_envelope_json)
 values(1001,1001,1001,1002,'00000000-0000-4000-8000-000000001001',1001,'model-delete','model.delete','flighthub.model.delete','upgrade-key','v1',repeat('a',64),repeat('a',64),'{}');
insert into connector_object_upload_jobs(project_id,team_id,connector_instance_id,operation_kind,source_asset_id,requested_by_user_id,idempotency_key,requested_name,reconciliation_name)
 values(1001,1001,1001,'wayline',1001,1001,'upgrade-key','name','reconciliation');
insert into connector_device_admin_jobs(project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json)
 values(1001,1001,1001,1001,1001,'00000000-0000-4000-8000-000000001001','rtk-calibrate','device.rtk.calibrate','flighthub.rtk.calibrate','upgrade-key',repeat('a',64),'{}');
insert into connector_management_write_jobs(project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json)
 values(1001,1001,1001,1001,'00000000-0000-4000-8000-000000001001','project-member-upsert','organization.project-member.write','flighthub.organization.project-member','upgrade-key',repeat('a',64),'{}',repeat('a',64),'{}');
insert into agent_sessions(id,project_id) values(1001,1001);
insert into agent_drafts(id,project_id,team_id,session_id,created_by_user_id,draft_type,title) values('00000000-0000-4000-8000-000000002001',1001,1001,1001,1001,'report','original draft');
insert into agent_draft_evidence(project_id,agent_draft_id,reference_type,reference_id,reference_version,observed_at,quality)
 values(1001,'00000000-0000-4000-8000-000000002001','asset','1001','3','2026-10-08T10:00:00Z','verified');
insert into algorithm_providers(id,project_id,team_id,name,provider_type,base_url) values(1001,1001,1001,'upgrade provider','http-json','https://algo.invalid');
insert into algorithm_definitions(id,project_id,team_id,provider_id,name,capability_code) values(1001,1001,1001,1001,'upgrade algorithm','detection');
insert into algorithm_definition_versions(id,project_id,team_id,algorithm_definition_id,version,execution_mode,model_or_process) values(1001,1001,1001,1001,1,'synchronous','fixture');
insert into algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key) values('00000000-0000-4000-8000-000000002003',1001,1001,1001,1001,'upgrade-algorithm');
insert into detections(id,project_id,team_id,algorithm_run_id,input_asset_id,detection_key,label,confidence,pixel_geometry_json,transform_version,captured_at)
 values(1001,1001,1001,'00000000-0000-4000-8000-000000002003',1001,'person-1','person',0.9,'{}','fixture',now());
insert into detection_groups(id,project_id,team_id,label,location_quality,first_detected_at,last_detected_at) values(1001,1001,1001,'person','unavailable',now(),now());
insert into detection_group_members(project_id,team_id,detection_group_id,detection_id) values(1001,1001,1001,1001);
insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,scope_description,observed_from,observed_to)
 values('00000000-0000-4000-8000-000000002004',1001,1001,1002,1001,'assets','original observation',now(),now());
insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version)
 values('00000000-0000-4000-8000-000000002005',1001,1001,1002,1001,'00000000-0000-4000-8000-000000002004','external','fixture');
insert into inspection_assessments(id,project_id,team_id,task_run_id,task_run_step_id,evidence_set_id)
 values('00000000-0000-4000-8000-000000002006',1001,1001,1002,1001,'00000000-0000-4000-8000-000000002005');
insert into issues(id,project_id,number,title,source_type) values(1001,1001,1,'original issue','manual');
insert into inspection_issue_sources(project_id,source_key,issue_id,assessment_id) values(1001,'source:upgrade',1001,'00000000-0000-4000-8000-000000002006');
insert into inspection_issue_sources(project_id,source_key,issue_id,assessment_id) values(1001,'source:upgrade-2',1001,'00000000-0000-4000-8000-000000002006');
insert into audit_events(id,project_id,team_id,request_id,actor_user_id,action,resource_type,input_hash) values(1001,1001,1001,'project-audit',1001,'existing','asset','hash');
insert into platform_audit_events(id,actor_user_id,request_id,action,resource_type,input_hash) values(1001,1001,'platform-audit','existing','platform','hash');
`

func TestCompactSchemaUpgradePreservesDataAndScope(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	previous := fstest.MapFS{}
	for _, m := range list {
		if m.Name < "0090_" {
			previous[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	if _, err = Run(ctx, db, previous); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(compactUpgradeFixture); err != nil {
		t.Fatal("old fixture", err)
	}
	var originalIDs string
	if err = db.QueryRow(`select string_agg(id::text,',' order by id) from (
select id from connector_action_jobs union all select id from connector_live_action_jobs union all select id from connector_geospatial_action_jobs union all select id from connector_model_jobs union all select id from connector_model_delete_jobs union all select id from connector_object_upload_jobs union all select id from connector_device_admin_jobs union all select id from connector_management_write_jobs) jobs`).Scan(&originalIDs); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(ctx, db, source); err != nil {
		t.Fatal("upgrade", err)
	}
	var newIDs string
	if err = db.QueryRow(`select string_agg(id::text,',' order by id) from connector_jobs`).Scan(&newIDs); err != nil || newIDs != originalIDs {
		t.Fatal("job IDs changed", err)
	}
	checks := []string{
		`select count(*)=8 and count(distinct job_type)=8 from connector_jobs`,
		`select business_run_id=1002 and business_step_id=1001 and task_run_id=1001 from connector_jobs where job_type='flight'`,
		`select task_managed_alerts from device_adapters where id=1001`,
		`select group_id=1001 and grouped_at is not null from detections where id=1001`,
		`select issue_id=1001 and link_type='inspection_source' and target_id=assessment_id::text from issue_links where project_id=1001 and source_key='source:upgrade'`,
		`select count(*)=2 from issue_links where project_id=1001 and issue_id=1001 and assessment_id='00000000-0000-4000-8000-000000002006' and source_key is not null`,
		`select legal_hold and retention_reason='preserve original reason' from assets where id=1001`,
		`select inspection_flight_id='private-flight' and inspection_evidence_json->>'reason'='original evidence' and not(summary_json ? 'reason') from connector_remote_resources where id=1003`,
		`select evidence_refs_json->0->>'id'='1001' and evidence_refs_json->0->>'version'='3' and evidence_refs_json->0->>'quality'='verified' from agent_drafts`,
		`select count(*)=3 from audit_events where action='schema.archive'`,
		`select details_json->>'reason'='preserve original reason' from audit_events where action='schema.archive' and resource_type='retention_holds'`,
		`select count(*)=1 from audit_events where scope='platform' and legacy_platform_id=1001 and project_id is null`,
		`select count(*)=1 from audit_events where scope='project' and request_id='project-audit' and id=1001`,
		`select count(*)=84 from pg_class c join pg_namespace n on n.oid=c.relnamespace where n.nspname='public' and c.relkind in('r','p') and c.relname not in('spatial_ref_sys','schema_migrations')`,
	}
	for _, query := range checks {
		var ok bool
		if err = db.QueryRow(query).Scan(&ok); err != nil || !ok {
			t.Fatalf("%s: %v %v", query, ok, err)
		}
	}
	for _, query := range []string{
		`update connector_jobs set business_run_id=1001 where job_type='flight'`,
		`update connector_jobs set project_id=1002 where job_type='flight'`,
		`update connector_jobs set job_type='live' where job_type='flight'`,
		`update connector_jobs set status='succeeded' where job_type='live'`,
		`update connector_jobs set device_id=null where job_type='flight'`,
		`update connector_jobs set source_asset_id=1002 where job_type='object-upload'`,
	} {
		result, err := db.Exec(query)
		if err == nil {
			count, _ := result.RowsAffected()
			if count != 0 {
				t.Fatalf("invalid update accepted: %s", query)
			}
		}
	}
	if _, err = db.Exec(`delete from team_members where team_id=1001 and user_id=1001`); err != nil {
		t.Fatal("historical records prevented leaving team", err)
	}
	if _, err = db.Exec(`delete from detection_groups where id=1001`); err != nil {
		t.Fatal(err)
	}
	var ungrouped bool
	if err = db.QueryRow(`select group_id is null from detections where id=1001`).Scan(&ungrouped); err != nil || !ungrouped {
		t.Fatal("deleting group deleted its detection", err)
	}
	var count int
	if err = db.QueryRow(`select count(*) from connector_jobs`).Scan(&count); err != nil || count != 8 {
		t.Fatal("leaving team lost historical jobs", count, err)
	}
}
