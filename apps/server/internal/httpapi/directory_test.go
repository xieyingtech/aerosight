package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDirectoryCreationAndViews(t *testing.T) {
	f := newAPIFixture(t)
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		result := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%s %s %d %+v", method, path, res.StatusCode, result)
		}
		return result
	}
	list := func(path string) []map[string]any {
		t.Helper()
		res := f.request(t, "GET", path, "")
		defer res.Body.Close()
		var rows []map[string]any
		if err := json.NewDecoder(res.Body).Decode(&rows); err != nil || res.StatusCode != 200 {
			t.Fatalf("list %s %d %v", path, res.StatusCode, err)
		}
		return rows
	}
	for _, path := range []string{"/api/teams", "/api/projects", "/api/admin/teams", "/api/admin/projects"} {
		if rows := list(path); rows == nil || len(rows) != 0 {
			t.Fatalf("empty %s %+v", path, rows)
		}
	}
	for _, name := range []string{" ", strings.Repeat("😀", 51), strings.Repeat("x", 101)} {
		body, _ := json.Marshal(map[string]any{"name": name})
		call("POST", "/api/teams", string(body), 400)
	}
	team := call("POST", "/api/teams", `{"name":"  Team  "}`, 201)["id"].(float64)
	rows := list("/api/teams?scope=managed&search=Team")
	if len(rows) != 1 || rows[0]["name"] != "Team" || rows[0]["role"] != "owner" || rows[0]["memberCount"] != float64(1) {
		t.Fatalf("team %+v", rows)
	}
	project := call("POST", "/api/projects", fmt.Sprintf(`{"teamId":"%d","name":"  Project  "}`, int(team)), 201)["id"].(float64)
	detail := call("GET", fmt.Sprintf("/api/projects/%d", int(project)), "", 200)
	if detail["name"] != "Project" {
		t.Fatalf("project %+v", detail)
	}
	for _, body := range []string{`{"teamId":0,"name":"x"}`, `{"teamId":1.5,"name":"x"}`, `{"teamId":"bad","name":"x"}`, `{"teamId":true,"name":"x"}`} {
		call("POST", "/api/projects", body, 400)
	}
	body, _ := json.Marshal(map[string]any{"teamId": team, "name": strings.Repeat("😀", 51)})
	call("POST", "/api/projects", string(body), 400)
	profile := call("GET", "/api/profile", "", 200)
	if len(profile["profile"].([]any)) != 1 || len(profile["teams"].([]any)) != 1 {
		t.Fatalf("profile %+v", profile)
	}
	overview := call("GET", "/api/admin/overview", "", 200)
	if overview["users"] != float64(1) || overview["teams"] != float64(1) || overview["projects"] != float64(1) {
		t.Fatalf("overview %+v", overview)
	}
	for _, path := range []string{"/api/admin/users", "/api/admin/teams", "/api/admin/projects"} {
		rows := list(path)
		if len(rows) != 1 {
			t.Fatalf("admin %s %+v", path, rows)
		}
		raw, _ := json.Marshal(rows)
		if strings.Contains(string(raw), "password") {
			t.Fatal("password field exposed")
		}
	}
	var agents int
	if err := f.db.QueryRow("select count(*) from agents where project_id=$1 and config_json->>'kind'='copilot'", int(project)).Scan(&agents); err != nil || agents != 1 {
		t.Fatalf("bootstrap agent %d %v", agents, err)
	}
	// Team and owner creation are one transaction, even if the owner insert fails.
	if _, err := f.db.Exec(`create function fail_team_owner() returns trigger language plpgsql as $$ begin raise exception 'private insert failure'; end $$; create trigger fail_team_owner before insert on team_members for each row execute function fail_team_owner()`); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/teams", `{"name":"Rollback"}`, 500)
	if len(list("/api/admin/teams")) != 1 {
		t.Fatal("orphan team survived failed owner creation")
	}
	if _, err := f.db.Exec("drop trigger fail_team_owner on team_members"); err != nil {
		t.Fatal(err)
	}
	// Platform admin does not override the team's project-creation permission.
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", int(team)); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/projects", fmt.Sprintf(`{"teamId":%d,"name":"Denied"}`, int(team)), 403)
	if len(list("/api/teams?scope=managed")) != 0 || len(list("/api/projects?scope=joined")) != 1 {
		t.Fatal("role scopes")
	}
	if _, err := f.db.Exec("update team_members set role='admin' where team_id=$1", int(team)); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/projects", fmt.Sprintf(`{"teamId":%d,"name":"Admin project"}`, int(team)), 201)
	if _, err := f.db.Exec("delete from team_members where team_id=$1", int(team)); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/projects", fmt.Sprintf(`{"teamId":%d,"name":"Gone"}`, int(team)), 403)
	call("GET", fmt.Sprintf("/api/teams/%d", int(team)), "", 404)
	call("GET", fmt.Sprintf("/api/projects/%d", int(project)), "", 404)
	if len(list("/api/projects")) != 0 {
		t.Fatal("revoked project remained visible")
	}
	if _, err := f.db.Exec("update users set role='user' where email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/admin/overview", "/api/admin/users", "/api/admin/teams", "/api/admin/projects"} {
		call("GET", path, "", 403)
	}
}
