package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAgentWorkflowInputBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"run_task", `{"taskId":1,"expectedVersionId":2,"inputs":{}}`},
		{"run_algorithm", `{"configurationSnapshotId":1,"assetId":2,"parameters":{}}`},
		{"publish_task", `{"taskId":1,"versionId":2,"expectedRevision":1}`},
	} {
		if _, _, err := parseAgentWorkflowInput(tc.name, json.RawMessage(tc.raw)); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	for _, tc := range []struct{ name, raw string }{
		{"run_task", `{"taskId":1,"expectedVersionId":2,"inputs":{"userId":3}}`},
		{"run_task", `{"taskId":1,"inputs":{}}`},
		{"run_algorithm", `{"configurationSnapshotId":1,"assetId":2,"parameters":{},"url":"https://evil.example"}`},
		{"publish_task", `{"taskId":1.5,"versionId":2,"expectedRevision":1}`},
		{"generate_report", `{"taskRunId":0}`},
		{"http_request", `{"path":"/admin"}`},
		{"submit_flight", `{"connectorId":1,"taskRunId":2,"approvalRequestId":"missing","waylineResourceId":3,"request":{"taskType":"recurring"}}`},
	} {
		if _, _, err := parseAgentWorkflowInput(tc.name, json.RawMessage(tc.raw)); err == nil {
			t.Fatalf("accepted %s %s", tc.name, tc.raw)
		}
	}
	specs := map[string]bool{}
	for _, spec := range agentWorkflowTools() {
		if spec.Name == "" || spec.Permission == "" || specs[spec.Name] {
			t.Fatal("invalid catalogue")
		}
		specs[spec.Name] = true
	}
	if !agentIsWriteTool("submit_flight") || agentIsWriteTool("query_inspection") {
		t.Fatal("write classification")
	}
}

func TestAgentWorkflowWritesRequireClickAndFreezeVersions(t *testing.T) {
	f, _, pid, uid, sid, _ := newChatFixture(t)
	stubApprovalFollowup(t, f, "平台已返回处理结果。")
	definition, _, _, err := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": reportTaskYAML})
	if err != nil {
		t.Fatal(err)
	}
	propose := func(name string, args any) string {
		t.Helper()
		raw, _ := json.Marshal(args)
		result, e := f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, name, raw, "workflow-test")
		if e != nil {
			t.Fatal(e)
		}
		return result["approvalId"].(string)
	}
	decide := func(id, decision string) (int, map[string]any) {
		t.Helper()
		response := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, id), fmt.Sprintf(`{"decision":%q}`, decision))
		return response.StatusCode, decodedResponse(t, response)
	}
	approval := propose("create_inspection_task", gin.H{"definition": definition})
	var count int
	if err = f.db.QueryRow("select count(*) from tasks where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatal("created before click", count, err)
	}
	code, result := decide(approval, "approve")
	if code != 200 || result["status"] != "succeeded" {
		t.Fatalf("create %d %+v", code, result)
	}
	output := result["result"].(map[string]any)["output"].(map[string]any)
	tid, vid := output["taskId"], output["versionId"]
	if code, _ = decide(approval, "approve"); code != 409 {
		t.Fatal("duplicate approval accepted")
	}
	if err = f.db.QueryRow("select count(*) from tasks where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicated task", count, err)
	}
	stale := propose("publish_task", gin.H{"taskId": tid, "versionId": vid, "expectedRevision": 1})
	if _, err = f.db.Exec("update task_versions set author_revision=author_revision+1 where id=$1", vid); err != nil {
		t.Fatal(err)
	}
	code, result = decide(stale, "approve")
	if code != 200 || result["status"] != "failed" {
		t.Fatalf("stale publish %d %+v", code, result)
	}
	var state string
	if err = f.db.QueryRow("select status from task_versions where id=$1", vid).Scan(&state); err != nil || state != "draft" {
		t.Fatal("published stale version", state, err)
	}
	publish := propose("publish_task", gin.H{"taskId": tid, "versionId": vid, "expectedRevision": 2})
	code, result = decide(publish, "approve")
	if code != 200 || result["status"] != "succeeded" {
		t.Fatalf("publish %d %+v", code, result)
	}
	enable := propose("set_task_state", gin.H{"taskId": tid, "status": "active", "expectedVersionId": vid})
	code, result = decide(enable, "approve")
	if code != 200 || result["status"] != "succeeded" {
		t.Fatalf("enable %d %+v", code, result)
	}
	run := propose("run_task", gin.H{"taskId": tid, "expectedVersionId": vid, "inputs": gin.H{}})
	if err = f.db.QueryRow("select count(*) from task_runs where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
		t.Fatal("run before click", count, err)
	}
	code, result = decide(run, "approve")
	if code != 200 || result["status"] != "succeeded" {
		t.Fatalf("run %d %+v", code, result)
	}
	if code, _ = decide(run, "approve"); code != 409 {
		t.Fatal("duplicate run approval accepted")
	}
	if err = f.db.QueryRow("select count(*) from task_runs where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicated run", count, err)
	}
}

