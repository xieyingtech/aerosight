package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestMainIssueFeedbackAndReport(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "succeeded")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	number := func(q string, args ...any) int {
		t.Helper()
		var id int
		if e := f.db.QueryRow(q, args...).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	provider := number("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'feedback','http-json','https://algorithm.example','active') returning id", pid, team)
	definition := number("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'feedback','detection') returning id", pid, team, provider)
	version := number("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','callback','test','{}') returning id", pid, team, definition)
	asset := number("insert into assets(project_id,team_id,task_run_id,kind,storage_key,logical_key,status) values($1,$2,$3,'image','feedback.jpg','feedback.jpg','available') returning id", pid, team, run)
	const algorithm = "ac111111-1111-4111-8111-111111111111"
	exec("insert into algorithm_runs(id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status) values($1,$2,$3,$4,$5,'feedback-test','succeeded')", algorithm, pid, team, version, asset)
	detection := number("insert into detections(project_id,team_id,algorithm_run_id,input_asset_id,task_run_id,detection_key,label,confidence,pixel_geometry_json,transform_version,captured_at) values($1,$2,$3,$4,$5,'test','person',0.9,'{}','test',now()) returning id", pid, team, algorithm, asset, run)
	issue := number("insert into issues(project_id,number,title,source_type,task_run_id) values($1,1,'Main feedback','manual',$2) returning id", pid, run)
	exec("insert into issue_links(project_id,issue_id,link_type,target_id) values($1,$2,'detection',$3)", pid, issue, fmt.Sprint(detection))
	body := gin.H{"expectedVersion": 0, "clientKey": "ad111111-1111-4111-8111-111111111111", "detectionId": detection, "action": "confirm", "reason": "人工确认"}
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s: %d %+v", path, res.StatusCode, data)
		}
		return data
	}
	path := fmt.Sprintf("/api/projects/%d/issues/%d/feedback", pid, issue)
	first := call("POST", path, body, 200)
	if replay := call("POST", path, body, 200); replay["id"] != first["id"] || replay["replayed"] != true {
		t.Fatalf("replay %+v", replay)
	}
	body["clientKey"] = "ae111111-1111-4111-8111-111111111111"
	call("POST", path, body, 409)
	body["expectedVersion"] = 1
	exec("create function reject_feedback_event() returns trigger language plpgsql as $$ begin raise exception 'private failure'; end $$;create trigger reject_feedback_event before insert on issue_events for each row execute function reject_feedback_event()")
	call("POST", path, body, 400)
	if count := number("select count(*) from issue_feedback where issue_id=$1", issue); count != 1 {
		t.Fatalf("rollback count %d", count)
	}
	exec("drop trigger reject_feedback_event on issue_events;drop function reject_feedback_event()")
	report := call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/reports", pid, run), nil, 201)
	var raw []byte
	if e := f.db.QueryRow("select content_json from generated_report_versions where id=$1", report["versionId"]).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var content map[string]any
	if e := json.Unmarshal(raw, &content); e != nil {
		t.Fatal(e)
	}
	sections := content["sections"].(map[string]any)
	if len(sections["issues"].([]any)) != 1 || len(sections["comments"].([]any)) != 1 || len(sections["feedback"].([]any)) != 1 {
		t.Fatalf("report missing main evidence %s", raw)
	}
}
