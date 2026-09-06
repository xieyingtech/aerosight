package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeviceCommandSafetyIdempotencyAndAudit(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	for _, cap := range []string{"dock.debug.control", "flight.return_home"} {
		if _, err := f.db.Exec("insert into device_capabilities(project_id,device_id,capability_code,risk_level) values($1,$2,$3,'high')", pid, did, cap); err != nil {
			t.Fatal(err)
		}
	}
	path := fmt.Sprintf("/api/projects/%d/devices/%d/commands", pid, did)
	input := map[string]any{"capabilityCode": "dock.debug.control", "commandKey": "cover.open", "parameters": map[string]any{}, "idempotencyKey": "command-test-1", "reason": "test command", "deadlineSeconds": 600}
	call := func() (int, map[string]any) {
		raw, _ := json.Marshal(input)
		res := f.request(t, "POST", path, string(raw))
		return res.StatusCode, decodedResponse(t, res)
	}
	status, data := call()
	if status != 409 || data["error"] != "DEVICE_COMMAND_CONFIRMATION_REQUIRED" {
		t.Fatalf("confirmation %d %+v", status, data)
	}
	input["confirmation"] = fmt.Sprintf("CONFIRM %d dock.debug.control", did)
	status, data = call()
	if status != 200 || data["status"] != "dispatchable" || data["reused"] != false {
		t.Fatalf("submit %d %+v", status, data)
	}
	commandID := data["id"].(string)
	var priority int
	var deadline float64
	var safety map[string]any
	var raw []byte
	if err := f.db.QueryRow("select priority,extract(epoch from deadline_at-created_at),safety_context_json from device_commands where id=$1", commandID).Scan(&priority, &deadline, &raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &safety); err != nil {
		t.Fatal(err)
	}
	if priority != 80 || deadline != 300 || safety["confirmationRequired"] != true {
		t.Fatalf("safety %d %f %+v", priority, deadline, safety)
	}
	// The existing contract returns the original accepted command, even when the retry's body changes.
	input["parameters"] = map[string]any{"changed": true}
	input["confirmation"] = nil
	status, data = call()
	if status != 200 || data["id"] != commandID || data["reused"] != true {
		t.Fatalf("retry %d %+v", status, data)
	}
	for _, table := range []string{"device_commands", "project_events", "outbox_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("duplicate %s %d %v", table, count, err)
		}
	}
	// A deny must take effect before an old idempotent response can be returned.
	if _, err := f.db.Exec("insert into device_capability_grants(project_id,team_id,user_id,scope_type,action_pattern,effect) select $1,$2,id,'project','dock.*','deny' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	status, data = call()
	if status != 403 || data["error"] != "DEVICE_CAPABILITY_EXPLICITLY_DENIED" {
		t.Fatalf("deny retry %d %+v", status, data)
	}
	if _, err := f.db.Exec("delete from device_capability_grants where project_id=$1", pid); err != nil {
		t.Fatal(err)
	}
	input["idempotencyKey"] = "command-test-2"
	input["confirmation"] = fmt.Sprintf("CONFIRM %d dock.debug.control", did)
	if _, err := f.db.Exec("update devices set status='offline' where id=$1", did); err != nil {
		t.Fatal(err)
	}
	status, data = call()
	if status != 409 || data["error"] != "DEVICE_COMMAND_DEVICE_NOT_ONLINE" {
		t.Fatalf("offline %d %+v", status, data)
	}
	if _, err := f.db.Exec("update devices set status='online' where id=$1", did); err != nil {
		t.Fatal(err)
	}
	var task int
	if err := f.db.QueryRow("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'conflicting task','manual','') returning id", pid, team).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into task_runs(project_id,team_id,task_id,selected_device_id,trigger_source,status) values($1,$2,$3,$4,'manual','running')", pid, team, task, did); err != nil {
		t.Fatal(err)
	}
	status, data = call()
	if status != 409 || data["error"] != "DEVICE_COMMAND_ACTIVE_TASK_CONFLICT" {
		t.Fatalf("task conflict %d %+v", status, data)
	}
	f.server.writeRate = userRateLimiter(1, 1)
	quota := f.request(t, "POST", "/api/teams", `{"name":"consume command quota"}`)
	quota.Body.Close()
	if quota.StatusCode != 201 {
		t.Fatalf("consume quota %d", quota.StatusCode)
	}
	status, data = call()
	if status != 429 {
		t.Fatalf("ordinary command bypassed quota %d %+v", status, data)
	}
	input["capabilityCode"] = "flight.return_home"
	input["commandKey"] = "return_home"
	input["confirmation"] = fmt.Sprintf("CONFIRM %d flight.return_home", did)
	status, data = call()
	if status != 200 {
		t.Fatalf("return home exception %d %+v", status, data)
	}
	if err := f.db.QueryRow("select priority,safety_context_json from device_commands where id=$1", data["id"]).Scan(&priority, &raw); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &safety)
	if priority != 100 || safety["activeTaskOverride"] != true {
		t.Fatalf("return home safety %d %+v", priority, safety)
	}
	var audits int
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and status='completed'", pid).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("audit outcomes %d %v", audits, err)
	}
}

