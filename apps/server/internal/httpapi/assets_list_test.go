package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestProjectAssetListContract(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/assets", pid)
	read := func() []map[string]any {
		t.Helper()
		res := f.request(t, "GET", path, "")
		defer res.Body.Close()
		var rows []map[string]any
		if res.StatusCode != 200 {
			t.Fatalf("status %d", res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
			t.Fatal(err)
		}
		if rows == nil {
			t.Fatal("null instead of array")
		}
		return rows
	}
	if len(read()) != 0 {
		t.Fatal("nonempty new project")
	}
	for i, status := range []string{"available", "pending", "available"} {
		if _, err := f.db.Exec(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status,created_at)
    values($1,$2,'image','private/storage',$3,$4,'2026-09-01T00:00:00Z'::timestamptz + $5 * interval '1 hour')`, pid, team, fmt.Sprint(i), status, i); err != nil {
			t.Fatal(err)
		}
	}
	rows := read()
	if len(rows) != 2 {
		t.Fatalf("available rows %+v", rows)
	}
	if len(rows[0]) != 5 || rows[0]["mimeType"] != nil || rows[0]["capturedAt"] != nil || rows[0]["createdAt"] != "2026-09-01T02:00:00.000Z" || rows[1]["createdAt"] != "2026-09-01T00:00:00.000Z" {
		t.Fatalf("DTO/order %+v", rows)
	}
	if _, ok := rows[0]["id"].(float64); !ok {
		t.Fatal("asset ID must be JSON number")
	}
	otherTeam, otherPID := f.project(t)
	if _, err := f.db.Exec(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image','other','other','available')`, otherPID, otherTeam); err != nil {
		t.Fatal(err)
	}
	if len(read()) != 2 {
		t.Fatal("cross-project data")
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "GET", path, "")
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("revoked access %d", res.StatusCode)
	}
}
