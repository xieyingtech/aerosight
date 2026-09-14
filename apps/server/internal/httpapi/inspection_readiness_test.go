package httpapi

import (
	"fmt"
	"testing"
)

func TestInspectionReadinessIsScopedCatalogueOnly(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	otherTeam, other := f.project(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	read := func(project int, want int) map[string]any {
		t.Helper()
		res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/inspection/readiness", project), "")
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("read %d %+v", res.StatusCode, data)
		}
		return data
	}
	empty := read(pid, 200)
	if empty["availableImages"] != float64(0) || empty["detectionVersions"] != float64(0) || empty["verification"] != "catalogue-only" {
		t.Fatal("empty catalogue", empty)
	}
	exec(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image','ready','ready','available'),($1,$2,'image','pending','pending','pending'),($1,$2,'video','video','video','available')`, pid, team)
	exec(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status,deleted_at) values($1,$2,'image','deleted','deleted','available',now())`, pid, team)
	exec(`insert into assets(project_id,team_id,kind,storage_key,logical_key,status) values($1,$2,'image','other','other','available')`, other, otherTeam)
	got := read(pid, 200)
	if got["availableImages"] != float64(1) {
		t.Fatal("wrong image scope", got)
	}
	exec(`update agents set status='disabled' where project_id=$1`, pid)
	got = read(pid, 200)
	if got["copilotConfigured"] != false {
		t.Fatal("inactive copilot counted", got)
	}
	exec(`delete from team_members where team_id=$1`, team)
	read(pid, 403)
	read(other, 200)
}