func TestConcurrentDeviceCommandOnlyQueuesOnce(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	if _, err := f.db.Exec("insert into device_capabilities(project_id,device_id,capability_code) values($1,$2,'sensor.configure')", pid, did); err != nil {
		t.Fatal(err)
	}
	body := `{"capabilityCode":"sensor.configure","commandKey":"configure","parameters":{},"idempotencyKey":"concurrent-test","reason":"test"}`
	type outcome struct {
		status int
		body   []byte
		err    error
	}
	results := make(chan outcome, 4)
	for n := 0; n < 4; n++ {
		go func() {
			req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/projects/%d/devices/%d/commands", f.host.URL, pid, did), strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			res, err := f.client.Do(req)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			raw, err := io.ReadAll(res.Body)
			res.Body.Close()
			results <- outcome{res.StatusCode, raw, err}
		}()
	}
	id := ""
	created := 0
	for n := 0; n < 4; n++ {
		out := <-results
		if out.err != nil || out.status != 200 {
			t.Fatalf("concurrent %d %s %v", out.status, out.body, out.err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.body, &result); err != nil {
			t.Fatal(err)
		}
		if id == "" {
			id = result["id"].(string)
		}
		if result["id"] != id {
			t.Fatal("same key created multiple commands")
		}
		if result["reused"] == false {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d times", created)
	}
	var events int
	if err := f.db.QueryRow("select count(*) from outbox_events where project_id=$1 and event_type='device.command.dispatch'", pid).Scan(&events); err != nil || events != 1 {
		t.Fatalf("duplicate dispatch %d %v", events, err)
	}
}

func TestDeviceCommandScopeReplayAndFailureRollback(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	if _, err := f.db.Exec("insert into device_capabilities(project_id,device_id,capability_code) values($1,$2,'sensor.configure')", pid, did); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/devices/%d/commands", pid, did)
	body := `{"capabilityCode":"sensor.configure","commandKey":"configure","parameters":{},"idempotencyKey":"rollback-test","reason":"test"}`
	res := f.request(t, "POST", path+"?mode=replay", body)
	data := decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "device control is forbidden in replay mode" {
		t.Fatalf("replay %d %+v", res.StatusCode, data)
	}
	_, other := f.project(t)
	res = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/devices/%d/commands", other, did), body)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "DEVICE_CAPABILITY_NOT_FOUND" {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
	for _, invalid := range []string{`{}`, strings.Replace(body, `"parameters":{}`, `"parameters":[]`, 1), strings.Replace(body, `"reason":"test"`, `"reason":"test","deadlineSeconds":null`, 1)} {
		res = f.request(t, "POST", path, invalid)
		data = decodedResponse(t, res)
		if res.StatusCode != 400 {
			t.Fatalf("input %d %+v", res.StatusCode, data)
		}
	}
	if _, err := f.db.Exec("create function reject_command_outbox() returns trigger language plpgsql as $$ begin raise exception 'injected database detail'; end $$; create trigger reject_command_outbox before insert on outbox_events for each row execute function reject_command_outbox()"); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, body)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "DEVICE_COMMAND_FAILED" {
		t.Fatalf("failure sanitized %d %+v", res.StatusCode, data)
	}
	for _, table := range []string{"audit_events", "device_commands", "project_events", "outbox_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s %d %v", table, count, err)
		}
	}
	if _, err := f.db.Exec("drop trigger reject_command_outbox on outbox_events; drop function reject_command_outbox()"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, body)
	data = decodedResponse(t, res)
	if res.StatusCode != http.StatusForbidden || data["error"] != "DEVICE_CAPABILITY_NOT_GRANTED" {
		t.Fatalf("member %d %+v", res.StatusCode, data)
	}
}
