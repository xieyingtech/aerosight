package httpapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGlobalAIModelDefaults(t *testing.T) {
	f := newAPIFixture(t)
	f.server.credentialSecret = "global-default-test-secret"
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d %+v", method, path, res.StatusCode, out)
		}
		return out
	}
	const path = "/api/admin/ai-providers"
	body := map[string]any{"name": "Text provider", "providerType": "openai", "enabled": true, "models": []map[string]any{{"id": "compat", "protocol": "openai-compatible"}, {"id": "responses", "protocol": "responses"}, {"id": "claude", "protocol": "anthropic-messages"}}}
	text := call("POST", path, body, 201)
	voice := call("POST", path, map[string]any{"name": "Voice provider", "providerType": "openai", "baseUrl": "http://127.0.0.1:9000/v1", "enabled": true, "models": []map[string]any{{"id": "voice", "protocol": "stepfun-realtime"}}}, 201)
	for _, model := range []string{"compat", "claude", "responses"} {
		call("PUT", path+"/defaults", map[string]any{"kind": "text", "providerId": text["id"], "modelId": model}, 200)
	}
	call("PUT", path+"/defaults", map[string]any{"kind": "realtime", "providerId": voice["id"], "modelId": "voice"}, 200)
	call("PUT", path+"/defaults", map[string]any{"kind": "text", "providerId": voice["id"], "modelId": "voice"}, 400)
	call("PUT", path+"/defaults", map[string]any{"kind": "text", "providerId": text["id"], "modelId": "missing"}, 400)
	body["name"] = "Renamed"
	edited := call("PATCH", path+"/"+text["id"].(string), body, 200)
	if edited["isDefault"] != true || edited["modelId"] != "responses" {
		t.Fatal("provider edit lost global default")
	}
	models := edited["models"].([]any)
	if len(models[0].(map[string]any)["capabilities"].([]any)) != 8 {
		t.Fatal("capabilities not implicit")
	}
	body["models"] = []map[string]any{{"id": "compat", "protocol": "openai-compatible"}}
	edited = call("PATCH", path+"/"+text["id"].(string), body, 200)
	if edited["isDefault"] != false {
		t.Fatal("removed model still default")
	}
	realtime, err := f.server.queries.ReadDefaultRealtimeProvider(context.Background())
	if err != nil || len(realtime) != 1 {
		t.Fatal("text edit changed voice default")
	}
	body["models"] = []map[string]any{{"id": "unspecified", "protocol": ""}}
	call("PATCH", path+"/"+text["id"].(string), body, 400)
	if _, err := f.db.Exec("update users set role='user' where email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	call("PUT", path+"/defaults", map[string]any{"kind": "text", "providerId": text["id"], "modelId": "compat"}, 403)
}
