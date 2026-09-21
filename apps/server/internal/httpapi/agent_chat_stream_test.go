package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func streamedFixture(r *http.Request, output []any, delta string) *http.Response {
	var body strings.Builder
	if delta != "" {
		event, _ := json.Marshal(gin.H{"type": "response.output_text.delta", "delta": delta})
		fmt.Fprintf(&body, "data: %s\n\n", event)
	}
	event, _ := json.Marshal(gin.H{"type": "response.completed", "response": gin.H{"id": "resp_stream", "object": "response", "status": "completed", "output": output}})
	fmt.Fprintf(&body, "data: %s\n\n", event)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body.String())), Request: r}
}

func TestChatStreamToolBeforeFinalAndPersistedOrder(t *testing.T) {
	f, _, _, _, sid, path := newChatFixture(t)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	calls := 0
	f.server.aiHTTPClientFactory = func(*url.URL, []netip.Addr) *http.Client {
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			var params map[string]any
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				return nil, err
			}
			if params["stream"] != true {
				t.Error("upstream must actually stream")
			}
			if calls == 1 {
				return streamedFixture(r, []any{chatText("先检查设备。"), chatFunction("query_devices", "call_devices", `{}`)}, "先检查设备。"), nil
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return streamedFixture(r, []any{chatText("没有设备。")}, "没有设备。"), nil
		})}
	}
	req, _ := http.NewRequest("POST", f.host.URL+path, strings.NewReader(`{"content":"检查设备"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://frontend.test")
	req.Header.Set("X-CSRF-Token", f.csrf)
	req.Header.Set("Accept", "application/x-ndjson")
	res, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	scanner := bufio.NewScanner(res.Body)
	var order []string
	for scanner.Scan() {
		var event struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		order = append(order, event.Type)
		if event.Type == "error" {
			t.Fatalf("stream error %v", event.Data)
		}
		if event.Type == "tool" && event.Data["status"] == "succeeded" {
			close(release)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "status,step,text,tool,tool,status,step,text,done" {
		t.Fatalf("order: %v", order)
	}
	rows, err := f.db.QueryContext(context.Background(), "select content,tool_calls_json from agent_messages where session_id=$1 and role='assistant' order by id", sid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var content string
		var tools []byte
		if err := rows.Scan(&content, &tools); err != nil {
			t.Fatal(err)
		}
		texts = append(texts, content)
		if len(texts) == 1 && !strings.Contains(string(tools), "query_devices") {
			t.Fatal("tool history missing")
		}
	}
	if strings.Join(texts, "|") != "先检查设备。|没有设备。" {
		t.Fatalf("history %v", texts)
	}
}

func TestChatResponseStreamRejectsTruncation(t *testing.T) {
	sdk := openai.NewClient(option.WithAPIKey("test"), option.WithHTTPClient(&http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")), Request: r}, nil
	})}))
	var text string
	_, err := streamChatResponse(context.Background(), sdk, responses.ResponseNewParams{}, func(delta string) { text += delta })
	if err == nil || text != "partial" {
		t.Fatalf("truncated stream accepted: %q %v", text, err)
	}
}

func TestChatResponseStreamCancellation(t *testing.T) {
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sdk := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(upstream.URL), option.WithMaxRetries(0))
	_, err := streamChatResponse(ctx, sdk, responses.ResponseNewParams{}, func(string) { cancel() })
	if err == nil {
		t.Fatal("cancelled stream succeeded")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream not cancelled")
	}
}
