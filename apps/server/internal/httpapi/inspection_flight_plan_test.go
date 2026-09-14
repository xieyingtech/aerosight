package httpapi

import (
	"context"
	"fmt"
	"testing"
)

func TestInspectionFlightPlanPreviewScopedAndNoActions(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	id := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	cid := id("insert into device_adapters(project_id,team_id,name,adapter_type,status) values($1,$2,'plan','dji-flighthub2','connected') returning id", pid, team)
	exec("insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'plan-device','managed')", pid, team, cid, did)
	rid := id(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,remote_version,summary_json) values($1,$2,$3,'wayline','plan-wayline','1000:200','{"name":"plan","updatedAt":1000,"sizeBytes":200}') returning id`, pid, team, cid)
	path := fmt.Sprintf("/api/projects/%d/inspection/flight-plan?connectorId=%d&waylineResourceId=%d&deviceId=%d", pid, cid, rid, did)
	call := func(path string, want int) map[string]any {
		t.Helper()
		res := f.request(t, "GET", path, "")
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("preview %d %+v", res.StatusCode, out)
		}
		return out
	}
	options := call(fmt.Sprintf("/api/projects/%d/inspection/flight-plan-options", pid), 200)
	if len(options["waylines"].([]any)) != 1 || len(options["devices"].([]any)) != 1 {
		t.Fatal("scoped options missing", options)
	}
	out := call(path, 200)
	if out["verification"] != "catalogue-only" || out["waylineVersion"].(map[string]any)["remoteVersion"] != "1000:200" {
		t.Fatal("snapshot missing", out)
	}
	validate := func(want string) {
		t.Helper()
		tx, err := f.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		input := map[string]any{"mode": "flighthub-flight", "connectorId": cid, "deviceId": did, "waylineResourceId": rid, "waylineVersion": out["waylineVersion"], "schedulerOwner": "aerosight", "taskType": "immediate"}
		err = validateInspectionFlightSelection(context.Background(), tx, int32(pid), input)
		if want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || err.Error() != want {
			t.Fatalf("want %s, got %v", want, err)
		}
	}
	validate("")
	frozen := out["waylineVersion"].(map[string]any)
	frozen["waylineId"] = "forged"
	validate("TASK_WAYLINE_VERSION_INVALID")
	frozen["waylineId"] = "plan-wayline"
	exec("update device_external_identities set discovery_status='discovered' where adapter_id=$1", cid)
	validate("TASK_RESOURCE_SCOPE_INVALID")
	call(path, 404)
	exec("update device_external_identities set discovery_status='managed' where adapter_id=$1", cid)
	if n := id("select count(*) as id from connector_action_jobs where project_id=$1", pid); n != 0 {
		t.Fatal("preview created action", n)
	}
	_, other := f.project(t)
	call(fmt.Sprintf("/api/projects/%d/inspection/flight-plan?connectorId=%d&waylineResourceId=%d&deviceId=%d", other, cid, rid, did), 404)
	exec(`update connector_remote_resources set summary_json='{"updatedAt":2000,"sizeBytes":200}' where id=$1`, rid)
	call(path, 409)
	validate("TASK_WAYLINE_VERSION_INVALID")
	exec(`update connector_remote_resources set summary_json='{}' where id=$1`, rid)
	call(path, 409)
	exec("update connector_remote_resources set status='missing',missing_at=now() where id=$1", rid)
	call(path, 404)
	exec("delete from team_members where team_id=$1", team)
	call(path, 403)
}
