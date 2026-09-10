package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func chatResponse(r *http.Request, output []any) *http.Response {
	raw, _ := json.Marshal(gin.H{"id": "resp_test", "object": "response", "status": "completed", "output": output})
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}
}
func chatFunction(name, id, args string) gin.H {
	return gin.H{"type": "function_call", "id": "fc_" + id, "call_id": id, "name": name, "arguments": args, "status": "completed"}
}
func chatText(text string) gin.H {
	return gin.H{"type": "message", "id": "msg_test", "role": "assistant", "status": "completed", "phase": "final_answer", "content": []any{gin.H{"type": "output_text", "text": text, "annotations": []any{}}}}
}

func newChatFixture(t *testing.T) (*apiFixture, int, int, int32, int32, string) {
	t.Helper()
	f := newAPIFixture(t)
	team, pid := f.project(t)
	f.server.credentialSecret = "chat-sdk-test-secret"
	res := f.request(t, "POST", "/api/admin/ai-providers", aiProviderBody)
	data := decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("provider %+v", data)
	}
	res = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/agent-sessions", pid), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("session %+v", data)
	}
	sid := int32(data["id"].(float64))
	var uid int32
	if err := f.db.QueryRow("select id from users where email='admin@example.com'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	return f, team, pid, uid, sid, fmt.Sprintf("/api/projects/%d/agent-sessions/%d/messages", pid, sid)
}

func TestChatResponsesToolLoop(t *testing.T) {
	f, _, pid, uid, sid, path := newChatFixture(t)
	for i := 0; i < 23; i++ {
		if _, err := f.server.appendAgentMessage(context.Background(), uid, int32(pid), sid, "user", fmt.Sprintf("history-%d", i), nil, "seed-history"); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != "/v1/responses" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret-ai-key" {
				t.Fatalf("protocol %s %s", r.Method, r.URL)
			}
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) < 100*time.Second {
				t.Fatal("chat inherited ordinary API timeout")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "model-test" || body["store"] != false || body["previous_response_id"] != nil || len(body["tools"].([]any)) != 6 {
				t.Fatalf("params %+v", body)
			}
			input := body["input"].([]any)
			if calls == 1 {
				if len(input) != 20 || input[0].(map[string]any)["content"] != "history-4" || input[19].(map[string]any)["content"] != "现在的态势" {
					t.Fatalf("history %+v", input)
				}
				output := []any{gin.H{"type": "reasoning", "id": "reasoning_test", "summary": []any{}, "encrypted_content": "encrypted-reasoning-test"}}
				for i, name := range []string{"query_devices", "query_tasks", "query_issues", "query_assets", "query_tracks", "query_map_context"} {
					output = append(output, chatFunction(name, fmt.Sprint(i), `{}`))
				}
				return chatResponse(r, output), nil
			}
			if calls != 2 {
				t.Fatalf("extra turn %d", calls)
			}
			found, reasoning := 0, false
			for _, raw := range input {
				item := raw.(map[string]any)
				if item["type"] == "reasoning" {
					reasoning = item["encrypted_content"] == "encrypted-reasoning-test"
				}
				if item["type"] != "function_call_output" {
					continue
				}
				found++
				var output map[string]any
				if err := json.Unmarshal([]byte(item["output"].(string)), &output); err != nil {
					t.Fatal(err)
				}
				if output["projectId"] != float64(pid) || output["quality"] != "authoritative-project-query" {
					t.Fatalf("tool result %+v", output)
				}
			}
			if found != 6 || !reasoning {
				t.Fatalf("continuation tools %d reasoning %v", found, reasoning)
			}
			return chatResponse(r, []any{chatText("当前没有运行中的任务。")}), nil
		})}
	}
	res := f.request(t, "POST", path, `{"content":"现在的态势"}`)
	result := decodedResponse(t, res)
	if res.StatusCode != 201 || result["content"] != "当前没有运行中的任务。" || result["modelId"] != "openai:model-test" || calls != 2 {
		t.Fatalf("chat %d %+v calls %d", res.StatusCode, result, calls)
	}
	var content string
	var raw []byte
	if err := f.db.QueryRow("select content,tool_calls_json from agent_messages where session_id=$1 and role='assistant' order by id desc limit 1", sid).Scan(&content, &raw); err != nil {
		t.Fatal(err)
	}
	var stored []map[string]any
	json.Unmarshal(raw, &stored)
	if len(stored) != 6 || strings.Contains(string(raw), "encrypted-reasoning") || strings.Contains(string(raw), "deviceCount") {
		t.Fatalf("retention %s", raw)
	}
	refs := stored[5]["evidenceRefs"].([]any)
	ref := refs[0].(map[string]any)
	if ref["type"] != "map-context" || ref["id"] != "current" || !strings.Contains(ref["href"].(string), "/projects/detail/?projectId=") {
		t.Fatalf("evidence %+v", ref)
	}
}