func TestAgentWorkflowRejectExpireScopeAndRevocation(t *testing.T) {
	f, team, pid, uid, sid, _ := newChatFixture(t)
	stubApprovalFollowup(t, f, "处理失败。")
	run := f.missionRun(t, pid, team, "running")
	args := gin.H{"taskRunId": run, "action": "pause", "expectedVersion": 0, "reason": "operator"}
	raw, _ := json.Marshal(args)
	propose := func() string {
		t.Helper()
		out, e := f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "control_task_run", raw, "test")
		if e != nil {
			t.Fatal(e)
		}
		return out["approvalId"].(string)
	}
	click := func(id, decision string) int {
		r := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, id), fmt.Sprintf(`{"decision":%q}`, decision))
		r.Body.Close()
		return r.StatusCode
	}
	rejected := propose()
	if click(rejected, "reject") != 200 || click(rejected, "approve") != 409 {
		t.Fatal("rejected operation executed")
	}
	expired := propose()
	if _, e := f.db.Exec("update agent_write_approvals set expires_at=now()-interval '1 second' where id=$1", expired); e != nil {
		t.Fatal(e)
	}
	if click(expired, "approve") != 410 {
		t.Fatal("expired approval accepted")
	}
	otherTeam, otherPID := f.project(t)
	otherRun := f.missionRun(t, otherPID, otherTeam, "running")
	args["taskRunId"] = otherRun
	foreign, _ := json.Marshal(args)
	if _, e := f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "control_task_run", foreign, "test"); e == nil {
		t.Fatal("foreign run accepted")
	}
	revoke := propose()
	if _, e := f.db.Exec("delete from team_members where team_id=$1 and user_id=$2", team, uid); e != nil {
		t.Fatal(e)
	}
	if click(revoke, "approve") != 403 {
		t.Fatal("revoked user executed")
	}
	var state string
	if e := f.db.QueryRow("select status from task_runs where id=$1", run).Scan(&state); e != nil || state != "running" {
		t.Fatal("unexpected mutation", state, e)
	}
}

func TestAgentInspectionQueryAndPhotoBoundaries(t *testing.T) {
	f, _, pid, uid, _, _ := newChatFixture(t)
	result, err := f.server.executeInspectionQuery(context.Background(), uid, int32(pid), json.RawMessage(`{"resource":"readiness"}`))
	if err != nil || result["quality"] != "authoritative-project-query" {
		t.Fatal(result, err)
	}
	if _, err = f.server.executeInspectionQuery(context.Background(), uid, int32(pid), json.RawMessage(`{"resource":"task_run"}`)); err == nil {
		t.Fatal("missing ID accepted")
	}
	if _, err = f.server.executeInspectionQuery(context.Background(), uid, int32(pid), json.RawMessage(`{"resource":"readiness","projectId":2}`)); err == nil {
		t.Fatal("project override accepted")
	}
	var imageBytes bytes.Buffer
	if err = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2048, 1024))); err != nil {
		t.Fatal(err)
	}
	preview, err := agentPhotoPreview(imageBytes.Bytes())
	if err != nil || preview.width != 1024 || preview.height != 512 {
		t.Fatal("image preview", preview.width, preview.height, err)
	}
	if _, err = agentPhotoPreview([]byte("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}

func TestAgentWorkflowConcurrentClickAndFlightGate(t *testing.T) {
	f, team, pid, uid, sid, _ := newChatFixture(t)
	stubApprovalFollowup(t, f, "平台已返回处理结果。")
	definition, _, _, e := parseTaskAuthorInput(map[string]any{"sourceFormat": "yaml", "source": reportTaskYAML})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(gin.H{"definition": definition})
	proposal, e := f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "create_inspection_task", raw, "test")
	if e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, proposal["approvalId"])
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r := f.request(t, "POST", path, `{"decision":"approve"}`)
			r.Body.Close()
			codes <- r.StatusCode
		}()
	}
	one, two := <-codes, <-codes
	if !(one == 200 && two == 409 || one == 409 && two == 200) {
		t.Fatalf("concurrent decisions %d/%d", one, two)
	}
	var count int
	if e = f.db.QueryRow("select count(*) from tasks where project_id=$1", pid).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate creation", count, e)
	}
	cid, _ := f.device(t, team, pid)
	run := f.missionRun(t, pid, team, "ready")
	var wayline int
	if e = f.db.QueryRow(`insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'wayline','fixture-only') returning id`, pid, team, cid).Scan(&wayline); e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(gin.H{"connectorId": cid, "taskRunId": run, "approvalRequestId": uuid.NewString(), "waylineResourceId": wayline, "request": gin.H{"name": "fixture only", "timeZone": "Asia/Shanghai", "taskType": "immediate"}})
	proposal, e = f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "submit_flight", raw, "test")
	if e != nil {
		t.Fatal(e)
	}
	r := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, proposal["approvalId"]), `{"decision":"approve"}`)
	body := decodedResponse(t, r)
	if r.StatusCode != 200 || body["status"] != "failed" {
		t.Fatalf("flight gate bypassed: %d %+v", r.StatusCode, body)
	}
	if e = f.db.QueryRow("select count(*) from connector_action_jobs where project_id=$1", pid).Scan(&count); e != nil || count != 0 {
		t.Fatal("flight job without prerequisites", count, e)
	}
	terminal := f.missionRun(t, pid, team, "succeeded")
	raw, _ = json.Marshal(gin.H{"taskRunId": terminal})
	proposal, e = f.server.proposeWorkflowWrite(context.Background(), uid, int32(pid), sid, "generate_report", raw, "test")
	if e != nil {
		t.Fatal(e)
	}
	r = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, proposal["approvalId"]), `{"decision":"approve"}`)
	body = decodedResponse(t, r)
	if r.StatusCode != 200 || body["status"] != "succeeded" {
		t.Fatalf("report tool: %d %+v", r.StatusCode, body)
	}
}
