package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestRealtimeStepEndpoint(t *testing.T) {
	for _, base := range []string{"https://api.stepfun.com/v1", "https://api.stepfun.com/step_plan/v1/", "https://api.stepfun.ai/v1", "https://gateway.example/realtime-api/v1"} {
		u, err := realtimeURL(base, "custom-realtime-v3 &voice")
		if err != nil || u.Query().Get("model") != "custom-realtime-v3 &voice" {
			t.Fatalf("endpoint %s %v", base, err)
		}
	}
	for _, base := range []string{"http://api.stepfun.com/v1", "https://secret@api.stepfun.com/v1", "https://api.stepfun.com/v1?x=1"} {
		if _, err := realtimeURL(base, "custom-realtime-v3 &voice"); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	tools := stepRealtimeSession()["tools"].([]gin.H)
	if len(tools) != 1 || tools[0]["function"] == nil || tools[0]["name"] != nil {
		t.Fatal("StepFun requires nested function definitions")
	}
}

func TestRealtimeDispatcherDoesNotAcceptScopeOrWriteTools(t *testing.T) {
	for resource, name := range realtimeResources {
		actual, err := realtimeReadTool(stepRealtimeItem{Name: "query_project", Arguments: `{"resource":"` + resource + `"}`})
		if err != nil || actual != name {
			t.Fatalf("resource %s: %s %v", resource, actual, err)
		}
	}
	for _, args := range []string{`{"resource":"devices","projectId":2}`, `{"resource":"device.command"}`, `null`, `{}`, `{"resource":5}`} {
		if _, err := realtimeReadTool(stepRealtimeItem{Name: "query_project", Arguments: args}); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
	if _, err := realtimeReadTool(stepRealtimeItem{Name: "execute_command", Arguments: `{}`}); err == nil {
		t.Fatal("accepted write tool")
	}
}

func TestRealtimeWebSocketTranscriptToolsAndPersistence(t *testing.T) {
	f, _, _, _, sid, path := newChatFixture(t)
	path = strings.TrimSuffix(path, "messages") + "realtime"
	if _, err := f.db.Exec("update ai_providers set base_url='https://api.stepfun.com/v1',realtime_protocol='stepfun',realtime_model_id='configured-voice-model'"); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_ = conn.WriteJSON(gin.H{"type": "session.created"})
		var setup map[string]any
		if err = conn.ReadJSON(&setup); err != nil {
			t.Error(err)
			return
		}
		if setup["type"] != "session.update" {
			t.Error("missing setup")
			return
		}
		_ = conn.WriteJSON(gin.H{"type": "session.updated"})
		var greeting map[string]any
		if err = conn.ReadJSON(&greeting); err != nil || greeting["type"] != "response.create" {
			t.Errorf("missing greeting: %v %v", greeting, err)
			return
		}
		for _, event := range []gin.H{
			{"type": "input_audio_buffer.speech_started", "item_id": "user1"},
			{"type": "response.output_item.added", "item": gin.H{"id": "assistant1", "type": "message", "role": "assistant"}},
			{"type": "response.audio_transcript.delta", "item_id": "assistant1", "delta": "先查询设备。"},
			{"type": "response.audio_transcript.done", "item_id": "assistant1", "transcript": "先查询设备。"},
			// Deliberately late ASR: history must still put the user first.
			{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "检查设备"},
			{"type": "response.output_item.done", "item": gin.H{"id": "tool1", "type": "function_call", "call_id": "call1", "name": "query_project", "arguments": `{"resource":"devices"}`}},
		} {
			_ = conn.WriteJSON(event)
		}
		var result map[string]any
		if err = conn.ReadJSON(&result); err != nil {
			t.Error(err)
			return
		}
		item, _ := result["item"].(map[string]any)
		if item["type"] != "function_call_output" || item["call_id"] != "call1" {
			t.Errorf("result %+v", result)
			return
		}
		// A duplicate completion must not execute the tool again.
		_ = conn.WriteJSON(gin.H{"type": "response.output_item.done", "item": gin.H{"id": "tool1", "type": "function_call", "call_id": "call1", "name": "query_project", "arguments": `{"resource":"devices"}`}})
		_ = conn.WriteJSON(gin.H{"type": "response.done", "response": gin.H{"status": "completed"}})
		if err = conn.ReadJSON(&result); err != nil {
			t.Error(err)
			return
		}
		if result["type"] != "response.create" {
			t.Error("missing continuation")
			return
		}
		_ = conn.WriteJSON(gin.H{"type": "response.audio_transcript.done", "item_id": "final1", "transcript": "没有设备。"})
		// Remain connected until the application closes this upstream socket.
		_, _, _ = conn.ReadMessage()
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
	conn, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.host.URL, "http")+path, headers)
	if err != nil {
		t.Fatalf("upgrade %v %v", res, err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var seenRunning, seenSucceeded bool
	for {
		var event struct {
			Type string
			Data json.RawMessage
		}
		if err = conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "ready" {
			var ready struct{ Model string }
			_ = json.Unmarshal(event.Data, &ready)
			if ready.Model != "configured-voice-model" {
				t.Fatalf("model not from provider: %s", ready.Model)
			}
		}
		if event.Type == "error" {
			t.Fatalf("error %s", event.Data)
		}
		if event.Type == "message" {
			var m realtimeMessage
			_ = json.Unmarshal(event.Data, &m)
			for _, tool := range m.ToolCalls {
				if tool["status"] == "running" {
					seenRunning = true
				}
				if tool["status"] == "succeeded" {
					seenSucceeded = true
				}
			}
			if m.Content == "没有设备。" {
				_ = conn.WriteJSON(gin.H{"type": "stop"})
			}
		}
		if event.Type == "done" {
			break
		}
	}
	if !seenRunning || !seenSucceeded {
		t.Fatal("missing inline tool lifecycle")
	}
	rows, err := f.db.Query("select role,content from agent_messages where session_id=$1 order by id", sid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var role, text string
		rows.Scan(&role, &text)
		texts = append(texts, role+":"+text)
	}
	if strings.Join(texts, "|") != "user:检查设备|assistant:先查询设备。|assistant:|assistant:没有设备。" {
		t.Fatalf("history %v", texts)
	}
}

func TestRealtimeRejectsCrossOrigin(t *testing.T) {
	f, _, _, _, _, path := newChatFixture(t)
	req, _ := http.NewRequest("GET", f.host.URL+strings.TrimSuffix(path, "messages")+"realtime", nil)
	req.Header.Set("Origin", "https://evil.test")
	res, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestRealtimeUnconfiguredProviderDoesNotDial(t *testing.T) {
	f, _, _, _, _, _ := newChatFixture(t)
	f.server.realtimeConnect = func(context.Context) (*websocket.Conn, error) {
		t.Fatal("unconfigured provider was dialed")
		return nil, nil
	}
	_, _, err := f.server.connectRealtime(context.Background())
	if err == nil || err.Error() != "AI_REALTIME_PROVIDER_REQUIRED" {
		t.Fatalf("missing configuration: %v", err)
	}
}

func TestRealtimeWelcomesOnceAfterSessionConfigured(t *testing.T) {
	writes, ready := 0, 0
	r := &realtimeConversation{config: realtimeConfig{Model: "configured-model"},
		write: func(value any) error {
			writes++
			event := value.(gin.H)
			if event["type"] != "response.create" || event["response"].(gin.H)["tool_choice"] != "none" {
				t.Fatalf("invalid greeting %v", event)
			}
			return nil
		},
		emit: func(kind string, value any) error {
			if kind == "ready" {
				ready++
			}
			return nil
		},
	}
	for i := 0; i < 2; i++ {
		if err := r.handle(context.Background(), stepRealtimeEvent{Type: "session.updated"}); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 || ready != 1 {
		t.Fatalf("duplicate welcome: %d ready: %d", writes, ready)
	}
}
