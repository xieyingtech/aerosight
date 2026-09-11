package httpapi

import (
	"fmt"
	"testing"
)

func TestProjectFeatureTreeCatalog(t *testing.T) {
	seen := map[string]bool{}
	var visit func([]featureNode)
	visit = func(nodes []featureNode) {
		for _, n := range nodes {
			if seen[n.ID] {
				t.Fatalf("duplicate tree node %s", n.ID)
			}
			seen[n.ID] = true
			visit(n.Children)
		}
	}
	visit(projectFeatureTree)
	leaves := featureLeafIDs(projectFeatureTree)
	for _, policies := range []map[string]fhActionPolicy{fhActionPolicies, fhAdminPolicies} {
		for _, p := range policies {
			if !leaves[p.flag] {
				t.Fatalf("missing flag %s", p.flag)
			}
		}
	}
	for _, p := range fhDiscretePolicies {
		if !leaves[p.flag] {
			t.Fatalf("missing device flag %s", p.flag)
		}
	}
	for _, id := range []string{"live.control", "flight.execute", "device.control"} {
		if !leaves[id] {
			t.Fatal(id)
		}
	}
	values, err := featureValues([]byte(`{"live.control":"true","flight.execute":true,"unknown":true}`))
	if err != nil || values["live.control"] || !values["flight.execute"] || values["unknown"] {
		t.Fatalf("invalid read: %v %v", values, err)
	}
}

func TestProjectFeatureSettings(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/feature-settings", pid)
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		r := f.request(t, method, path, body)
		d := decodedResponse(t, r)
		if r.StatusCode != status {
			t.Fatalf("%s %s: status %d expected %d: %v", method, path, r.StatusCode, status, d)
		}
		return d
	}
	values := call("GET", path, "", 200)["values"].(map[string]any)
	if len(values) != len(featureLeafIDs(projectFeatureTree)) {
		t.Fatal("missing defaults")
	}
	for k, v := range values {
		if v != false {
			t.Fatalf("default enabled %s", k)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`insert into project_feature_flags(project_id,flighthub_action_flags_json) values($1,'{"future.feature":true,"flighthub.model.delete":true}')`, pid)
	body := `{"changes":{"live.control":{"expected":false,"enabled":true},"storage.objects":{"expected":false,"enabled":true}}}`
	call("PATCH", path, body, 200)
	call("PATCH", path, body, 409)
	// An unrelated stale page can safely change a different leaf without replacing the saved map.
	call("PATCH", path, `{"changes":{"flight.execute":{"expected":false,"enabled":true}}}`, 200)
	values = call("GET", path, "", 200)["values"].(map[string]any)
	for _, key := range []string{"live.control", "storage.objects", "flight.execute", "flighthub.model.delete"} {
		if values[key] != true {
			t.Fatalf("lost %s", key)
		}
	}
	if _, ok := values["future.feature"]; ok {
		t.Fatal("exposed unknown feature")
	}
	for _, bad := range []string{
		`{"changes":{}}`,
		`{"changes":{"flighthub":{"enabled":true,"expected":false}}}`,
		`{"changes":{"live.*":{"enabled":true,"expected":false}}}`,
		`{"changes":{"live.control":{"enabled":null,"expected":true}}}`,
		`{"changes":{"live.control":{"enabled":false}}}`,
		`{"changes":{"live.control":{"enabled":false,"expected":true,"userId":1}}}`,
		`{"changes":{"live.control":{"enabled":false,"expected":true}},"userId":1}`,
		`{"changes":{"live.control":{"enabled":false,"expected":true},"unknown":{"enabled":true,"expected":false}}}`,
	} {
		call("PATCH", path, bad, 400)
	}
	var preserved, live bool
	if e := f.db.QueryRow(`select flighthub_action_flags_json @> '{"future.feature":true}', flighthub_action_flags_json @> '{"live.control":true}' from project_feature_flags where project_id=$1`, pid).Scan(&preserved, &live); e != nil || !preserved || !live {
		t.Fatalf("invalid patch partially applied %v", e)
	}
	var grants, evidence, audits int
	if e := f.db.QueryRow(`select (select count(*) from project_permissions where project_id=$1),(select count(*) from connector_capability_snapshots where project_id=$1),(select count(*) from audit_events where project_id=$1 and action='project.features.update')`, pid).Scan(&grants, &evidence, &audits); e != nil {
		t.Fatal(e)
	}
	if grants != 0 || evidence != 0 || audits != 2 {
		t.Fatalf("side effects grants=%d evidence=%d audits=%d", grants, evidence, audits)
	}
	// Explicit grants and platform admin status cannot bypass the preset manager roles.
	exec(`update team_members set role='member' where team_id=$1`, team)
	exec(`insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'device:configure' from users where email='admin@example.com'`, pid, team)
	call("GET", path, "", 403)
	call("PATCH", path, `{"changes":{"live.control":{"expected":true,"enabled":false}}}`, 403)
	exec(`update team_members set role='admin' where team_id=$1`, team)
	call("PATCH", path, `{"changes":{"live.control":{"expected":true,"enabled":false}}}`, 200)
	otherTeam, other := f.project(t)
	exec(`delete from team_members where team_id=$1`, otherTeam)
	call("GET", fmt.Sprintf("/api/projects/%d/feature-settings", other), "", 403)
	call("PATCH", fmt.Sprintf("/api/projects/%d/feature-settings", other), body, 403)
}
