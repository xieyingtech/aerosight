package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestInspectionReviewAPIIsScopedAuditedAndIdempotent(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
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
	var uid int32
	if err := f.db.QueryRow("select user_id from team_members where team_id=$1", team).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	run := f.missionRun(t, pid, team, "paused")
	var task int64
	if err := f.db.QueryRow("select task_id from task_runs where id=$1", run).Scan(&task); err != nil {
		t.Fatal(err)
	}
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", pid, team, task)
	exec("update task_runs set task_version_id=$2,created_by_user_id=$3 where id=$1", run, version, uid)
	steps := []int64{}
	for n, uses := range []string{"inspection.observe", "inspection.detect", "copilot.run"} {
		step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,$4,$5,$5,$5,$5) returning id", pid, team, version, n+1, uses)
		steps = append(steps, id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,$5,'paused') returning id", pid, team, run, step, n+1))
	}
	observation, evidence, assessment := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','partial','review fixture',now(),now(),'{}',now())", observation, pid, team, run, steps[0])
	evidenceJSON := fmt.Sprintf(`{"id":%q,"run":{"projectId":%d,"teamId":%d,"runId":%d,"stepId":%d},"observationId":%q,"source":"external","modelVersion":"fixture","completeness":"partial","targetAlgorithmConfirmed":false,"evidenceRefs":["fixture-ref"],"candidates":[]}`, evidence, pid, team, run, steps[1], observation)
	exec("insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,completeness,model_version,evidence_json) values($1,$2,$3,$4,$5,$6,'external','partial','fixture',$7)", evidence, pid, team, run, steps[1], observation, evidenceJSON)
	exec("insert into inspection_assessments(id,project_id,team_id,task_run_id,task_run_step_id,evidence_set_id,status,revision,original_output) values($1,$2,$3,$4,$5,$6,'needs_review',1,'original protocol fixture')", assessment, pid, team, run, steps[2], evidence)
	exec(`insert into inspection_assessment_revisions(assessment_id,project_id,revision,source,decisions_json,idempotency_key) values($1,$2,1,'model','[]','model-fixture')`, assessment, pid)
	var agent int64
	if err := f.db.QueryRow("select id from agents where project_id=$1 and config_json->>'kind'='copilot'", pid).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	session := id("insert into agent_sessions(project_id,agent_id,task_run_id,started_by_user_id) values($1,$2,$3,$4) returning id", pid, agent, run, uid)
	exec(`insert into agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,tool_name,required_permission,args_json,status,context_expires_at) values($1,$2,$3,$4,'inspection_assessment','agent:use',jsonb_build_object('assessmentId',$5::text),'succeeded',now()+interval '1 day')`, pid, team, session, uid, assessment)
	path := fmt.Sprintf("/api/projects/%d/inspection/assessments/%s", pid, assessment)
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s got %d want %d %+v", path, res.StatusCode, want, out)
		}
		return out
	}
	summaryPath := fmt.Sprintf("/api/projects/%d/task-runs/%d/inspection-summary", pid, run)
	before := call("GET", summaryPath, nil, 200)
	if before["pendingReviewCount"] != float64(1) || before["final"] != false || before["runStatus"] != "paused" {
		t.Fatal("pending summary missing", before)
	}
	content := before["inspection"].(map[string]any)
	assessments := content["assessments"].([]any)
	if len(assessments) != 1 || assessments[0].(map[string]any)["status"] != "needs_review" {
		t.Fatal("pending assessment missing", before)
	}
	if len(before["dataGaps"].([]any)) == 0 {
		t.Fatal("pending summary has no data gaps")
	}
	var reports int
	if err := f.db.QueryRow("select count(*) from generated_reports where project_id=$1", pid).Scan(&reports); err != nil || reports != 0 {
		t.Fatal("read created a report", reports, err)
	}
	read := call("GET", path, nil, 200)
	if read["originalOutput"] != "original protocol fixture" {
		t.Fatal("original missing")
	}
	denied := call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), map[string]any{"action": "resume", "expectedVersion": 0, "reason": "normal resume"}, 409)
	if denied["error"] != "INSPECTION_REVIEW_REQUIRED" {
		t.Fatal("review bypassed", denied)
	}
	_, other := f.project(t)
	call("GET", fmt.Sprintf("/api/projects/%d/inspection/assessments/%s", other, assessment), nil, 404)
	body := map[string]any{"expectedRevision": 1, "idempotencyKey": "reject-once", "decisions": []any{map[string]any{"action": "reject", "reason": "dismiss fixture", "evidenceRefs": []string{"fixture-ref"}, "missingInformation": []string{}}}}
	call("GET", fmt.Sprintf("/api/projects/%d/task-runs/%d/inspection-summary", other, run), nil, 404)
	call("GET", fmt.Sprintf("/api/projects/%d/task-runs/2147483647/inspection-summary", pid), nil, 404)
	call("POST", path+"/review", body, 200)
	after := call("GET", summaryPath, nil, 200)
	if after["pendingReviewCount"] != float64(0) || after["runStatus"] != "running" || after["final"] != false {
		t.Fatal("stale summary after review", after)
	}

	replay := call("POST", path+"/review", body, 200)
	if replay["replayed"] != true {
		t.Fatal("review did not replay")
	}
	var count int
	if err := f.db.QueryRow("select count(*) from inspection_assessment_revisions where assessment_id=$1", assessment).Scan(&count); err != nil || count != 2 {
		t.Fatal("duplicate revision", count, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and action='inspection.assessment.review' and status='completed'", pid).Scan(&count); err != nil || count != 2 {
		t.Fatal("audit missing", count, err)
	}
	exec("update task_runs set status='canceled' where id=$1", run)
	call("POST", path+"/review", body, 409)
	exec("delete from team_members where team_id=$1", team)
	call("GET", summaryPath, nil, 403)
}
