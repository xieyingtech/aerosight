package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRealtimeFlightHubDirectRBACExecution(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	f.server.credentialSecret = testFlightHubSecret
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d expected %d: %+v", method, path, res.StatusCode, want, data)
		}
		return data
	}
	cid := call("POST", path, gin.H{"token": "stored-token", "projectUuid": testFlightHubProject}, 201)["id"].(string)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	_, did := f.device(t, team, pid)
	exec("update devices set device_type_id=(select id from device_types where type_key='dji.dock2' and status='active' limit 1),adapter_id=$1,device_model='test-model',firmware_version='test-fw',status='online' where id=$2", cid, did)
	exec("update device_adapters set status='connected',discovery_scope_json=discovery_scope_json||jsonb_build_object('accountFingerprint',repeat('a',64)) where id=$1", cid)
	exec("insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'device-test','managed')", pid, team, cid, did)
	exec("insert into device_connector_bindings(project_id,team_id,device_id,connector_instance_id,external_identity_id,priority,status) select $1,$2,$3,$4,id,100,'active' from device_external_identities where project_id=$1 and adapter_id=$4 and device_id=$3", pid, team, did, cid)
	exec("insert into device_capabilities(device_id,project_id,capability_code,risk_level) values($1,$2,'flight.return_home','critical')", did, pid)
	exec("insert into device_latest_telemetry(device_id,project_id,adapter_id,event_id,telemetry_type,captured_at,received_at,payload_json) values($1,$2,$3,'state-test','dji.flighthub.state',now(),now(),'{\"mode\":\"manual_flight\"}')", did, pid, cid)
	exec("insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{\"device.control\":true}') on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json", pid)
	exec(`insert into device_capabilities(device_id,project_id,capability_code,risk_level)
      select $1,$2,entry.key,'low' from device_types type cross join lateral jsonb_each(type.capability_profile_json) entry
      where type.id=(select device_type_id from devices where id=$1) and entry.value->>'enabled'='true'
      on conflict(device_id,capability_code) do nothing`, did, pid)
	endpoint := fmt.Sprintf("/api/projects/%d/devices/%d/flighthub-operations", pid, did)
	read := call("GET", endpoint, nil, 200)
	if len(read["operations"].([]any)) != 5 {
		t.Fatal("dock operation catalog incomplete", read)
	}
	body := gin.H{"action": "execute", "operation": "return_home", "parameters": gin.H{}, "idempotencyKey": "manual-return-test"}
	call("POST", endpoint, body, 200) // No prior field acceptance or safety policy required.
	call("POST", endpoint, gin.H{"action": "request", "operation": "return_home", "parameters": gin.H{}, "idempotencyKey": "old-request-mode"}, 400)
	call("POST", endpoint, gin.H{"action": "execute", "operation": "camera.change_lens", "parameters": gin.H{"cameraIndex": "CAMERA", "lensType": "wide"}, "idempotencyKey": "invalid-dock-lens"}, 400)
	_, foreign := f.project(t)
	call("POST", fmt.Sprintf("/api/projects/%d/devices/%d/flighthub-operations", foreign, did), body, 404)
	exec("insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,account_fingerprint,device_model,firmware_version,verified_at) values($1,$2,$3,'device.control','supported','field-write','cn','cn-public-cloud',repeat('a',64),'test-model','test-fw',now())", pid, team, cid)
	first := call("POST", endpoint, body, 200)
	repeat := call("POST", endpoint, body, 200)
	if first["id"] != repeat["id"] {
		t.Fatal("duplicate execution")
	}
	call("POST", endpoint, gin.H{"action": "execute", "operation": "flighttask_pause", "parameters": gin.H{}, "idempotencyKey": "manual-return-test"}, 409)
	var count int
	if err := f.db.QueryRow("select count(*) from device_commands where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate dispatch", count, err)
	}
	if err := f.db.QueryRow("select count(*) from approvals where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatal("manual operation required another approver", count, err)
	}
	read = call("GET", endpoint, nil, 200)
	if read["executions"].([]any)[0].(map[string]any)["receipt"] == nil {
		t.Fatal("receipt missing", read)
	}
	exec("insert into device_capabilities(device_id,project_id,capability_code,risk_level) values($1,$2,'camera.change','low') on conflict(device_id,capability_code) do nothing", did, pid)
	exec(`update project_feature_flags set flighthub_action_flags_json=flighthub_action_flags_json||'{"flighthub.camera.change":true}'::jsonb where project_id=$1`, pid)
	exec("insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,account_fingerprint,device_model,firmware_version,verified_at) values($1,$2,$3,'device.camera.change','supported','field-write','cn','cn-public-cloud',repeat('a',64),'test-model','test-fw',now())", pid, team, cid)
	exec("update projects set current_safety_policy_version_id=null where id=$1", pid)
	call("POST", endpoint, gin.H{"action": "execute", "operation": "camera.change", "parameters": gin.H{"cameraIndex": "165-0-7", "cameraPosition": "indoor"}, "idempotencyKey": "manual-camera-test"}, 200)
	exec(`update device_types set capability_profile_json=capability_profile_json-'camera.change' where id=(select device_type_id from devices where id=$1)`, did)
	withoutCamera := call("GET", endpoint, nil, 200)
	for _, item := range withoutCamera["operations"].([]any) {
		if item.(map[string]any)["key"] == "camera.change" {
			t.Fatal("stale instance capability added a button absent from type")
		}
	}
	call("POST", endpoint, gin.H{"action": "execute", "operation": "camera.change", "parameters": gin.H{"cameraIndex": "165-0-7", "cameraPosition": "indoor"}, "idempotencyKey": "removed-camera-test"}, 400)
	secret := "rtk-secret-never-returned"
	call("POST", endpoint, gin.H{"action": "execute", "operation": "rtk-calibrate", "parameters": gin.H{"host": "rtk.example.com", "port": 2101, "account": "operator", "password": secret, "mountPoint": "mount"}, "idempotencyKey": "manual-rtk-test"}, 403)
	var contextRaw string
	if err := f.db.QueryRow("select context_json::text from approval_requests where project_id=$1 and context_json->>'operation'='rtk-calibrate'", pid).Scan(&contextRaw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contextRaw, secret) {
		t.Fatal("secret unencrypted")
	}
	raw, _ := json.Marshal(call("GET", endpoint, nil, 200))
	if strings.Contains(string(raw), secret) {
		t.Fatal("secret exposed")
	}
	exec("update team_members set role='member' where team_id=$1 and user_id=(select id from users where email='admin@example.com')", team)
	call("POST", endpoint, body, 403)
}

