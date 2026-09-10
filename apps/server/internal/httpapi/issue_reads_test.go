package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestIssueReadContractAndPermissions(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var issue, asset, agent, session int
	if err := f.db.QueryRow("insert into issues(project_id,number,title,source_type) values($1,1,'案件','manual') returning id", pid).Scan(&issue); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/issues/%d", pid, issue)
	res := f.request(t, "GET", path, "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("empty %d %+v", res.StatusCode, data)
	}
	for _, key := range []string{"events", "links", "detections", "assets", "assignees", "drafts"} {
		if len(data[key].([]any)) != 0 {
			t.Fatalf("empty %s %+v", key, data[key])
		}
	}
	if len(data["members"].([]any)) != 1 || data["canHandle"] != true || data["canAssign"] != true || data["canUseAgent"] != true {
		t.Fatalf("scope permissions %+v", data)
	}
	if _, err := f.db.Exec("insert into issue_events(project_id,issue_id,event_type,body) values($1,$2,'comment','系统记录')", pid, issue); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key) values($1,$2,'image','case.jpg','case.jpg') returning id", pid, team).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into issue_links(project_id,issue_id,link_type,target_id) values($1,$2,'asset',$3),($1,$2,'detection','not-a-number')", pid, issue, fmt.Sprint(asset)); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`select id from agents where project_id=$1 and config_json->>'kind'='copilot'`, pid).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into issue_assignees(project_id,team_id,issue_id,assignee_type,agent_id,assigned_by_user_id) select $1,$2,$3,'agent',$4,id from users where email='admin@example.com'", pid, team, issue, agent); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into agent_sessions(project_id,agent_id,issue_id) values($1,$2,$3) returning id", pid, agent, issue).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into agent_drafts(project_id,team_id,session_id,created_by_user_id,draft_type,title) select $1,$2,$3,id,'report','Draft' from users where email='admin@example.com'", pid, team, session); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["assets"].([]any)) != 1 || len(data["drafts"].([]any)) != 1 || data["events"].([]any)[0].(map[string]any)["actorName"] != "系统" {
		t.Fatalf("linked %d %+v", res.StatusCode, data)
	}
	if _, ok := data["assignees"].([]any)[0].(map[string]any)["id"].(string); !ok {
		t.Fatal("assignee bigint must be string")
	}
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/issues", pid), "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(rows) != 1 || rows[0]["hasMapLocation"] != false || rows[0]["id"] != float64(issue) {
		t.Fatalf("list %+v", rows)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'event:handle' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if data["canHandle"] != true || data["canAssign"] != false || data["canUseAgent"] != false {
		t.Fatalf("permission alias %+v", data)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/issues/%d", other, issue), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
}
