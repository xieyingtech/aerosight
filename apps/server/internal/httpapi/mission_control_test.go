package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (f *apiFixture) missionRun(t *testing.T, pid, team int, status string) int {
	t.Helper()
	var task, run int
	if err := f.db.QueryRow("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'control test '||gen_random_uuid(),'manual','') returning id", pid, team).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into task_runs(project_id,team_id,task_id,trigger_source,status) values($1,$2,$3,'manual',$4) returning id", pid, team, task, status).Scan(&run); err != nil {
		t.Fatal(err)
	}
	return run
}
func TestMissionControlTransitionsAndQueue(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "running")
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run)
	call := func(action string, version int) (int, map[string]any) {
		res := f.request(t, "POST", path, fmt.Sprintf(`{"action":%q,"expectedVersion":%d,"reason":"operator reason"}`, action, version))
		return res.StatusCode, decodedResponse(t, res)
	}
	for i, step := range []struct{ action, status string }{{"pause", "paused"}, {"resume", "running"}, {"cancel", "canceling"}, {"emergency_stop", "canceling"}} {
		status, data := call(step.action, i)
		if status != 200 || data["status"] != step.status || data["stateVersion"] != float64(i+1) {
			t.Fatalf("%s %d %+v", step.action, status, data)
		}
	}
	status, data := call("emergency_stop", 3)
	if status != 409 || data["error"] != "TASK_RUN_VERSION_CONFLICT" {
		t.Fatalf("retry %d %+v", status, data)
	}
	status, data = call("pause", 4)
	if status != 409 || data["error"] != "TASK_RUN_TRANSITION_INVALID:canceling:paused" {
		t.Fatalf("transition %d %+v", status, data)
	}
	var events, controls, audits int
	if err := f.db.QueryRow("select count(*),count(*) filter(where event_type='mission.control') from outbox_events where project_id=$1", pid).Scan(&events, &controls); err != nil || events != 4 || controls != 2 {
		t.Fatalf("queue %d %d %v", events, controls, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and status='completed'", pid).Scan(&audits); err != nil || audits != 4 {
		t.Fatalf("audit %d %v", audits, err)
	}
	queued := f.missionRun(t, pid, team, "queued")
	path = fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, queued)
	status, data = call("cancel", 0)
	if status != 200 || data["status"] != "canceled" {
		t.Fatalf("queued cancel %d %+v", status, data)
	}
	_, other := f.project(t)
	path = fmt.Sprintf("/api/projects/%d/task-runs/%d/control", other, run)
	status, data = call("pause", 4)
	if status != 409 || data["error"] != "TASK_RUN_NOT_FOUND" {
		t.Fatalf("scope %d %+v", status, data)
	}
}

func TestMissionControlConcurrencyPermissionAndRollback(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "running")
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run)
	type result struct {
		status int
		err    error
	}
	results := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			req, _ := http.NewRequest("POST", f.host.URL+path, strings.NewReader(`{"action":"pause","expectedVersion":0,"reason":"concurrent pause"}`))
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			res, err := f.client.Do(req)
			if err != nil {
				results <- result{err: err}
				return
			}
			res.Body.Close()
			results <- result{status: res.StatusCode}
		}()
	}
	success, conflict := 0, 0
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		switch r.status {
		case 200:
			success++
		case 409:
			conflict++
		default:
			t.Fatalf("status %d", r.status)
		}
	}
	if success != 1 || conflict != 3 {
		t.Fatalf("concurrent %d %d", success, conflict)
	}
	if _, err := f.db.Exec(`create function reject_control_event() returns trigger language plpgsql as $$ begin raise exception 'private database detail'; end $$;create trigger reject_control_event before insert on outbox_events for each row execute function reject_control_event()`); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "POST", path, `{"action":"resume","expectedVersion":1,"reason":"rollback"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "MISSION_CONTROL_FAILED" {
		t.Fatalf("rollback %d %+v", res.StatusCode, data)
	}
	var state string
	var version int
	if err := f.db.QueryRow("select status,state_version from task_runs where id=$1", run).Scan(&state, &version); err != nil || state != "paused" || version != 1 {
		t.Fatalf("state rollback %s %d %v", state, version, err)
	}
	for _, table := range []string{"audit_events", "project_events", "outbox_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("rollback %s %d %v", table, count, err)
		}
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, `{"action":"emergency_stop","expectedVersion":1,"reason":"unauthorized"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 403 || data["error"] != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("permission %d %+v", res.StatusCode, data)
	}
	for _, body := range []string{`{"action":"other","expectedVersion":1,"reason":"x"}`, `{"action":"pause","expectedVersion":1.5,"reason":"x"}`, `{"action":"pause","expectedVersion":1,"reason":" "}`} {
		res = f.request(t, "POST", path, body)
		data = decodedResponse(t, res)
		if res.StatusCode != 400 {
			t.Fatalf("input %d %+v", res.StatusCode, data)
		}
	}
}

func TestMissionControlApproval(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "blocked")
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run)
	body := `{"action":"approve","expectedVersion":0,"reason":"reviewed"}`
	res := f.request(t, "POST", path, body)
	data := decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "TASK_RUN_APPROVAL_NOT_REQUIRED" {
		t.Fatalf("missing %d %+v", res.StatusCode, data)
	}
	var approval string
	if err := f.db.QueryRow("insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,required_approvals,require_separation,expires_at) select gen_random_uuid(),$1,$2,'task_run',$3,'mission.start',id,2,false,now()+interval '1 hour' from users where email='admin@example.com' returning id", pid, team, fmt.Sprint(run)).Scan(&approval); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update task_runs set approval_request_id=$2 where id=$1", run, approval); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"require_separation=true", "require_separation=false,expires_at=now()-interval '1 minute'"} {
		if _, err := f.db.Exec("update approval_requests set "+policy+" where id=$1", approval); err != nil {
			t.Fatal(err)
		}
		res = f.request(t, "POST", path, body)
		data = decodedResponse(t, res)
		if res.StatusCode != 409 || data["error"] != "MISSION_CONTROL_FAILED" {
			t.Fatalf("approval policy %d %+v", res.StatusCode, data)
		}
	}
	if _, err := f.db.Exec("update approval_requests set require_separation=false,expires_at=now()+interval '1 hour' where id=$1", approval); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, body)
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["approval"] != "approved" || data["status"] != "blocked" || data["stateVersion"] != float64(0) {
		t.Fatalf("approve %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "POST", path, body)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 {
		t.Fatalf("duplicate approval %d %+v", res.StatusCode, data)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from approvals where approval_request_id=$1", approval).Scan(&count); err != nil || count != 1 {
		t.Fatalf("approval rows %d %v", count, err)
	}
	if err := f.db.QueryRow("select count(*) from outbox_events where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("approval side effects %d %v", count, err)
	}
	var auditInput map[string]any
	var raw []byte
	if err := f.db.QueryRow("select policy_result_json from audit_events where project_id=$1", pid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &auditInput); err != nil || auditInput["permission"] != "mission:approve" {
		t.Fatalf("approval audit %+v %v", auditInput, err)
	}
}