func TestRealtimeFlightControlsFollowState(t *testing.T) {
	for _, test := range []struct {
		mode, task string
		fresh      bool
		want       []string
	}{
		{"standby", "", true, nil},
		{"not_connected", "", true, nil},
		{"manual_flight", "", true, []string{"return_home"}},
		{"auto_returning_to_home", "executing", true, []string{"return_home_cancel"}},
		{"wayline_flight", "executing", true, []string{"return_home", "flighttask_pause"}},
		{"wayline_flight", "paused", true, []string{"return_home", "flighttask_recovery"}},
		{"wayline_flight", "suspended", true, []string{"return_home"}},
		{"automatic_landing", "executing", true, nil},
		{"manual_flight", "", false, nil},
	} {
		state := fhRealtimeFlightState{Mode: test.mode, TaskStatus: test.task, Fresh: test.fresh}
		for _, key := range []string{"return_home", "return_home_cancel", "flighttask_pause", "flighttask_recovery"} {
			want := false
			for _, expected := range test.want {
				if key == expected {
					want = true
				}
			}
			if state.allows(key) != want {
				t.Fatalf("%+v: %s allowed=%v want=%v", state, key, state.allows(key), want)
			}
		}
		if !state.allows("camera.change") {
			t.Fatal("flight status hid camera operation")
		}
	}
}

func TestRealtimeOperationParameterContracts(t *testing.T) {
	for _, operation := range fhRealtimeCatalog() {
		if operation.Kind == "command" && operation.Key != "camera.change" && operation.Key != "camera.change_lens" {
			if _, err := fhRealtimeBody(operation, gin.H{"invalid": "value"}, 1, 2, "11111111-1111-4111-8111-111111111111"); err == nil {
				t.Fatalf("accepted extra command parameter: %s", operation.Key)
			}
		}
	}
	if _, ok := fhRealtimeFind("cover.open", "dji.dock2", map[string]string{"dock.debug.control": "available"}); ok {
		t.Fatal("Cloud-only control exposed")
	}
	if _, ok := fhRealtimeFind("return_home", "dji.matrice3td", map[string]string{"flight.return_home": "available"}); ok {
		t.Fatal("aircraft used as dock command target")
	}
}

func TestRealtimeOperationsFollowCapabilities(t *testing.T) {
	for _, operation := range fhRealtimeCatalog() {
		if _, ok := fhRealtimeFind(operation.Key, "dji.dock2", map[string]string{}); ok {
			t.Fatalf("operation without capability: %s", operation.Key)
		}
	}
	if _, ok := fhRealtimeFind("live-quality-set", "custom.model", map[string]string{"stream.video.quality": "available"}); !ok {
		t.Fatal("capability operation still tied to hardcoded model list")
	}
	if _, ok := fhRealtimeFind("control.acquire", "dji.matrice3td", map[string]string{"camera.lens.change": "available"}); ok {
		t.Fatal("lens capability inferred control authority")
	}
}
