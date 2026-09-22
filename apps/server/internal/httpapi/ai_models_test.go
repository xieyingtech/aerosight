package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

func TestAIPrivateEndpoints(t *testing.T) {
	s := &Server{networkResolver: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.168.1.10")}, nil
	}}
	for _, endpoint := range []string{"http://127.0.0.1:8000/v1", "http://10.0.0.2/v1", "https://192.168.1.2/v1", "http://[::1]:8000/v1", "http://inference.internal/v1"} {
		if _, _, err := s.resolveAIURL(context.Background(), endpoint); err != nil {
			t.Fatalf("%s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"file:///etc/passwd", "ftp://localhost", "http://key@localhost", "http://localhost/#fragment"} {
		if _, _, err := s.resolveAIURL(context.Background(), endpoint); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	// The AI-specific change must not weaken the algorithm provider policy.
	if _, _, err := s.resolveOutboundURL(context.Background(), "https://192.168.1.2", []string{"192.168.1.2"}); err == nil {
		t.Fatal("algorithm policy changed")
	}
}

func TestAIProviderLocalTextAndRealtimeRuntime(t *testing.T) {
	f := newAPIFixture(t)
	f.server.credentialSecret = "local-runtime-test-secret"
	textUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("wrong text endpoint %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "local-text" {
			t.Errorf("wrong text model %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"local-response","object":"response","status":"completed","output":[]}`))
	}))
	defer textUpstream.Close()
	voiceUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("model") != "local-voice" {
			t.Errorf("wrong voice endpoint %s", r.URL)
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		conn.WriteJSON(map[string]string{"type": "session.created"})
	}))
	defer voiceUpstream.Close()
	for _, body := range []string{
		`{"name":"text","providerType":"openai","baseUrl":"` + textUpstream.URL + `/v1","enabled":true,"isDefault":true,"modelId":"local-text","models":[{"id":"local-text","protocol":"responses","capabilities":["text"],"enabled":true}]}`,
		`{"name":"voice","providerType":"openai","baseUrl":"` + voiceUpstream.URL + `/v1","enabled":true,"isRealtimeDefault":true,"realtimeProtocol":"stepfun","realtimeModelId":"local-voice","models":[{"id":"local-voice","protocol":"stepfun-realtime","capabilities":["realtime"],"enabled":true}]}`,
	} {
		res := f.request(t, "POST", "/api/admin/ai-providers", body)
		result := decodedResponse(t, res)
		if res.StatusCode != 201 {
			t.Fatalf("create %v", result)
		}
	}
	sdk, model, cleanup, err := f.server.configuredChatClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	response, err := sdk.Responses.New(context.Background(), responses.ResponseNewParams{Model: model, Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("hello")}})
	if err != nil || response.ID != "local-response" {
		t.Fatalf("local text %v %v", response, err)
	}
	conn, config, err := f.server.connectRealtime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var event map[string]string
	if err := conn.ReadJSON(&event); err != nil || event["type"] != "session.created" || config.Model != "local-voice" {
		t.Fatalf("local voice %v %v", event, err)
	}
}

