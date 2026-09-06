package httpapi

import (
	"fmt"
	"testing"
)

func TestExhaustedWriteLimitPreservesEmergencyAuthorization(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "running")
	otherRun := f.missionRun(t, pid, team, "running")
	f.server.writeRate = userRateLimiter(1, 1)
	res := f.request(t, "POST", "/api/teams", `{"name":"consume ordinary quota"}`)
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("first write %d", res.StatusCode)
	}
	var teamsBefore, teamsAfter int
	if err := f.db.QueryRow("select count(*) from teams").Scan(&teamsBefore); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", "/api/teams", `{"name":"must not exist"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" || data["error"] != "RATE_LIMITED" {
		t.Fatalf("limit %d %+v", res.StatusCode, data)
	}
	if err := f.db.QueryRow("select count(*) from teams").Scan(&teamsAfter); err != nil || teamsAfter != teamsBefore {
		t.Fatalf("limited write changed DB %d %d %v", teamsBefore, teamsAfter, err)
	}
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run)
	res = f.request(t, "POST", path, `{"action":"pause","expectedVersion":0,"reason":"ordinary control"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 429 {
		t.Fatalf("control bypassed quota %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "POST", path, `{"action":"emergency_stop","expectedVersion":0,"reason":"safety stop"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["status"] != "canceling" || data["stateVersion"] != float64(1) {
		t.Fatalf("emergency blocked %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "POST", path, `{"action":"emergency_stop","expectedVersion":0,"reason":"safety stop"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "TASK_RUN_VERSION_CONFLICT" {
		t.Fatalf("retry %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, otherRun), `{"action":"emergency_stop","expectedVersion":0,"reason":"revoked operator"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 403 || data["error"] != "PROJECT_ACCESS_DENIED" {
		t.Fatalf("authorization bypassed %d %+v", res.StatusCode, data)
	}
	var status string
	var version int
	if err := f.db.QueryRow("select status,state_version from task_runs where id=$1", otherRun).Scan(&status, &version); err != nil || status != "running" || version != 0 {
		t.Fatalf("denied mutation %s %d %v", status, version, err)
	}
	for table, want := range map[string]int{"audit_events": 1, "project_events": 1, "outbox_events": 1} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != want {
			t.Fatalf("side effects %s: %d want %d: %v", table, count, want, err)
		}
	}
}
