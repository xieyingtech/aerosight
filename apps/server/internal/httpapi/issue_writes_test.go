package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestIssueMutationInput(t *testing.T) {
	key := uuid.NewString()
	for _, mutation := range []string{`{"action":"comment","body":"hello"}`, `{"action":"labels","labels":[]}`, `{"action":"status","status":"closed"}`, `{"action":"assign","assigneeType":"agent","assigneeId":1}`} {
		var input map[string]any
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"expectedVersion":0,"clientKey":%q,"mutation":%s}`, key, mutation)), &input); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := decodeIssueMutation(input); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutation := range []string{`{"action":"comment"}`, `{"action":"comment","body":null}`, `{"action":"comment","body":"x","status":"open"}`, `{"action":"labels","labels":null}`, `{"action":"labels","labels":[1]}`, `{"action":"status","status":"unknown"}`, `{"action":"assign","assigneeType":"agent","assigneeId":1.5}`} {
		var input map[string]any
		json.Unmarshal([]byte(fmt.Sprintf(`{"expectedVersion":0,"clientKey":%q,"mutation":%s}`, key, mutation)), &input)
		if _, _, _, err := decodeIssueMutation(input); err == nil {
			t.Fatalf("accepted %s", mutation)
		}
	}
}

func TestIssueMutationTransactions(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var iid, uid, agent int
	if err := f.db.QueryRow("insert into issues(project_id,number,title,source_type) values($1,1,'Case','manual') returning id", pid).Scan(&iid); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("select id from agents where project_id=$1 and config_json->>'kind'='copilot'", pid).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/issues/%d/actions", pid, iid)
	call := func(version int, key, mutation string, status int) map[string]any {
		t.Helper()
		body := fmt.Sprintf(`{"expectedVersion":%d,"clientKey":%q,"mutation":%s}`, version, key, mutation)
		res := f.request(t, "POST", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d: %+v", res.StatusCode, status, data)
		}
		return data
	}
	count := func(query string, want int) {
		t.Helper()
		var n int
		if err := f.db.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("%s = %d want %d", query, n, want)
		}
	}
	key := uuid.NewString()
	data := call(0, key, `{"action":"comment","body":" @copilot 请分析 "}`, 200)
	if data["stateVersion"] != float64(1) || data["copilotJobId"] == nil || data["replayed"] != false {
		t.Fatalf("comment %+v", data)
	}
	count("select count(*) from agent_tool_jobs where status='queued' and tool_name='issue_copilot' and required_permission='agent:use'", 1)
	count("select count(*) from issue_events", 2)
	count("select count(*) from project_events where event_type='issue.updated'", 1)
	count("select count(*) from outbox_events where event_type='issue.updated'", 0)
	data = call(0, key, `{"action":"comment","body":"different retry"}`, 200)
	if data["replayed"] != true || data["stateVersion"] != float64(1) {
		t.Fatalf("replay %+v", data)
	}
	count("select count(*) from agent_tool_jobs", 1)
	call(0, uuid.NewString(), `{"action":"comment","body":"stale"}`, 409)
	call(1, uuid.NewString(), `{"action":"labels","labels":[" a ","a","b",""]}`, 200)
	count(`select count(*) from issues where labels_json='["a","b"]'::jsonb and state_version=2`, 1)
	call(2, uuid.NewString(), `{"action":"status","status":"closed"}`, 200)
	count("select count(*) from issues where status='closed' and closed_at is not null", 1)
	call(3, uuid.NewString(), `{"action":"status","status":"open"}`, 200)
	count("select count(*) from issues where status='open' and closed_at is null", 1)
	assign := fmt.Sprintf(`{"action":"assign","assigneeType":"agent","assigneeId":%d}`, agent)
	call(4, uuid.NewString(), assign, 200)
	data = call(5, uuid.NewString(), assign, 200)
	if data["noOp"] != true || data["stateVersion"] != float64(5) {
		t.Fatalf("noop %+v", data)
	}
	count("select count(*) from agent_tool_jobs", 2)
	call(5, uuid.NewString(), fmt.Sprintf(`{"action":"unassign","assigneeType":"agent","assigneeId":%d}`, agent), 200)
	count("select count(*) from issue_assignees where active", 0)
	call(6, uuid.NewString(), fmt.Sprintf(`{"action":"assign","assigneeType":"user","assigneeId":%d}`, uid), 200)
	call(7, uuid.NewString(), `{"action":"assign","assigneeType":"user","assigneeId":2147483647}`, 400)
	// A downstream event failure must undo the version, activity, session, job
	// and accepted audit, not leave a queued agent behind a failed HTTP request.
	if _, err := f.db.Exec(`create function fail_issue_event() returns trigger language plpgsql as $$ begin raise exception 'private failure'; end $$; create trigger fail_issue_event before insert on project_events for each row execute function fail_issue_event()`); err != nil {
		t.Fatal(err)
	}
	data = call(7, uuid.NewString(), `{"action":"comment","body":"@copilot again"}`, 400)
	if data["error"] != "ISSUE_MUTATION_FAILED" {
		t.Fatalf("error leak %+v", data)
	}
	count("select count(*) from issues where state_version=7", 1)
	count("select count(*) from agent_tool_jobs", 2)
	count("select count(*) from agent_sessions", 2)
	count("select count(*) from audit_events where status<>'completed'", 0)
	if _, err := f.db.Exec("drop trigger fail_issue_event on project_events"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) values($1,$2,$3,'event:handle'),($1,$2,$3,'issue:assign')", pid, team, uid); err != nil {
		t.Fatal(err)
	}
	call(7, uuid.NewString(), assign, 403)
	call(7, uuid.NewString(), `{"action":"comment","body":"@copilot no permission"}`, 200)
	count("select count(*) from agent_tool_jobs", 2)
	if _, err := f.db.Exec("delete from project_permissions where project_id=$1", pid); err != nil {
		t.Fatal(err)
	}
	call(0, key, `{"action":"comment","body":"retry after revoke"}`, 403)
	_, other := f.project(t)
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/issues/%d/actions", other, iid), fmt.Sprintf(`{"expectedVersion":8,"clientKey":%q,"mutation":{"action":"comment","body":"cross project"}}`, uuid.NewString()))
	if data := decodedResponse(t, res); res.StatusCode != 404 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
}

func TestIssueMutationConcurrentReplay(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	var iid int
	if err := f.db.QueryRow("insert into issues(project_id,number,title,source_type) values($1,1,'Case','manual') returning id", pid).Scan(&iid); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/issues/%d/actions", pid, iid)
	body := fmt.Sprintf(`{"expectedVersion":0,"clientKey":%q,"mutation":{"action":"comment","body":"@copilot"}}`, uuid.NewString())
	results := make(chan error, 4)
	for range 4 {
		go func() {
			req, err := http.NewRequest("POST", f.host.URL+path, strings.NewReader(body))
			if err != nil {
				results <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			res, err := f.client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				results <- fmt.Errorf("status %d", res.StatusCode)
				return
			}
			results <- nil
		}()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"select count(*) from agent_tool_jobs", "select count(*) from issues where state_version=1", "select count(*) from issue_events where event_type='comment.created'", "select count(*) from project_events where event_type='issue.updated'"} {
		var n int
		if err := f.db.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("%s = %d", query, n)
		}
	}
}
