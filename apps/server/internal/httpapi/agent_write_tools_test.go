package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"testing"
)

func stubApprovalFollowup(t *testing.T, f *apiFixture, answer string) {
	t.Helper()
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, hasTools := body["tools"]; hasTools {
				t.Fatal("approval followup must not offer write tools")
			}
			return chatResponse(r, []any{chatText(answer)}), nil
		})}
	}
}

func TestIssueToolInputBoundary(t *testing.T) {
	valid := json.RawMessage(`{"issueId":12,"expectedVersion":3,"mutation":{"action":"comment","body":"请核查"}}`)
	id, input, mutation, version, key, err := issueToolInput(valid)
	if err != nil || id != 12 || version != 3 || mutation.Action != "comment" || input["clientKey"] != key {
		t.Fatalf("valid input: %d %v %v", id, input, err)
	}
	for _, raw := range []string{
		`{"issueId":12,"expectedVersion":3,"mutation":{"action":"delete"}}`,
		`{"issueId":12,"expectedVersion":3,"mutation":{"action":"comment","body":"x"},"projectId":99}`,
		`{"issueId":12,"expectedVersion":3,"mutation":{"action":"comment","body":"x","userId":99}}`,
		`{"issueId":1.2,"expectedVersion":3,"mutation":{"action":"comment","body":"x"}}`,
	} {
		if _, _, _, _, _, err := issueToolInput(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestTaskDraftToolInputBoundary(t *testing.T) {
	if id, err := taskDraftToolInput(json.RawMessage(`{"taskId":42}`)); err != nil || id != 42 {
		t.Fatalf("valid task: %d %v", id, err)
	}
	for _, raw := range []string{`{"taskId":0}`, `{"taskId":1.5}`, `{"taskId":2,"userId":1}`, `{"taskId":"2"}`} {
		if _, err := taskDraftToolInput(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestAgentIssueWriteRequiresClickAndCurrentRBAC(t *testing.T) {
	f, team, pid, uid, sid, _ := newChatFixture(t)
	stubApprovalFollowup(t, f, "已为案件添加评论。")
	var iid int32
	if err := f.db.QueryRow(`INSERT INTO issues(project_id,number,title,source_type) VALUES($1,1,'Case','manual') RETURNING id`, pid).Scan(&iid); err != nil {
		t.Fatal(err)
	}
	propose := func(version int, body string) string {
		t.Helper()
		raw := json.RawMessage(fmt.Sprintf(`{"issueId":%d,"expectedVersion":%d,"mutation":{"action":"comment","body":%q}}`, iid, version, body))
		result, err := f.server.proposeIssueWrite(context.Background(), uid, int32(pid), sid, raw, "test")
		if err != nil {
			t.Fatal(err)
		}
		return result["approvalId"].(string)
	}
	id := propose(0, "first")
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM issue_events WHERE issue_id=$1`, iid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("write before click: %d %v", count, err)
	}
	path := fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, id)
	response := f.request(t, "POST", path, `{"decision":"approve"}`)
	approved := decodedResponse(t, response)
	if response.StatusCode != 200 || approved["followupStatus"] != "sent" {
		t.Fatalf("approve: %d %+v", response.StatusCode, approved)
	}
	var followup string
	if err := f.db.QueryRow(`SELECT content FROM agent_messages WHERE session_id=$1 AND role='assistant' ORDER BY id DESC LIMIT 1`, sid).Scan(&followup); err != nil || followup != "已为案件添加评论。" {
		t.Fatalf("followup: %q %v", followup, err)
	}
	expiredID := propose(1, "expired")
	if _, err := f.db.Exec(`UPDATE agent_write_approvals SET expires_at=now()-interval '1 second' WHERE id=$1`, expiredID); err != nil {
		t.Fatal(err)
	}
	response = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, expiredID), `{"decision":"approve"}`)
	expiredBody := decodedResponse(t, response)
	if response.StatusCode != 410 || expiredBody["error"] != "AGENT_APPROVAL_EXPIRED" {
		t.Fatalf("expired approval: %d %+v", response.StatusCode, expiredBody)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM issue_events WHERE issue_id=$1`, iid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("approved write: %d %v", count, err)
	}
	response = f.request(t, "POST", path, `{"decision":"approve"}`)
	if response.StatusCode != 409 {
		t.Fatalf("replayed approval: %d", response.StatusCode)
	}
	response.Body.Close()
	id = propose(1, "second")
	if _, err := f.db.Exec(`UPDATE team_members SET role='member' WHERE team_id=$1 AND user_id=$2`, team, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO project_permissions(project_id,team_id,user_id,permission) VALUES($1,$2,$3,'agent:use') ON CONFLICT DO NOTHING`, pid, team, uid); err != nil {
		t.Fatal(err)
	}
	response = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, id), `{"decision":"approve"}`)
	if response.StatusCode != 403 {
		t.Fatalf("revoked issue permission: %d %+v", response.StatusCode, decodedResponse(t, response))
	}
	response.Body.Close()
	if err := f.db.QueryRow(`SELECT count(*) FROM issue_events WHERE issue_id=$1`, iid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("write after revoke: %d %v", count, err)
	}
}

func TestAgentTaskDraftRequiresClick(t *testing.T) {
	f, _, pid, uid, sid, _ := newChatFixture(t)
	stubApprovalFollowup(t, f, "草稿已创建。")
	created := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks", pid), fmt.Sprintf(`{"sourceFormat":"yaml","source":%q,"idempotencyKey":"agent-draft-test"}`, reportTaskYAML))
	if created.StatusCode != 201 {
		t.Fatalf("create task: %d %+v", created.StatusCode, decodedResponse(t, created))
	}
	task := decodedResponse(t, created)
	tid := int32(task["taskId"].(float64))
	published := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/tasks/%d/versions", pid, tid), fmt.Sprintf(`{"action":"publish","versionId":%v,"expectedRevision":1}`, task["versionId"]))
	if published.StatusCode != 200 {
		t.Fatalf("publish original: %d %+v", published.StatusCode, decodedResponse(t, published))
	}
	published.Body.Close()
	result, err := f.server.proposeTaskDraft(context.Background(), uid, int32(pid), sid, json.RawMessage(fmt.Sprintf(`{"taskId":%d}`, tid)), "test")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.db.QueryRow(`SELECT count(*) FROM task_versions WHERE task_id=$1`, tid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("draft before click: %d %v", count, err)
	}
	path := fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%s", pid, sid, result["approvalId"])
	response := f.request(t, "POST", path, `{"decision":"approve"}`)
	approved := decodedResponse(t, response)
	if response.StatusCode != 200 || approved["followupStatus"] != "sent" {
		t.Fatalf("approve task: %d %+v", response.StatusCode, approved)
	}
	if err = f.db.QueryRow(`SELECT count(*) FROM task_versions WHERE task_id=$1`, tid).Scan(&count); err != nil || count != 2 {
		t.Fatalf("draft after click: %d %v", count, err)
	}
}