func TestChatStepLimitAndFailures(t *testing.T) {
	for _, mode := range []string{"eight-steps", "http-error", "oversized", "scope-injection", "unknown-tool", "revoked", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			f, team, _, _, sid, path := newChatFixture(t)
			calls := 0
			if mode == "timeout" {
				f.server.cfg.AIRequestTimeout = 100 * time.Millisecond
			}
			f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
				return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					switch mode {
					case "oversized":
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":"` + strings.Repeat("x", 5<<20) + `"}`)), Request: r}, nil
					case "http-error":
						return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"private-upstream-secret"}}`)), Request: r}, nil
					case "scope-injection":
						return chatResponse(r, []any{chatFunction("query_assets", "scope", `{"projectId":999}`)}), nil
					case "unknown-tool":
						return chatResponse(r, []any{chatFunction("device_command", "write", `{}`)}), nil
					case "revoked":
						if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
							t.Fatal(err)
						}
						return chatResponse(r, []any{chatFunction("query_map_context", "revoked", `{}`)}), nil
					case "timeout":
						<-r.Context().Done()
						return nil, r.Context().Err()
					default:
						return chatResponse(r, []any{chatFunction("query_map_context", fmt.Sprint(calls), `{}`)}), nil
					}
				})}
			}
			res := f.request(t, "POST", path, `{"content":"check"}`)
			result := decodedResponse(t, res)
			expected := 400
			if mode == "eight-steps" {
				expected = 201
			}
			if mode == "revoked" {
				expected = 403
			}
			if mode == "timeout" {
				expected = 504
			}
			if res.StatusCode != expected {
				t.Fatalf("status %d %+v", res.StatusCode, result)
			}
			var users, assistants int
			var content string
			if err := f.db.QueryRow("select count(*) filter(where role='user'),count(*) filter(where role='assistant'),coalesce(max(content) filter(where role='assistant'),'') from agent_messages where session_id=$1", sid).Scan(&users, &assistants, &content); err != nil {
				t.Fatal(err)
			}
			if mode == "eight-steps" {
				if calls != 8 || assistants != 1 || content != "未生成可用回复。" || result["content"] != "" {
					t.Fatalf("step limit calls %d assistant %d %+v", calls, assistants, result)
				}
			} else if calls != 1 || assistants != 0 {
				t.Fatalf("failed turn calls %d assistants %d", calls, assistants)
			}
			if users != 1 {
				t.Fatalf("user history %d", users)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "private-upstream-secret") {
				t.Fatal("upstream leaked")
			}
		})
	}
}

func TestChatInvalidInputAndUnavailableProvider(t *testing.T) {
	f, _, _, _, sid, path := newChatFixture(t)
	count := func() int {
		t.Helper()
		var n int
		if err := f.db.QueryRow("select count(*) from agent_messages where session_id=$1", sid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, body := range []string{`{}`, `null`, `{"content":" "}`, `{"content":1}`, `{"content":null}`} {
		res := f.request(t, "POST", path, body)
		result := decodedResponse(t, res)
		if res.StatusCode != 400 || result["error"] != "AGENT_MESSAGE_INVALID" || count() != 0 {
			t.Fatalf("input %d %+v", res.StatusCode, result)
		}
	}
	if _, err := f.db.Exec("update ai_providers set is_default=false,enabled=false"); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "POST", path, `{"content":"hello"}`)
	result := decodedResponse(t, res)
	if res.StatusCode != 400 || result["error"] != "AI_PROVIDER_UNAVAILABLE" || count() != 1 {
		t.Fatalf("provider %d %+v", res.StatusCode, result)
	}
	if _, err := f.db.Exec("update agent_sessions set status='closed' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, `{"content":"closed"}`)
	result = decodedResponse(t, res)
	if res.StatusCode != 404 || result["error"] != "AGENT_SESSION_NOT_FOUND" || count() != 1 {
		t.Fatalf("closed %d %+v", res.StatusCode, result)
	}
}
