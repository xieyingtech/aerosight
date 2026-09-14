package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"strings"
	"testing"
)

func TestInspectionAlertPolicyHTTPAuthorizationAndReplay(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, otherPID := f.project(t)
	id := func(query string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := f.db.QueryRow(query, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	adapter := id(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
 select $1,$2,'policy','dji-flighthub2',id,'2','connected' from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0' returning id`, pid, team)
	resource := id(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'ai-alert','private-alert-identity') returning id`, pid, team, adapter)
	if _, err := f.db.Exec(`insert into inspection_alert_sources(project_id,connector_instance_id,remote_resource_id,remote_flight_id,evidence_json) values($1,$2,$3,'private-flight-identity','{}')`, pid, adapter, resource); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`insert into inspection_flight_ownership(project_id,connector_instance_id,remote_flight_id,ownership) values($1,$2,'private-flight-identity','pending')`, pid, adapter); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/inspection/connectors/%d/alert-policy", pid, adapter)
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %d want %d %+v", path, res.StatusCode, want, out)
		}
		return out
	}
	view := call("GET", path, nil, 200)
	if view["taskManagedAlerts"] != false || len(view["heldAlerts"].([]any)) != 1 {
		t.Fatal(view)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "private-flight-identity") || strings.Contains(string(encoded), "private-alert-identity") {
		t.Fatal("private remote identity leaked")
	}
	body := gin.H{"action": "set-policy", "taskManagedAlerts": true, "idempotencyKey": "enable-policy"}
	call("POST", path, body, 200)
	replay := call("POST", path, body, 200)
	if replay["replayed"] != true {
		t.Fatal("policy retry not replayed")
	}
	body["taskManagedAlerts"] = false
	call("POST", path, body, 409)
	cross := fmt.Sprintf("/api/projects/%d/inspection/connectors/%d/alert-policy", otherPID, adapter)
	call("GET", cross, nil, 404)
	body["idempotencyKey"] = "cross"
	call("POST", cross, body, 404)
	release := gin.H{"action": "confirm-legacy", "resourceId": resource, "idempotencyKey": "release-flight"}
	call("POST", path, release, 200)
	view = call("GET", path, nil, 200)
	if len(view["heldAlerts"].([]any)) != 0 {
		t.Fatal("released flight remains held")
	}
	// Revocation is checked before serving either a cached replay or changing policy.
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call("POST", path, release, 403)
	call("GET", path, nil, 403)
}
