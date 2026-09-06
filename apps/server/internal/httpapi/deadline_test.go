package httpapi

import (
	"fmt"
	"testing"
	"time"
)

func TestRequestDeadlineCancelsSQLAndRollsBack(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "running")
	if _, err := f.db.Exec(`create function delay_test_outbox() returns trigger language plpgsql as $$ begin perform pg_sleep(5); return new; end $$; create trigger delay_test_outbox before insert on outbox_events for each row execute function delay_test_outbox()`); err != nil {
		t.Fatal(err)
	}
	f.server.cfg.RequestTimeout = 100 * time.Millisecond
	start := time.Now()
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), `{"action":"pause","expectedVersion":0,"reason":"deadline rollback"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 504 || data["error"] != "REQUEST_TIMEOUT" {
		t.Fatalf("deadline %d %+v", res.StatusCode, data)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("query not canceled: %s", elapsed)
	}
	var state string
	var version int
	if err := f.db.QueryRow("select status,state_version from task_runs where id=$1", run).Scan(&state, &version); err != nil || state != "running" || version != 0 {
		t.Fatalf("partial write %s %d %v", state, version, err)
	}
	for _, table := range []string{"audit_events", "outbox_events", "project_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s %d %v", table, count, err)
		}
	}
}
