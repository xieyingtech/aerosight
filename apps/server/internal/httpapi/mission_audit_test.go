package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestEmergencyDrillNeverDispatches(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	run := f.missionRun(t, pid, team, "running")
	if _, err := f.db.Exec("update task_runs set selected_device_id=$2 where id=$1", run, did); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_capabilities(device_id,project_id,capability_code) values($1,$2,'flight.return_home')", did, pid); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/emergency-stop-drill", pid, run)
	for _, step := range []struct{ outcome, state string }{{"ack", "confirmed"}, {"nack", "rejected"}, {"timeout", "unknown"}, {"disconnected", "unknown"}} {
		res := f.request(t, "POST", path, fmt.Sprintf(`{"dryRun":true,"outcome":%q}`, step.outcome))
		data := decodedResponse(t, res)
		if res.StatusCode != 200 || data["safetyState"] != step.state || data["complete"] != true {
			t.Fatalf("%s %d %+v", step.outcome, res.StatusCode, data)
		}
		command := data["stages"].(map[string]any)["commands"].([]any)[0].(map[string]any)
		if command["priority"] != float64(100) || !strings.HasPrefix(command["id"].(string), fmt.Sprintf("drill:%d:", run)) {
			t.Fatalf("command %+v", command)
		}
	}
	if _, err := f.db.Exec("update devices set status='offline' where id=$1", did); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "POST", path, `{"dryRun":true,"outcome":"ack"}`)
	data := decodedResponse(t, res)
	command := data["stages"].(map[string]any)["commands"].([]any)[0].(map[string]any)
	if data["safetyState"] != "unknown" || command["errorCode"] != "DEVICE_DISCONNECTED" || command["attempt"] != nil {
		t.Fatalf("offline %+v", data)
	}
	for _, body := range []string{`{"dryRun":false,"outcome":"ack"}`, `{"dryRun":true,"outcome":"bad"}`} {
		res = f.request(t, "POST", path, body)
		data = decodedResponse(t, res)
		if res.StatusCode != 400 {
			t.Fatalf("invalid %d %+v", res.StatusCode, data)
		}
	}
	for _, table := range []string{"device_commands", "project_events", "outbox_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("drill side effect %s %d %v", table, count, err)
		}
	}
	var status string
	var version, audits int
	if err := f.db.QueryRow("select status,state_version from task_runs where id=$1", run).Scan(&status, &version); err != nil || status != "running" || version != 0 {
		t.Fatalf("run side effect %s %d %v", status, version, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and status='completed' and policy_result_json->>'effect'='none'", pid).Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("audit %d %v", audits, err)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, `{"dryRun":true,"outcome":"ack"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("permission %d %+v", res.StatusCode, data)
	}
}

func TestMissionAuditTraceCorrelatesLatestAttempt(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	run := f.missionRun(t, pid, team, "running")
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/audit-trace", pid, run)
	res := f.request(t, "GET", path, "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || data["complete"] != false || data["safetyState"] != "not_requested" || len(data["missing"].([]any)) != 4 {
		t.Fatalf("empty %d %+v", res.StatusCode, data)
	}
	var policy int64
	if err := f.db.QueryRow("insert into safety_policy_versions(project_id,team_id,version,status,max_altitude_meters,max_speed_meters_per_second,minimum_battery_percent) values($1,$2,1,'published',100,10,20) returning id", pid, team).Scan(&policy); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`update task_runs set safety_policy_version_id=$2,preflight_snapshot_json='{"allowed":true,"checks":[{"code":"ok"}]}',trigger_source='agent' where id=$1`, run, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into audit_events(project_id,team_id,request_id,actor_user_id,action,resource_type,resource_id,input_hash) select $1,$2,'correlated-request',id,'task_run.transition','task_run',$3,'hash' from users where email='admin@example.com'", pid, team, fmt.Sprint(run)); err != nil {
		t.Fatal(err)
	}
	var command string
	if err := f.db.QueryRow("insert into device_commands(id,project_id,team_id,device_id,task_run_id,command_key,idempotency_key,capability_code,status,priority,deadline_at) values(gen_random_uuid(),$1,$2,$3,$4,'flight.return_home','audit-test','flight.return_home','sent',100,now()+interval '1 hour') returning id", pid, team, did, run).Scan(&command); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into command_attempts(project_id,team_id,command_id,adapter_id,attempt,status) select $1,$2,$3,adapter_id,n,case when n=1 then 'nacked' else 'acknowledged' end from devices cross join generate_series(1,2) n where devices.id=$4", pid, team, command, did); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["complete"] != true || data["safetyState"] != "confirmed" {
		t.Fatalf("trace %d %+v", res.StatusCode, data)
	}
	stages := data["stages"].(map[string]any)
	if stages["request"].(map[string]any)["actorType"] != "agent" || stages["preflight"].(map[string]any)["policyVersionId"] != fmt.Sprint(policy) {
		t.Fatalf("types %+v", stages)
	}
	latest := stages["commands"].([]any)[0].(map[string]any)
	if latest["attempt"] != float64(2) || latest["attemptStatus"] != "acknowledged" || latest["errorCode"] != nil {
		t.Fatalf("latest %+v", latest)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/task-runs/%d/audit-trace", other, run), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("permission %d %+v", res.StatusCode, data)
	}
}
