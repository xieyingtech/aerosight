package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFlightHubMainWorkspaceQueriesExecute(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	for _, section := range []string{"flight-operations", "geospatial", "models"} {
		response := f.request(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/%s", pid, section), "")
		data := decodedResponse(t, response)
		if response.StatusCode != 200 {
			t.Fatalf("%s: %d %+v", section, response.StatusCode, data)
		}
		if data["connectors"] == nil {
			t.Fatalf("%s missing empty connector array", section)
		}
	}
}
func TestFlightHubResourceActionInputAndGovernance(t *testing.T) {
	input, err := parseFHInput([]byte(`{"connectorInstanceId":2,"action":"live-quality-set","deviceId":3,"idempotencyKey":"  key-123456  ","request":{"cameraIndex":" camera ","qualityType":"smooth"}}`), fhLiveSchema)
	if err != nil {
		t.Fatal(err)
	}
	if input["idempotencyKey"] != "key-123456" || input["request"].(map[string]any)["cameraIndex"] != "camera" {
		t.Fatal("normalization differs from main")
	}
	raw, _ := json.Marshal(input)
	if _, err := parseFHInput([]byte(strings.Replace(string(raw), `"smooth"`, `"unknown"`, 1)), fhLiveSchema); err == nil {
		t.Fatal("invalid quality accepted")
	}
	row := gin.H{"teamId": float64(1), "role": "member", "hasOperatePermission": true, "connectorProjectId": float64(10), "connectorTeamId": float64(1), "connectorStatus": "connected", "actionEnabled": true, "capabilityFieldVerified": true, "deviceProjectId": float64(10), "deviceConnectorIdentityPresent": true}
	policy := fhActionPolicies["live-quality-set"]
	if err := fhAuthorizeResourceAction("live", 10, 2, input, row, policy); err != nil {
		t.Fatal(err)
	}
	row["capabilityFieldVerified"] = false
	if err := fhAuthorizeResourceAction("live", 10, 2, input, row, policy); err == nil {
		t.Fatal("unverified live action accepted")
	}
	row["capabilityFieldVerified"] = true
	row["deviceProjectId"] = float64(99)
	if err := fhAuthorizeResourceAction("live", 10, 2, input, row, policy); err == nil {
		t.Fatal("cross-project device accepted")
	}
}

func TestFlightHubMainGovernedWritesPostgres(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	f.server.credentialSecret = testFlightHubSecret
	response := f.request(t, "POST", path, `{"token":"stored-token","projectUuid":"`+testFlightHubProject+`"}`)
	created := decodedResponse(t, response)
	if response.StatusCode != 201 {
		t.Fatalf("create %+v", created)
	}
	cid := created["id"].(string)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`update device_adapters set status='connected',discovery_scope_json=discovery_scope_json||jsonb_build_object('accountFingerprint',repeat('a',64)) where id=$1`, cid)
	exec(`insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{"flighthub.model.delete":true,"flighthub.organization.project-member":true}') on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json`, pid)
	for _, cap := range []string{"model.delete", "organization.project-member.write", "organization.read"} {
		exec(`insert into connector_capability_snapshots(project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,account_fingerprint,verified_at) values($1,$2,$3,$4,'supported','field-write','cn','cn-public-cloud',repeat('a',64),now()-interval '1 second')`, pid, team, cid, cap)
	}
	var target int64
	if e := f.db.QueryRow(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version) values($1,$2,$3,'model','model-1','v1') returning id`, pid, team, cid).Scan(&target); e != nil {
		t.Fatal(e)
	}
	// Execute every workspace with a real connector, not just an empty result set.
	for _, url := range []string{fmt.Sprintf("/api/projects/%d/flight-operations", pid), fmt.Sprintf("/api/projects/%d/geospatial", pid), fmt.Sprintf("/api/projects/%d/models", pid), path + "/" + cid + "/diagnostics", path + "/" + cid + "/controlled-operations", path + "/" + cid + "/management"} {
		res := f.request(t, "GET", url, "")
		data := decodedResponse(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("read %s: %d %+v", url, res.StatusCode, data)
		}
	}
	modelURL := path + "/" + cid + "/model-actions"
	res := f.request(t, "GET", fmt.Sprintf("%s?action=model-delete&targetResourceId=%d", modelURL, target), "")
	preview := decodedResponse(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("preview %d %+v", res.StatusCode, preview)
	}
	const approval = "aa111111-1111-4111-8111-111111111111"
	exec(`insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,expires_at,context_json) select $1,$2,$3,'connector_remote_resource',$4,'flighthub.model.delete',id,'approved',now()+interval '1 hour',jsonb_build_object('previewDigest',$5::text,'expectedRemoteVersion','v1') from users where email='admin@example.com'`, approval, pid, team, fmt.Sprint(target), preview["previewDigest"])
	body := gin.H{"action": "model-delete", "targetResourceId": target, "approvalRequestId": approval, "expectedRemoteVersion": "v1", "previewDigest": preview["previewDigest"], "idempotencyKey": "model-test-key", "request": gin.H{"confirmation": "DELETE"}}
	call := func(method, url string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, url, string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %s got %d want %d: %+v", method, url, res.StatusCode, want, data)
		}
		return data
	}
	first := call("POST", modelURL, body, 202)
	second := call("POST", modelURL, body, 202)
	if first["id"] != second["id"] || second["reused"] != true {
		t.Fatalf("model idempotency %+v %+v", first, second)
	}
	call("GET", modelURL+"?jobId="+first["id"].(string), nil, 200)
	exec(`update connector_remote_resources set remote_version='v2' where id=$1`, target)
	call("POST", modelURL, body, 409)
	exec(`update connector_remote_resources set remote_version='v1' where id=$1`, target)
	exec(`update approval_requests set status='rejected' where id=$1`, approval)
	call("POST", modelURL, body, 403)
	// Management preview uses hashed organization identities; raw IDs stay encrypted.
	members := gin.H{"members": []any{map[string]any{"userId": "private-user-123", "role": "project-member", "nickname": "Example"}}}
	keys, e := fhMemberKeys(members)
	if e != nil {
		t.Fatal(e)
	}
	exec(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,summary_json) values($1,$2,$3,'organization','org','{"name":"Organization"}'),($1,$2,$3,'organization-user',$4,'{}')`, pid, team, cid, keys[0])
	memberURL := path + "/" + cid + "/management-actions"
	mp := call("PUT", memberURL, members, 200)
	const memberApproval = "bb111111-1111-4111-8111-111111111111"
	exec(`insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,expires_at,context_json) select $1,$2,$3,'connector',$4,'flighthub.organization.project-member-upsert',id,'approved',now()+interval '1 hour',jsonb_build_object('previewDigest',$5::text) from users where email='admin@example.com'`, memberApproval, pid, team, cid, mp["previewDigest"])
	members["confirmation"], members["previewDigest"], members["approvalRequestId"], members["idempotencyKey"] = "ADD PROJECT MEMBER", mp["previewDigest"], memberApproval, "member-test-key"
	mf := call("POST", memberURL, members, 202)
	ms := call("POST", memberURL, members, 202)
	if mf["id"] != ms["id"] || ms["reused"] != true {
		t.Fatal("member idempotency")
	}
	call("GET", memberURL+"?jobId="+mf["id"].(string), nil, 200)
	var jobs, outbox int
	if e = f.db.QueryRow(`select (select count(*) from connector_model_delete_jobs where project_id=$1)+(select count(*) from connector_management_write_jobs where project_id=$1),(select count(*) from outbox_events where project_id=$1 and event_type in('flighthub.model_delete.requested','flighthub.management_write.requested'))`, pid).Scan(&jobs, &outbox); e != nil || jobs != 2 || outbox != 2 {
		t.Fatalf("atomic dedupe jobs=%d outbox=%d %v", jobs, outbox, e)
	}
	call("POST", path+"/"+cid+"/diagnostics", nil, 202)
	call("DELETE", path+"/"+cid, nil, 200)
	call("PUT", path+"/"+cid, nil, 200)
	call("PUT", path+"/"+cid, nil, 409)
}

