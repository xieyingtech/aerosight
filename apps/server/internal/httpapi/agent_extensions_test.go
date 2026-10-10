package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAgentExtensionsRealMCP(t *testing.T) {
	f, team, pid, uid, sid, path := newChatFixture(t)
	ctx := context.Background()
	stubApprovalFollowup(t, f, "外部工具已返回结果。")
	call := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		raw := ""
		if body != nil {
			encoded, _ := json.Marshal(body)
			raw = string(encoded)
		}
		res := f.request(t, method, path, raw)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%s %s: %d %v", method, path, res.StatusCode, data)
		}
		return data
	}
	const base = "/api/admin/ai-providers"
	skillBody := gin.H{"slug": "material-review", "name": "素材复核", "description": "检查真实素材", "body": "先搜索素材，再核对原片；回答包含复核步骤。", "enabled": true}
	skill := call("POST", base+"/skills", skillBody, 200)
	skillID := skill["id"]
	result, err := f.server.loadPlatformSkill(ctx, uid, int32(pid), []byte(`{"skillName":"material-review"}`))
	if err != nil || !strings.Contains(fmt.Sprint(result), "核对原片") {
		t.Fatal(result, err)
	}
	if _, err = f.server.listAgentSkills(ctx, uid, int32(pid+100)); err == nil {
		t.Fatal("cross-project skill access")
	}
	// A real Responses tool loop receives the configured Markdown, not just a UI record.
	modelCalls := 0
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			modelCalls++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if modelCalls == 1 {
				return chatResponse(r, []any{chatFunction("list_skills", "catalog", `{}`), chatFunction("load_skill", "skill", `{"skillName":"material-review"}`)}), nil
			}
			raw, _ := json.Marshal(body["input"])
			if !strings.Contains(string(raw), "核对原片") {
				t.Error("skill body missing from model input")
			}
			return chatResponse(r, []any{chatText("按 Skill 复核原片。")}), nil
		})}
	}
	call("POST", path, gin.H{"content": "按素材复核技能处理"}, 201)
	if modelCalls != 2 {
		t.Fatal("skill loop", modelCalls)
	}
	skillBody["revision"] = 1
	skillBody["enabled"] = false
	call("PATCH", fmt.Sprintf("%s/skills/%v", base, skillID), skillBody, 200)
	if _, err = f.server.loadPlatformSkill(ctx, uid, int32(pid), []byte(`{"skillName":"material-review"}`)); err == nil {
		t.Fatal("disabled skill loaded")
	}
	skillBody["revision"] = 1
	call("PATCH", fmt.Sprintf("%s/skills/%v", base, skillID), skillBody, 404)

	remote := mcp.NewServer(&mcp.Implementation{Name: "local-test", Version: "1"}, nil)
	var executions atomic.Int32
	var failAfterDispatch atomic.Bool
	var headersOK atomic.Bool
	headersOK.Store(true)
	const bearer = "local-mcp-test-token"
	schema := map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "minLength": 1}}, "required": []string{"text"}, "additionalProperties": false}
	handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		executions.Add(1)
		if failAfterDispatch.Load() {
			return nil, errors.New("external outcome unknown")
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "真实工具结果 " + bearer}}, StructuredContent: map[string]any{"value": "ok", "token": bearer}}, nil
	}
	remote.AddTool(&mcp.Tool{Name: "echo", Description: "返回输入，供本地协议验证", InputSchema: schema}, handler)
	endpoint := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+bearer {
			headersOK.Store(false)
		}
		if project := r.Header.Get("X-AeroSight-Project-Id"); project != "" && (project != fmt.Sprint(pid) || r.Header.Get("X-AeroSight-User-Id") != fmt.Sprint(uid)) {
			headersOK.Store(false)
		}
		endpoint.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	config := gin.H{"name": "本地 MCP", "endpoint": upstream.URL, "bearer": bearer, "enabled": true}
	created := call("POST", base+"/mcp", config, 200)
	id := int64(created["id"].(float64))
	item := fmt.Sprintf("%s/mcp/%d", base, id)
	var encrypted string
	if err = f.db.QueryRow(`SELECT credential_envelope_json::text FROM agent_mcp_servers WHERE id=$1`, id).Scan(&encrypted); err != nil || strings.Contains(encrypted, bearer) {
		t.Fatal("credential not encrypted", err)
	}
	call("POST", item+"/discover", nil, 200)
	current, err := readPlatformMCP(ctx, f.db, id, false)
	if err != nil || len(current.Tools) != 1 || current.Tools[0].Policy != "disabled" {
		t.Fatal(current, err)
	}
	res := f.request(t, "GET", base+"/mcp", "")
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(raw), bearer) || strings.Contains(string(raw), "credentialEnvelope") {
		t.Fatal("public credential leak")
	}
	invokeRaw := func(args any) json.RawMessage {
		raw, _ := json.Marshal(gin.H{"serverId": id, "toolName": "echo", "arguments": args})
		return raw
	}
	invoke := func() (gin.H, error) {
		return f.server.executeMCPTool(ctx, uid, int32(pid), sid, invokeRaw(gin.H{"text": "test"}), "mcp-test")
	}
	if _, err = invoke(); err == nil || executions.Load() != 0 {
		t.Fatal("default disabled tool called")
	}
	savePolicy := func(policy string) {
		t.Helper()
		row, e := readPlatformMCP(ctx, f.db, id, false)
		if e != nil {
			t.Fatal(e)
		}
		call("PATCH", item, gin.H{"name": row.Name, "endpoint": row.Endpoint, "enabled": true, "revision": row.Revision, "tools": []gin.H{{"name": "echo", "policy": policy}}}, 200)
	}
	savePolicy("readonly")
	result, err = invoke()
	if err != nil || executions.Load() != 1 || strings.Contains(fmt.Sprint(result), bearer) {
		t.Fatal("readonly/redaction", result, err)
	}
	if !headersOK.Load() {
		t.Fatal("headers incorrect")
	}
	for _, args := range []any{gin.H{"text": 4}, gin.H{"text": ""}, gin.H{"text": "x", "projectId": pid}, gin.H{"text": "x", "extra": true}} {
		if _, err = f.server.executeMCPTool(ctx, uid, int32(pid), sid, invokeRaw(args), "invalid"); err == nil {
			t.Fatal("invalid input accepted", args)
		}
	}
	if executions.Load() != 1 {
		t.Fatal("invalid input called tool")
	}
	// Text Agent calls a real MCP and receives its bounded, redacted output.
	modelCalls = 0
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			modelCalls++
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if modelCalls == 1 {
				return chatResponse(r, []any{chatFunction("list_mcp_tools", "tools", `{}`), chatFunction("call_mcp_tool", "echo", string(invokeRaw(gin.H{"text": "test"})))}), nil
			}
			encoded, _ := json.Marshal(body["input"])
			if !strings.Contains(string(encoded), "真实工具结果") || strings.Contains(string(encoded), bearer) {
				t.Error("MCP result missing or leaked")
			}
			return chatResponse(r, []any{chatText("本地 MCP 返回成功。")}), nil
		})}
	}
	call("POST", path, gin.H{"content": "调用本地工具"}, 201)
	if modelCalls != 2 || executions.Load() != 2 {
		t.Fatal("MCP text loop")
	}
	verifyRealtimeExtensions(t, f, pid, sid, string(invokeRaw(gin.H{"text": "realtime"})))
	if executions.Load() != 3 {
		t.Fatal("realtime MCP tool did not execute")
	}
	stubApprovalFollowup(t, f, "外部工具已返回结果。")
	savePolicy("approval")
	result, err = invoke()
	if err != nil || result["status"] != "confirmation_required" || executions.Load() != 3 {
		t.Fatal("executed before click", result, err)
	}
	approvalPath := func(id any) string {
		return fmt.Sprintf("/api/projects/%d/agent-sessions/%d/approvals/%v", pid, sid, id)
	}
	decision := call("POST", approvalPath(result["approvalId"]), gin.H{"decision": "approve"}, 200)
	if decision["status"] != "succeeded" || executions.Load() != 4 {
		t.Fatal("approval", decision)
	}
	call("POST", approvalPath(result["approvalId"]), gin.H{"decision": "approve"}, 409)
	if executions.Load() != 4 {
		t.Fatal("duplicate executed")
	}
	stale, err := invoke()
	if err != nil {
		t.Fatal(err)
	}
	savePolicy("approval")
	call("POST", approvalPath(stale["approvalId"]), gin.H{"decision": "approve"}, 400)
	if executions.Load() != 4 {
		t.Fatal("old config executed")
	}
	uncertain, err := invoke()
	if err != nil {
		t.Fatal(err)
	}
	failAfterDispatch.Store(true)
	decision = call("POST", approvalPath(uncertain["approvalId"]), gin.H{"decision": "approve"}, 200)
	if decision["status"] != "executing" || executions.Load() != 5 {
		t.Fatal("uncertain outcome retriable", decision)
	}
	call("POST", approvalPath(uncertain["approvalId"]), gin.H{"decision": "approve"}, 409)
	failAfterDispatch.Store(false)
	revoked, err := invoke()
	if err != nil {
		t.Fatal(err)
	}
	var memberRole string
	if err = f.db.QueryRow(`SELECT role FROM team_members WHERE team_id=$1 AND user_id=$2`, team, uid).Scan(&memberRole); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, team, uid); err != nil {
		t.Fatal(err)
	}
	call("POST", approvalPath(revoked["approvalId"]), gin.H{"decision": "approve"}, 403)
	if executions.Load() != 5 {
		t.Fatal("revoked permission executed")
	}
	if _, err = f.db.Exec(`INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,$3)`, team, uid, memberRole); err != nil {
		t.Fatal(err)
	}
	remote.AddTool(&mcp.Tool{Name: "echo", Description: "schema drift", InputSchema: schema}, handler)
	if _, err = invoke(); err == nil {
		t.Fatal("schema drift accepted")
	}
	if executions.Load() != 5 {
		t.Fatal("drift executed")
	}
	current, _ = readPlatformMCP(ctx, f.db, id, false)
	call("PATCH", item, gin.H{"name": "changed", "endpoint": upstream.URL + "/new", "enabled": true, "revision": current.Revision}, 400)
	call("PATCH", item, gin.H{"name": "changed", "endpoint": upstream.URL + "/new", "bearer": "", "enabled": true, "revision": current.Revision}, 200)
	current, _ = readPlatformMCP(ctx, f.db, id, false)
	if current.HasCredential || len(current.Tools) != 0 {
		t.Fatal("endpoint kept token/tools")
	}
	call("DELETE", item, nil, 200)
	if _, err = invoke(); err == nil {
		t.Fatal("deleted MCP available")
	}
	call("DELETE", fmt.Sprintf("%s/skills/%v", base, skillID), nil, 200)
	// Project admins do not gain platform administration.
	if _, err = f.db.Exec(`UPDATE users SET role='user' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	call("POST", base+"/skills", skillBody, 403)
	res = f.request(t, "GET", base+"/mcp", "")
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("non-admin MCP registry")
	}
}

func verifyRealtimeExtensions(t *testing.T, f *apiFixture, pid int, sid int32, mcpArgs string) {
	t.Helper()
	if _, err := f.db.Exec(`UPDATE ai_providers SET base_url='https://api.stepfun.com/v1',realtime_protocol='stepfun',realtime_model_id='local-voice',is_realtime_default=true`); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		conn.WriteJSON(gin.H{"type": "session.created"})
		var input gin.H
		if conn.ReadJSON(&input) != nil || input["type"] != "session.update" {
			t.Error("realtime setup")
			return
		}
		conn.WriteJSON(gin.H{"type": "session.updated"})
		if conn.ReadJSON(&input) != nil {
			t.Error("greeting")
			return
		}
		for i, call := range []struct{ name, args, expect string }{{"list_skills", `{}`, objectSkillName}, {"load_skill", `{"skillName":"inspection-object-query"}`, "instructions"}, {"list_mcp_tools", `{}`, "echo"}, {"call_mcp_tool", mcpArgs, "真实工具结果"}} {
			id := fmt.Sprint(i)
			conn.WriteJSON(gin.H{"type": "response.output_item.done", "item": gin.H{"id": "ext-" + id, "type": "function_call", "call_id": "ext-" + id, "name": call.name, "arguments": call.args}})
			if err = conn.ReadJSON(&input); err != nil {
				t.Error(err)
				return
			}
			item, _ := input["item"].(map[string]any)
			if item == nil || !strings.Contains(fmt.Sprint(item["output"]), call.expect) {
				t.Errorf("realtime output %s: %v", call.name, input)
				return
			}
			conn.WriteJSON(gin.H{"type": "response.done", "response": gin.H{"status": "completed"}})
			if conn.ReadJSON(&input) != nil || input["type"] != "response.create" {
				t.Error("continuation")
				return
			}
		}
		conn.WriteJSON(gin.H{"type": "response.audio_transcript.done", "item_id": "extension-final", "transcript": "扩展调用验收完成。"})
		conn.ReadMessage()
	}))
	defer upstream.Close()
	f.server.realtimeConnect = func(ctx context.Context) (*websocket.Conn, error) {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(upstream.URL, "http"), nil)
		return conn, err
	}
	u, _ := url.Parse(f.host.URL)
	headers := http.Header{"Origin": {"http://frontend.test"}}
	for _, cookie := range f.client.Jar.Cookies(u) {
		headers.Add("Cookie", cookie.String())
	}
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws%s/api/projects/%d/agent-sessions/%d/realtime", strings.TrimPrefix(f.host.URL, "http"), pid, sid), headers)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	for {
		var event struct {
			Type string
			Data json.RawMessage
		}
		if err = conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "error" {
			t.Fatalf("realtime %s", event.Data)
		}
		if event.Type == "message" {
			var message realtimeMessage
			json.Unmarshal(event.Data, &message)
			if message.Content == "扩展调用验收完成。" {
				conn.WriteJSON(gin.H{"type": "stop"})
			}
		}
		if event.Type == "done" {
			break
		}
	}
}

func TestAgentExtensionEndpointAndSchemaBoundaries(t *testing.T) {
	for _, raw := range []string{"https://user:secret@example.com/mcp", "https://example.com/mcp?token=x", "https://example.com/mcp#x", "file:///tmp/mcp"} {
		if validateMCPEndpoint(raw) == nil {
			t.Fatal(raw)
		}
	}
	if validateMCPEndpoint("http://127.0.0.1:8000/mcp") != nil {
		t.Fatal("local endpoint rejected")
	}
	for _, raw := range []string{`{"type":"object","$ref":"https://example.com/schema"}`, `{"type":"object","properties":{"bad":4}}`} {
		if _, err := resolveMCPInputSchema([]byte(raw)); err == nil {
			t.Fatal("unsafe schema", raw)
		}
	}
}