func TestAIModelCatalogValidation(t *testing.T) {
	valid := `{"name":"LAN","providerType":"openai","baseUrl":"http://localhost:8000/v1","enabled":true,"models":[{"id":"voice","protocol":"stepfun-realtime","capabilities":["realtime","audio-input","audio-output"],"enabled":true}],"realtimeProtocol":"stepfun","realtimeModelId":"voice","isRealtimeDefault":true}`
	var raw map[string]any
	json.Unmarshal([]byte(valid), &raw)
	p, err := parseAIProvider(raw)
	if err != nil || !p.IsRealtimeDefault || p.Params.IsDefault || len(p.Models) != 1 {
		t.Fatalf("voice-only provider %+v %v", p, err)
	}
	for _, body := range []string{
		strings.Replace(valid, `"id":"voice"`, `"id":"other"`, 1),
		strings.Replace(valid, `"enabled":true`, `"enabled":false`, 1),
		strings.Replace(valid, `"protocol":"stepfun-realtime"`, `"protocol":"unknown"`, 1),
		strings.Replace(valid, `"models":[`, `"modelId":123,"models":[`, 1),
	} {
		raw = nil
		json.Unmarshal([]byte(body), &raw)
		if _, err := parseAIProvider(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	raw = nil
	json.Unmarshal([]byte(`{"name":"Empty","providerType":"openai","models":[]}`), &raw)
	if _, err := parseAIProvider(raw); err != nil {
		t.Fatalf("empty draft: %v", err)
	}
}

func TestAIProviderIndependentModelsAndDiscovery(t *testing.T) {
	f := newAPIFixture(t)
	f.server.credentialSecret = "model-catalog-test-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"voice"},{"id":"text"},{"id":"voice"}]}`))
	}))
	defer upstream.Close()
	call := func(method, path, body string, want int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d %+v", method, path, res.StatusCode, data)
		}
		return data
	}
	const path = "/api/admin/ai-providers"
	text := call("POST", path, aiProviderBody, 201)
	body := `{"name":"Local voice","providerType":"openai","baseUrl":"` + upstream.URL + `/v1","enabled":true,"models":[{"id":"voice","protocol":"stepfun-realtime","capabilities":["realtime"],"enabled":true}],"realtimeProtocol":"stepfun","realtimeModelId":"voice","isRealtimeDefault":true}`
	voice := call("POST", path, body, 201)
	if voice["isDefault"] != false || voice["isRealtimeDefault"] != true {
		t.Fatalf("defaults %+v", voice)
	}
	chat, err := f.server.queries.ReadDefaultChatProvider(context.Background())
	if err != nil || len(chat) != 1 || chat[0].ModelID != "model-test" {
		t.Fatalf("text default %+v %v", chat, err)
	}
	realtime, err := f.server.queries.ReadDefaultRealtimeProvider(context.Background())
	if err != nil || len(realtime) != 1 || realtime[0].RealtimeModelID != "voice" {
		t.Fatalf("voice default %+v %v", realtime, err)
	}
	discovered := call("POST", path+"/models", `{"providerId":"`+voice["id"].(string)+`","baseUrl":"`+upstream.URL+`/v1"}`, 200)
	if len(discovered["models"].([]any)) != 2 {
		t.Fatalf("models %+v", discovered)
	}
	// Unsaved providers can discover models without a key on local no-auth services.
	call("POST", path+"/models", `{"baseUrl":"`+upstream.URL+`/v1"}`, 200)
	call("POST", path+"/models", `{"providerId":"`+text["id"].(string)+`","baseUrl":"`+upstream.URL+`/v1"}`, 400)
	health := call("POST", path+"/"+voice["id"].(string)+"/test", "", 200)
	if health["ok"] != true {
		t.Fatalf("LAN health %+v", health)
	}
	// Replacing a realtime default must leave the text default intact.
	second := call("POST", path, strings.Replace(body, "Local voice", "Second voice", 1), 201)
	realtime, err = f.server.queries.ReadDefaultRealtimeProvider(context.Background())
	if err != nil || len(realtime) != 1 || realtime[0].ID == chat[0].ID {
		t.Fatalf("replacement %+v %v", realtime, err)
	}
	disabled := strings.Replace(body, "Local voice", "Second voice", 1)
	disabled = strings.Replace(disabled, `"enabled":true`, `"enabled":false`, 1)
	disabled = strings.Replace(disabled, `"isRealtimeDefault":true`, `"isRealtimeDefault":false`, 1)
	call("PATCH", path+"/"+second["id"].(string), disabled, 200)
	realtime, err = f.server.queries.ReadDefaultRealtimeProvider(context.Background())
	if err != nil || len(realtime) != 0 {
		t.Fatal("disabled voice still routed")
	}
	chat, err = f.server.queries.ReadDefaultChatProvider(context.Background())
	if err != nil || len(chat) != 1 {
		t.Fatal("disabling voice changed text default")
	}
	if _, err := f.db.Exec("update users set role='user' where email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	call("POST", path+"/models", `{"baseUrl":"`+upstream.URL+`/v1"}`, 403)
	call("POST", path, body, 403)
	call("PATCH", path+"/"+voice["id"].(string), body, 403)
}
