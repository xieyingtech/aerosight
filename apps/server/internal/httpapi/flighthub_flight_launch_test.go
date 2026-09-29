package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"aerosight/server/internal/credentials"
	"aerosight/server/internal/flighthub"
	"context"
	"github.com/gin-gonic/gin"
)

func TestManualFlightLaunchScopedIdempotentRBAC(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	f.server.credentialSecret = testFlightHubSecret
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	_, did := f.device(t, team, pid)
	exec(`update devices set device_type_id=(select id from device_types where type_key='dji.dock2' and status='active' limit 1),adapter_id=$1 where id=$2`, cid, did)
	exec(`update device_adapters set status='connected' where id=$1`, cid)
	exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status,identity_json) values($1,$2,$3,$4,'dock','managed','{"attributes":{"serialNumber":"DOCK"}}')`, pid, team, cid, did)
	exec(`insert into device_connector_bindings(project_id,team_id,device_id,connector_instance_id,external_identity_id,priority,status) select $1,$2,$3,$4,id,100,'active' from device_external_identities where project_id=$1 and adapter_id=$4 and device_id=$3`, pid, team, did, cid)
	exec(`insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{"flight.execute":true}') on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json`, pid)
	var route int64
	if err := f.db.QueryRow(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json) values($1,$2,$3,'wayline','wayline-test','active','{"name":"测试"}') returning id`, pid, team, cid).Scan(&route); err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf("/api/projects/%d/devices/%d/flight-launch", pid, did)
	options := call("GET", endpoint, nil, 200)
	if options["canExecute"] != true || len(options["waylines"].([]any)) != 1 {
		t.Fatal(options)
	}
	var aircraft int
	if err := f.db.QueryRow(`insert into devices(project_id,name,type,device_type_id,adapter_id) values($1,'aircraft','drone',(select id from device_types where type_key='dji.matrice3td' and status='active' limit 1),$2) returning id`, pid, cid).Scan(&aircraft); err != nil {
		t.Fatal(err)
	}
	exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'aircraft','managed')`, pid, team, cid, aircraft)
	exec(`insert into device_connector_bindings(project_id,team_id,device_id,connector_instance_id,external_identity_id,priority,status) select $1,$2,$3,$4,id,100,'active' from device_external_identities where project_id=$1 and adapter_id=$4 and device_id=$3`, pid, team, aircraft, cid)
	aircraftEndpoint := fmt.Sprintf("/api/projects/%d/devices/%d/flight-launch", pid, aircraft)
	call("GET", aircraftEndpoint, nil, 409)
	exec(`insert into device_relationships(project_id,team_id,from_device_id,to_device_id,relation_type) values($1,$2,$3,$4,'contains')`, pid, team, did, aircraft)
	aircraftOptions := call("GET", aircraftEndpoint, nil, 200)
	if aircraftOptions["executionDeviceId"] != float64(did) {
		t.Fatal("aircraft did not resolve dock", aircraftOptions)
	}
	body := gin.H{"waylineResourceId": route, "name": "测试", "waylinePrecisionType": "gps", "rthAltitude": 50, "idempotencyKey": "manual-launch-test"}
	first := call("POST", endpoint, body, 202)
	repeat := call("POST", endpoint, body, 202)
	throughAircraft := call("POST", aircraftEndpoint, body, 202)
	if throughAircraft["id"] != first["id"] {
		t.Fatal("paired aircraft sent a different flight")
	}
	if first["id"] != repeat["id"] || first["runId"] != repeat["runId"] || repeat["reused"] != true {
		t.Fatal("duplicate flight", first, repeat)
	}
	receipt := call("GET", endpoint+"?idempotencyKey=manual-launch-test", nil, 200)
	if receipt["accepted"] != false {
		t.Fatal("queued was reported as accepted", receipt)
	}
	store := flighthub.NewSQLFlightActionStore(f.db)
	job, err := store.Load(context.Background(), int(pid), first["id"].(string))
	if err != nil || !job.ApprovalValid {
		t.Fatal("RBAC launch cannot dispatch", err, job.ApprovalValid)
	}
	var envelope credentials.Envelope
	json.Unmarshal(job.RequestEnvelope, &envelope)
	var request flighthub.FlightActionRequest
	if err := credentials.DecryptJSON(envelope, testFlightHubSecret, credentials.AAD("flighthub-flight-action", job.ID, pid), &request); err != nil {
		t.Fatal(err)
	}
	if !request.ManualDeviceFlight || request.WaylinePrecisionType != "gps" || request.RTHAltitude != 50 || request.TaskType != "immediate" {
		t.Fatal("wrong flight parameters", request)
	}
	changed := gin.H{"waylineResourceId": route, "name": "测试", "waylinePrecisionType": "rtk", "rthAltitude": 50, "idempotencyKey": "manual-launch-test"}
	call("POST", endpoint, changed, 409)
	body["waylineResourceId"] = route + 1000
	body["idempotencyKey"] = "foreign-wayline-test"
	call("POST", endpoint, body, 403)
	// The Copilot wrapper must not enqueue anything until a scoped human click.
	call("POST", "/api/admin/ai-providers", json.RawMessage(aiProviderBody), 201)
	stubApprovalFollowup(t, f, "飞行操作已入队，等待司空受理。")
	session := call("POST", fmt.Sprintf("/api/projects/%d/agent-sessions", pid), nil, 201)
	sid := int32(session["id"].(float64))
	var uid int32
	if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(gin.H{"deviceId": aircraft, "waylineResourceId": route, "name": "测试", "waylinePrecisionType": "gps", "rthAltitude": 50})
	proposal, err := f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "launch_flight", raw, "agent-flight-test")
	if err != nil {
		t.Fatal(err)
	}
	var beforeClick int
	if err := f.db.QueryRow("select count(*) from connector_action_jobs where project_id=$1", pid).Scan(&beforeClick); err != nil || beforeClick != 1 {
		t.Fatal("flight queued before human click", beforeClick, err)
	}
	approvalPath := fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, proposal["approvalId"])
	approved := call("POST", approvalPath, gin.H{"decision": "approve"}, 200)
	if approved["status"] != "succeeded" {
		t.Fatal(approved)
	}
	output := approved["result"].(map[string]any)["output"].(map[string]any)
	if output["runId"] == nil {
		t.Fatal("no run ID for realtime navigation", approved)
	}
	call("POST", approvalPath, gin.H{"decision": "approve"}, 409)
	agentJob, err := store.Load(context.Background(), int(pid), output["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	exec(`update team_members set role='member' where team_id=$1 and user_id=(select id from users where email='admin@example.com')`, team)
	body["waylineResourceId"] = route
	body["idempotencyKey"] = "no-permission-test"
	call("POST", endpoint, body, 403)
	var count int
	if err := f.db.QueryRow(`select count(*) from connector_action_jobs where project_id=$1`, pid).Scan(&count); err != nil || count != 2 {
		t.Fatal("unexpected queued writes", count, err)
	}
	if err := store.Fail(context.Background(), agentJob, "request_invalid"); err != nil {
		t.Fatal(err)
	}
	var runStatus, reason string
	if err := f.db.QueryRow("select status,state_reason from task_runs where project_id=$1 and id=$2", pid, agentJob.TaskRunID).Scan(&runStatus, &reason); err != nil || runStatus != "failed" || reason != "request_invalid" {
		t.Fatal("failed flight left its run dispatching", runStatus, reason, err)
	}
}
