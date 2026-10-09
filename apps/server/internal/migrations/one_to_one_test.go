package migrations

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

	"aerosight/server/internal/credentials"
	"aerosight/server/internal/testdb"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestInlineExtensionsPreserveRowsConstraintsAndParentLifetime(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	preCompact, previous := fstest.MapFS{}, fstest.MapFS{}
	for _, m := range list {
		if m.Name < "0090_" {
			preCompact[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
		if m.Name < "0092_" {
			previous[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	if _, err = Run(ctx, db, preCompact); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(compactUpgradeFixture); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(ctx, db, previous); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	check := func(q string) {
		t.Helper()
		var ok bool
		if e := db.QueryRow(q).Scan(&ok); e != nil || !ok {
			t.Fatalf("check %s: %v / %v", q, ok, e)
		}
	}
	reject := func(code, q string) {
		t.Helper()
		_, e := db.Exec(q)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != code {
			t.Fatalf("expected %s for %s, got %v", code, q, e)
		}
	}
	exec(`
insert into device_adapters(id,project_id,team_id,name,adapter_type) values(1101,1001,1001,'extension source','simulator'),(1102,1002,1001,'foreign source','simulator');
insert into devices(id,project_id,adapter_id,name,type,device_type_id) values(1101,1001,1101,'pose source','drone',(select id from device_types where type_key='dji.dock2' limit 1)),(1102,1002,1102,'foreign pose source','drone',(select id from device_types where type_key='dji.dock2' limit 1));
insert into connector_remote_resources(id,project_id,team_id,connector_instance_id,resource_kind,remote_id) values(1101,1001,1001,1101,'flight-media','private-media'),(1102,1002,1001,1102,'flight-media','foreign-media');
insert into device_commands(id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,deadline_at,status) values('00000000-0000-4000-8000-000000004001',1001,1001,1001,'start','command-1','flight',now(),'unknown'),('00000000-0000-4000-8000-000000004002',1001,1001,1001,'start','command-2','flight',now(),'dispatchable');
insert into device_command_protocol_correlations(id,project_id,team_id,command_id,adapter_id,mapping_version,transaction_id,business_id,method,request_topic,request_payload_json,status,reply_event_id,reply_result,reply_payload_json,sent_at,replied_at,created_at,updated_at)
 values(4101,1001,1001,'00000000-0000-4000-8000-000000004001',1101,'v1','tx-1','biz-1','start','request/topic','{"request":true}','unknown','late-reply',0,'{"late":true}','2026-10-08T10:01Z','2026-10-08T10:02Z','2026-10-08T10:00Z','2026-10-08T10:03Z');
insert into observations(id,project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at,original_geometry,standard_geometry)
 values(4101,1001,1001,1001,1001,'pose','legacy-position','2026-10-08T10:00Z','2026-10-08T10:02Z',ST_MakePoint(1,2,3),ST_SetSRID(ST_MakePoint(4,5,6),4326));
insert into poses(observation_id,project_id,device_id,captured_at,standard_position,original_position,orientation_x,orientation_y,orientation_z,orientation_w,velocity_x,velocity_y,velocity_z,horizontal_accuracy_m,vertical_accuracy_m,attitude_accuracy_deg,vertical_datum,transform_version,spatial_quality)
 values(4101,1001,1101,'2026-10-08T10:01Z',ST_SetSRID(ST_MakePoint(120,30,50),4326),ST_MakePoint(121,31,51),0.1,0.2,0.3,0.4,1,2,3,4,5,6,'ellipsoid','old-transform','degraded');`)
	secret := "local-extension-upgrade-test-secret"
	envelope, err := credentials.EncryptJSON(map[string]string{"mediaUUID": "private-media", "taskUUID": "original-task"}, secret, credentials.AAD("flighthub-asset-reference", 1001, 1001))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	exec(`insert into connector_asset_access_refs(id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json,created_at,updated_at) values(1001,1001,1001,1101,1101,'flight-media',repeat('a',64),$1,'2026-10-08T10:00Z','2026-10-08T10:03Z')`, raw)
	views := []string{"device_command_protocol_correlations", "connector_asset_access_refs", "poses"}
	snapshots := make(map[string][]byte)
	for _, v := range views {
		var row []byte
		if err = db.QueryRow("select to_jsonb(v) from " + v + " v").Scan(&row); err != nil {
			t.Fatal(err)
		}
		snapshots[v] = row
	}
	if _, err = Run(ctx, db, source); err != nil {
		t.Fatal("upgrade", err)
	}
	for _, v := range views {
		var same bool
		if err = db.QueryRow("select to_jsonb(v)=$1::jsonb from "+v+" v", snapshots[v]).Scan(&same); err != nil || !same {
			t.Fatalf("%s changed: %v", v, err)
		}
		check("select relkind='v' from pg_class where oid='" + v + "'::regclass")
	}
	var moved []byte
	if err = db.QueryRow("select remote_credential_envelope_json from assets where id=1001").Scan(&moved); err != nil {
		t.Fatal(err)
	}
	parsed, err := credentials.ParseEnvelope(moved)
	if err != nil {
		t.Fatal(err)
	}
	var locator map[string]string
	if err = credentials.DecryptJSON(parsed, secret, credentials.AAD("flighthub-asset-reference", 1001, 1001), &locator); err != nil || locator["mediaUUID"] != "private-media" {
		t.Fatal("access locator lost", err)
	}
	check(`select device_id=1001 and pose_device_id=1101 and captured_at<>pose_captured_at and ST_X(standard_geometry)=4 and ST_X(pose_standard_position)=120 from observations where id=4101`)
	check(`select nextval('device_commands_protocol_correlation_id_seq')>4101`)
	reject("23503", `update assets set remote_resource_id=1102 where id=1001`)
	reject("23503", `update device_commands set protocol_adapter_id=1102 where protocol_correlation_id=4101`)
	reject("23503", `insert into poses(observation_id,project_id,device_id,captured_at) values(4101,1002,1101,now())`)
	reject("23503", `update poses set device_id=1102 where observation_id=4101`)
	reject("23505", `insert into poses(observation_id,project_id,device_id,captured_at) values(4101,1001,1101,now())`)
	reject("23514", `update poses set horizontal_accuracy_m=-1 where observation_id=4101`)
	reject("23514", `update connector_asset_access_refs set project_id=1002 where id=1001`)
	reject("23505", `insert into device_command_protocol_correlations(project_id,team_id,command_id,adapter_id,mapping_version,transaction_id,business_id,method,request_topic,request_payload_json) values(1001,1001,'00000000-0000-4000-8000-000000004002',1101,'v1','tx-1','biz-2','start','topic','{}')`)
	reject("23505", `insert into device_command_protocol_correlations(project_id,team_id,command_id,adapter_id,mapping_version,transaction_id,business_id,method,request_topic,request_payload_json) values(1001,1001,'00000000-0000-4000-8000-000000004002',1101,'v1','tx-2','biz-1','start','topic','{}')`)
	exec(`insert into assets(id,project_id,team_id,kind,storage_key,logical_key) values(1101,1001,1001,'image','extra-image','extra-image')`)
	reject("23505", `insert into connector_asset_access_refs(id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json) values(1101,1001,1001,1101,1101,'flight-media',repeat('a',64),'{}')`)
	// Legacy deletes remove only an extension; business parent state stays intact.
	exec(`delete from device_command_protocol_correlations where id=4101; delete from connector_asset_access_refs where id=1001; delete from poses where observation_id=4101;`)
	check(`select status='unknown' and protocol_status is null from device_commands where id='00000000-0000-4000-8000-000000004001'`)
	check(`select remote_access_kind is null from assets where id=1001`)
	check(`select pose_spatial_quality is null and ST_X(standard_geometry)=4 from observations where id=4101`)
	// Compatibility inserts still honor old defaults and updates affect only extensions.
	exec(`insert into device_command_protocol_correlations(project_id,team_id,command_id,adapter_id,mapping_version,transaction_id,business_id,method,request_topic,request_payload_json) values(1001,1001,'00000000-0000-4000-8000-000000004001',1101,'v1','tx-3','biz-3','start','topic','{}');
update device_command_protocol_correlations set status='sent',sent_at=now() where command_id='00000000-0000-4000-8000-000000004001';
insert into connector_asset_access_refs(id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json) values(1001,1001,1001,1101,1101,'flight-media',repeat('a',64),'{}');
insert into poses(observation_id,project_id,device_id,captured_at) values(4101,1001,1101,now());`)
	check(`select status='unknown' and protocol_status='sent' from device_commands where id='00000000-0000-4000-8000-000000004001'`)
	check(`select count(*)=3 from pg_indexes where indexname in('observations_pose_device_time_idx','observations_pose_project_time_idx','observations_pose_position_idx')`)
	// A pose may historically belong to another device than its generic parent.
	exec(`delete from devices where id=1101`)
	check(`select pose_spatial_quality is null and device_id=1001 from observations where id=4101`)
	exec(`delete from connector_remote_resources where id=1101`)
	check(`select remote_credential_envelope_json is null from assets where id=1001`)
	exec(`insert into connector_remote_resources(id,project_id,team_id,connector_instance_id,resource_kind,remote_id) values(1103,1001,1001,1101,'flight-media','second-private-media');
insert into connector_asset_access_refs(id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json) values(1001,1001,1001,1101,1103,'flight-media',repeat('b',64),'{}');`)
	exec(`delete from device_adapters where id=1101`)
	check(`select remote_credential_envelope_json is null and remote_connector_id is null from assets where id=1001`)
	check(`select protocol_status is null and status='unknown' from device_commands where id='00000000-0000-4000-8000-000000004001'`)
	for _, v := range views {
		check("select count(*)=0 from " + v)
	}
}
