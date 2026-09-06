package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestReportDraftLifecycleAndRollback(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	run := f.missionRun(t, pid, team, "running")
	path := fmt.Sprintf("/api/projects/%d/task-runs/%d/reports", pid, run)
	res := f.request(t, "POST", path, "")
	data := decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_TASK_RUN_NOT_TERMINAL" {
		t.Fatalf("running %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update task_runs set status='succeeded' where id=$1", run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into assets(project_id,team_id,task_run_id,kind,storage_key,logical_key) values($1,$2,$3,'image','report.jpg','report.jpg')", pid, team, run); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 201 || data["completeness"] != "incomplete" || data["version"] != float64(1) || len(data["dataGaps"].([]any)) != 4 {
		t.Fatalf("draft %d %+v", res.StatusCode, data)
	}
	report := data["reportId"].(string)
	version := data["versionId"].(string)
	var raw []byte
	var content map[string]any
	if err := f.db.QueryRow("select content_json from generated_report_versions where id=$1", version).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	if len(content["evidence"].([]any)) != 2 || content["sections"].(map[string]any)["track"] != nil || len(content["conclusions"].([]any)) != 1 {
		t.Fatalf("aggregate %+v", content)
	}
	var count int
	if err := f.db.QueryRow("select count(*) from generated_report_evidence where report_version_id=$1 and asset_id is not null", version).Scan(&count); err != nil || count != 1 {
		t.Fatalf("evidence %d %v", count, err)
	}
	if _, err := f.db.Exec(`create function reject_report_evidence() returns trigger language plpgsql as $$ begin raise exception 'private evidence failure'; end $$;create trigger reject_report_evidence before insert on generated_report_evidence for each row execute function reject_report_evidence()`); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPORT_CREATE_FAILED" {
		t.Fatalf("rollback %d %+v", res.StatusCode, data)
	}
	var status string
	if err := f.db.QueryRow("select status from generated_report_versions where id=$1", version).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("retirement rollback %s %v", status, err)
	}
	if err := f.db.QueryRow("select count(*) from generated_report_versions where generated_report_id=$1", report).Scan(&count); err != nil || count != 1 {
		t.Fatalf("version rollback %d %v", count, err)
	}
	if _, err := f.db.Exec("drop trigger reject_report_evidence on generated_report_evidence"); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 201 || data["reportId"] != report || data["version"] != float64(2) {
		t.Fatalf("next %d %+v", res.StatusCode, data)
	}
	if err := f.db.QueryRow("select status from generated_report_versions where id=$1", version).Scan(&status); err != nil || status != "retired" {
		t.Fatalf("retired %s %v", status, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1", pid).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit %d %v", count, err)
	}
	_, other := f.project(t)
	res = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/reports", other, run), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "TASK_RUN_NOT_FOUND" {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
}

func TestReportAggregateIncludesAllEvidence(t *testing.T) {
	sources := reportSources{Run: gin.H{"id": float64(1), "status": "succeeded", "stateVersion": float64(3), "taskVersionId": "9", "taskVersion": float64(2), "deviceId": float64(4), "deviceUpdatedAt": "2026-09-06T00:00:00Z"}, Track: gin.H{"pointCount": float64(2), "endedAt": "2026-09-06T00:00:00Z"}, Steps: []gin.H{{"id": "5", "status": "succeeded"}}, Events: []gin.H{{"id": "event", "stateVersion": float64(1)}}, Feedback: []gin.H{{"id": "6", "eventId": "event", "reason": "人工确认", "createdAt": "2026-09-06T00:00:00Z"}}, Assets: []gin.H{{"id": float64(7), "version": float64(1), "checksum": "abc"}}}
	content, refs, err := aggregateReport(1, 1, sources)
	if err != nil || content["completeness"] != "complete" || len(refs) != 8 || len(content["dataGaps"].([]gin.H)) != 0 {
		t.Fatalf("complete %+v %v", content, err)
	}
	conclusions := content["conclusions"].([]gin.H)
	if len(conclusions) != 2 || conclusions[1]["kind"] != "human-conclusion" || conclusions[1]["text"] != "人工确认" {
		t.Fatalf("conclusions %+v", conclusions)
	}
	if refs[7]["version"] != "abc" || refs[7]["assetId"] != float64(7) {
		t.Fatalf("asset %+v", refs[7])
	}
	want := []string{
		"/projects/tasks/runs/detail/?projectId=1&runId=1",
		"/projects/tasks/?projectId=1",
		"/projects/devices/?projectId=1&selected=4",
		"/projects/detail/?projectId=1&selected=4",
		"/projects/tasks/runs/detail/?projectId=1&runId=1",
		"/projects/events/detail/?eventId=event&projectId=1",
		"/projects/events/detail/?eventId=event&projectId=1",
		"/projects/assets/?projectId=1&selected=7",
	}
	for i, href := range want {
		if refs[i]["href"] != href {
			t.Fatalf("reference %d: %+v", i, refs[i])
		}
	}
}
