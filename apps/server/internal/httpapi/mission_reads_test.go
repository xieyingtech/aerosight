package httpapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestMissionWorkbenchReadContract(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	run := f.missionRun(t, pid, team, "running")
	path := fmt.Sprintf("/api/projects/%d/task-runs", pid)
	res := f.request(t, "GET", path, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || len(rows) != 1 || rows[0]["id"] != float64(run) || rows[0]["deviceName"] != nil || rows[0]["startedAt"] != nil {
		t.Fatalf("list %d %+v", res.StatusCode, rows)
	}
	item := path + fmt.Sprintf("/%d", run)
	res = f.request(t, "GET", item, "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["steps"].([]any)) != 0 || !reflect.DeepEqual(data["actions"], []any{"pause", "cancel", "emergency_stop"}) {
		t.Fatalf("empty workbench %d %+v", res.StatusCode, data)
	}
	var version, step, runStep int64
	if err := f.db.QueryRow("insert into task_versions(project_id,team_id,task_id,version,script) select project_id,team_id,task_id,1,'' from task_runs where id=$1 returning id", run).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,capability_code) values($1,$2,$3,1,'capture','Capture','camera.photo','camera.photo') returning id", pid, team, version).Scan(&step); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position) values($1,$2,$3,$4,1) returning id", pid, team, run, step).Scan(&runStep); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update task_runs set task_version_id=$2,selected_device_id=$3 where id=$1", run, version, did); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_commands(id,project_id,team_id,device_id,task_run_id,task_run_step_id,command_key,idempotency_key,capability_code,status,deadline_at,created_at) select gen_random_uuid(),$1,$2,$3,$4,$5,'camera.photo','workbench-'||n,'camera.photo',case when n=1 then 'nacked' else 'acknowledged' end,now()+interval '1 hour',now()+n*interval '1 second' from generate_series(1,2)n", pid, team, did, run, runStep); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", item, "")
	data = decodedResponse(t, res)
	detail := data["run"].(map[string]any)
	latest := data["steps"].([]any)[0].(map[string]any)
	if detail["deviceId"] != float64(did) || detail["taskVersion"] != float64(1) || latest["commandStatus"] != "acknowledged" || latest["capabilityCode"] != "camera.photo" {
		t.Fatalf("detail %+v", data)
	}
	if _, ok := latest["commandId"].(string); !ok {
		t.Fatal("command UUID must be a string")
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", item, "")
	data = decodedResponse(t, res)
	if len(data["actions"].([]any)) != 0 {
		t.Fatalf("read-only actions %+v", data)
	}
	if _, err := f.db.Exec("update task_runs set status='blocked' where id=$1", run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'mission:approve' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", item, "")
	data = decodedResponse(t, res)
	if !reflect.DeepEqual(data["actions"], []any{"approve"}) {
		t.Fatalf("approver actions %+v", data)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/task-runs/%d", other, run), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 404 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
}

func TestTaskDefinitionReadScope(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/tasks", pid)
	res := f.request(t, "GET", path, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || rows == nil || len(rows) != 0 {
		t.Fatalf("empty %d %+v", res.StatusCode, rows)
	}
	run := f.missionRun(t, pid, team, "queued")
	var task int
	if err := f.db.QueryRow("select task_id from task_runs where id=$1", run).Scan(&task); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(rows) != 1 || rows[0]["id"] != float64(task) || rows[0]["description"] != nil || rows[0]["triggerType"] != "manual" {
		t.Fatalf("list %+v", rows)
	}
	res = f.request(t, "GET", fmt.Sprintf("%s/%d", path, task), "")
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || data["projectId"] != float64(pid) {
		t.Fatalf("detail %d %+v", res.StatusCode, data)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/tasks/%d", other, task), "")
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
