package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestFlightHubMainControlAndLivePostgres(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d want %d %+v", method, path, res.StatusCode, want, data)
		}
		return data
	}
	cid := call("POST", path, gin.H{"token": "stored-token", "projectUuid": testFlightHubProject}, 201)["id"].(string)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	_, did := f.device(t, team, pid)
	exec("update device_adapters set status='connected',discovery_scope_json=discovery_scope_json||jsonb_build_object('accountFingerprint',repeat('a',64)) where id=$1", cid)
	exec("update devices set adapter_id=$1,device_model='test-model',firmware_version='test-fw',status='online' where id=$2", cid, did)
	exec("insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'device-test','managed')", pid, team, cid, did)
	exec("insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{\"device.control\":true,\"live.control\":true}') on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json", pid)
	exec("insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,account_fingerprint,device_model,firmware_version,verified_at) values($1,$2,$3,'device.control','supported','field-write','cn','cn-public-cloud',repeat('a',64),'test-model','test-fw',now()),($1,$2,$3,'live.control','supported','field-write','cn','cn-public-cloud',repeat('a',64),'test-model',null,now())", pid, team, cid)
	var policy int
	if e := f.db.QueryRow("insert into safety_policy_versions(project_id,team_id,version,status,max_altitude_meters,max_speed_meters_per_second,minimum_battery_percent) values($1,$2,1,'published',100,10,20) returning id", pid, team).Scan(&policy); e != nil {
		t.Fatal(e)
	}
	exec("update projects set current_safety_policy_version_id=$1 where id=$2", policy, pid)
	exec("insert into device_latest_telemetry(device_id,project_id,adapter_id,event_id,telemetry_type,captured_at,received_at,payload_json) values($1,$2,$3,'state-test','dji.flighthub.state',now(),now(),'{}')", did, pid, cid)
	const approval = "ab111111-1111-4111-8111-111111111111"
	exec("insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,expires_at) select $1,$2,$3,'device',$4,'flighthub.control.acquire',id,'approved',now()+interval '1 hour' from users where email='admin@example.com'", approval, pid, team, fmt.Sprint(did))
	controlURL := fmt.Sprintf("/api/projects/%d/devices/%d/flighthub-control-sessions", pid, did)
	var connectorID int
	if _, e := fmt.Sscan(cid, &connectorID); e != nil {
		t.Fatal(e)
	}
	body := gin.H{"connectorInstanceId": connectorID, "safetyPolicyVersionId": policy, "approvalRequestId": approval, "idempotencyKey": "control-main-test", "controls": gin.H{"flight": true}}
	first := call("POST", controlURL, body, 202)
	if replay := call("POST", controlURL, body, 200); replay["id"] != first["id"] || replay["reused"] != true {
		t.Fatalf("replay %+v", replay)
	}
	id := first["id"].(string)
	sessionURL := controlURL + "/" + id
	exec("update connector_control_sessions set status='active',last_heartbeat_at=now()-interval '1 second' where id=$1", id)
	call("PATCH", sessionURL, gin.H{"action": "heartbeat"}, 200)
	call("PATCH", sessionURL, gin.H{"action": "heartbeat"}, 409)
	exec("insert into users(name,email) values('Other','control-other@example.com')")
	exec("insert into team_members(team_id,user_id,role) select $1,id,'member' from users where email='control-other@example.com'", team)
	exec("update connector_control_sessions set holder_user_id=(select id from users where email='control-other@example.com') where id=$1", id)
	call("PATCH", sessionURL, gin.H{"action": "release"}, 409)
	exec("update connector_control_sessions set holder_user_id=(select id from users where email='admin@example.com') where id=$1", id)
	call("PATCH", sessionURL, gin.H{"action": "release"}, 202)
	liveURL := fmt.Sprintf("/api/projects/%d/devices/%d/live-streams", pid, did)
	live := call("POST", liveURL, gin.H{"streamKey": "camera.main"}, 200)
	_ = live
	var streamID int
	if e := f.db.QueryRow("select id from live_streams where project_id=$1 and device_id=$2", pid, did).Scan(&streamID); e != nil {
		t.Fatal(e)
	}
	stopURL := fmt.Sprintf("/api/projects/%d/live-streams/%d", pid, streamID)
	call("POST", stopURL+"/stop", nil, 200)
	var status string
	var revoked bool
	if e := f.db.QueryRow("select status,local_authorization_revoked_at is not null from live_streams where id=$1", streamID).Scan(&status, &revoked); e != nil || status != "stopped" || !revoked {
		t.Fatalf("stop %s %v %v", status, revoked, e)
	}
}

func TestMainDiscoveryBindAndMigrate(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, _ := f.device(t, team, pid)
	if _, e := f.db.Exec("update device_adapters set status='connected' where id=$1", adapter); e != nil {
		t.Fatal(e)
	}
	var identity int
	var typeKey string
	if e := f.db.QueryRow("select type_key from device_types where status='active' order by id limit 1").Scan(&typeKey); e != nil {
		t.Fatal(e)
	}
	if e := f.db.QueryRow("insert into device_external_identities(project_id,team_id,adapter_id,external_device_id) values($1,$2,$3,'new-main') returning id", pid, team, adapter).Scan(&identity); e != nil {
		t.Fatal(e)
	}
	call := func(i int, body gin.H) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/device-adapters/discoveries/%d/bind", pid, i), string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("binding %d %+v", res.StatusCode, data)
		}
		return data
	}
	body := gin.H{"name": "Reviewed device", "deviceTypeKey": typeKey}
	first := call(identity, body)
	did := first["deviceId"]
	if replay := call(identity, body); replay["deviceId"] != did || replay["replayed"] != true {
		t.Fatalf("replay %+v", replay)
	}
	if _, e := f.db.Exec("update device_adapters set name='original-discovery' where id=$1", adapter); e != nil {
		t.Fatal(e)
	}
	if _,e:=f.db.Exec("update devices set name=name||'-original' where project_id=$1",pid);e!=nil{t.Fatal(e)}
 other, _ := f.device(t, team, pid)
	if _, e := f.db.Exec("update device_adapters set status='connected' where id=$1", other); e != nil {
		t.Fatal(e)
	}
	if e := f.db.QueryRow("insert into device_external_identities(project_id,team_id,adapter_id,external_device_id) values($1,$2,$3,'migrated-main') returning id", pid, team, other).Scan(&identity); e != nil {
		t.Fatal(e)
	}
	body["targetDeviceId"] = did
	call(identity, body)
	var active, standby int
	if e := f.db.QueryRow("select count(*) filter(where status='active'),count(*) filter(where status='standby') from device_connector_bindings where device_id=$1", did).Scan(&active, &standby); e != nil || active != 1 || standby != 1 {
		t.Fatalf("routes %d %d %v", active, standby, e)
	}
}