func TestTaskDraftMainRoundTrip(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var tid int
	if e := f.db.QueryRow(`insert into tasks(project_id,team_id,name,script,trigger_type) values($1,$2,'typed task','legacy','manual') returning id`, pid, team).Scan(&tid); e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/api/projects/%d/tasks/%d/versions", pid, tid)
	call := func(body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, "POST", path, string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("draft %d: %+v", res.StatusCode, data)
		}
		return data
	}
	created := call(gin.H{"action": "create"}, 200)
	vid := created["draft"].(map[string]any)["id"]
	if call(gin.H{"action": "create"}, 200)["replayed"] != true {
		t.Fatal("draft not reused")
	}
	schema := gin.H{"type": "object"}
	definition := gin.H{"name": "Main typed task", "inputSchema": schema, "trigger": gin.H{"type": "manual"}, "concurrencyLimit": 1, "steps": []any{gin.H{"key": "report", "name": "Report", "uses": "report.generate", "inputSchema": schema, "outputSchema": schema, "timeoutSeconds": 60, "retry": gin.H{"maxAttempts": 1, "backoffSeconds": 0}, "onFailure": "abort"}}}
	saved := call(gin.H{"action": "save", "versionId": vid, "definition": definition}, 200)
	if saved["stepCount"] != float64(1) {
		t.Fatal("steps missing")
	}
	res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/tasks/%d/workbench", pid, tid), "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || data["canEdit"] != true {
		t.Fatalf("workbench %+v", data)
	}
	call(gin.H{"action": "publish", "versionId": vid}, 200)
	triggerPath := fmt.Sprintf("/api/projects/%d/tasks/%d/runs", pid, tid)
	trigger := func(key string, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(gin.H{"type": "manual", "idempotencyKey": key, "occurredAt": "2026-09-10T00:00:00Z", "inputs": gin.H{}})
		response := f.request(t, "POST", triggerPath, string(raw))
		data := decodedResponse(t, response)
		if response.StatusCode != want {
			t.Fatalf("trigger %d want %d %+v", response.StatusCode, want, data)
		}
		return data
	}
	trigger("main-trigger", 201)
	trigger("main-trigger", 200)
	trigger("main-concurrency", 409)

	call(gin.H{"action": "save", "versionId": vid, "definition": definition}, 400)
	next := call(gin.H{"action": "create"}, 200)
	if next["draft"].(map[string]any)["version"] != float64(2) {
		t.Fatal("next version not copied")
	}
}

func TestMainChangedRouteInventory(t *testing.T) {
	f := newAPIFixture(t)
	raw, e := os.ReadFile("../../../../contracts/go-migration/main-route-inventory.json")
	if e != nil {
		t.Fatal(e)
	}
	var inventory struct {
		Routes []struct{ Method, Path string }
	}
	if e = json.Unmarshal(raw, &inventory); e != nil {
		t.Fatal(e)
	}
	routes := map[string]bool{}
	for _, route := range f.server.router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range inventory.Routes {
		if !routes[route.Method+" "+route.Path] {
			t.Errorf("main route missing: %s %s", route.Method, route.Path)
		}
	}
}
