package httpapi

import (
	"fmt"
	"testing"
)

func TestLegacyEventDetailAndReadOnlyEndpoints(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	var rule, version, group int64
	var event string
	if err := f.db.QueryRow("insert into event_rules(project_id,team_id,name) values($1,$2,'Legacy rule') returning id", pid, team).Scan(&rule); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into event_rule_versions(project_id,team_id,event_rule_id,version,label,minimum_confidence,severity) values($1,$2,$3,1,'construction',0.5,'medium') returning id", pid, team, rule).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into detection_groups(project_id,team_id,label,location_quality,first_detected_at,last_detected_at) values($1,$2,'construction','unavailable',now(),now()) returning id", pid, team).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into perception_events(id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,severity,first_detected_at,last_detected_at) values(gen_random_uuid(),$1,$2,$3,$4,'legacy-event','medium',now(),now()) returning id", pid, team, version, group).Scan(&event); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/events/%s", pid, event)
	res := f.request(t, "GET", path, "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("detail %d %+v", res.StatusCode, data)
	}
	detail := data["event"].(map[string]any)
	if detail["title"] != "疑似违建" || detail["detectionGroupId"] != fmt.Sprint(group) || detail["hasMapLocation"] != false || detail["locationSummary"] != "位置不可用，仅展示影像内标注" || len(data["detections"].([]any)) != 0 || len(data["feedback"].([]any)) != 0 {
		t.Fatalf("contract %+v", data)
	}
	for _, suffix := range []string{"actions", "agent-drafts"} {
		res = f.request(t, "POST", path+"/"+suffix, `{"action":"resolve"}`)
		data = decodedResponse(t, res)
		if res.StatusCode != 410 || data["error"] != "LEGACY_EVENT_READ_ONLY" || data["issuesHref"] != "../issues" {
			t.Fatalf("legacy %d %+v", res.StatusCode, data)
		}
	}
	var state string
	var stateVersion, count int
	if err := f.db.QueryRow("select status,state_version from perception_events where id=$1", event).Scan(&state, &stateVersion); err != nil || state != "open" || stateVersion != 0 {
		t.Fatalf("mutation %s %d %v", state, stateVersion, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("audit %d %v", count, err)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/events/%s", other, event), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("revoked %d %+v", res.StatusCode, data)
	}
}
