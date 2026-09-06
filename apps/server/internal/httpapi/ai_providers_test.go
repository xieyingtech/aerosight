package httpapi

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

const aiProviderBody = `{"name":" Primary ","providerType":"openai","modelId":" model-test ","apiKey":" secret-ai-key ","enabled":true,"isDefault":true}`

func TestAIProviderPolicy(t *testing.T) {
	var raw map[string]any
	json.Unmarshal([]byte(aiProviderBody), &raw)
	p, err := parseAIProvider(raw)
	if err != nil || p.APIKey != "secret-ai-key" || p.Params.Name != "Primary" || p.Params.ModelID != "model-test" || p.Audit["apiKey"] != nil {
		t.Fatalf("policy %+v %v", p, err)
	}
	for _, body := range []string{`null`, strings.Replace(aiProviderBody, `"enabled":true`, `"enabled":false`, 1), strings.Replace(aiProviderBody, `"openai"`, `"other"`, 1), strings.Replace(aiProviderBody, `"enabled":true`, `"enabled":null`, 1), strings.Replace(aiProviderBody, `"name":" Primary "`, `"name":" "`, 1), strings.Replace(aiProviderBody, `"modelId"`, `"unknown"`, 1)} {
		raw = nil
		json.Unmarshal([]byte(body), &raw)
		if _, err := parseAIProvider(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestAIProviderManagement(t *testing.T) {
	f := newAPIFixture(t)
	const secret = "ai-provider-test-secret"
	f.server.credentialSecret = secret
	const base = "/api/admin/ai-providers"
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%s %s: %d %+v", method, path, res.StatusCode, data)
		}
		return data
	}
	res := f.request(t, "GET", base, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || rows == nil || len(rows) != 0 {
		t.Fatalf("empty %d %+v", res.StatusCode, rows)
	}
	data := call("POST", base, aiProviderBody, 201)
	id := data["id"].(string)
	item := base + "/" + id
	encoded, _ := json.Marshal(data)
	if data["name"] != "Primary" || data["status"] != "untested" || data["baseUrl"] != nil || data["lastTestedAt"] != nil || strings.Contains(string(encoded), "secret-ai-key") {
		t.Fatalf("DTO %s", encoded)
	}
	envelopeRaw := func() []byte {
		t.Helper()
		var raw []byte
		if err := f.db.QueryRow("select credential_envelope_json from ai_providers where id=$1", id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	initial := envelopeRaw()
	var envelope credentials.Envelope
	json.Unmarshal(initial, &envelope)
	var payload map[string]string
	if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("ai-provider", id, nil), &payload); err != nil || payload["apiKey"] != "secret-ai-key" {
		t.Fatalf("envelope %v %+v", err, payload)
	}
	if _, err := f.db.Exec("update ai_providers set status='healthy' where id=$1", id); err != nil {
		t.Fatal(err)
	}
	blank := strings.Replace(aiProviderBody, " secret-ai-key ", " ", 1)
	if got := call("PATCH", item, blank, 200); got["status"] != "healthy" || string(envelopeRaw()) != string(initial) {
		t.Fatalf("blank changed credential/status %+v", got)
	}
	call("PATCH", item, aiProviderBody, 200)
	if string(envelopeRaw()) == string(initial) {
		t.Fatal("replacement did not encrypt again")
	}
	auditCount := func() int {
		t.Helper()
		var n int
		if err := f.db.QueryRow("select count(*) from platform_audit_events").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := auditCount()
	f.server.credentialSecret = ""
	call("POST", base, strings.Replace(aiProviderBody, " Primary ", "Rollback", 1), 400)
	f.server.credentialSecret = secret
	var defaults, n int
	if err := f.db.QueryRow("select count(*),count(*) filter(where is_default) from ai_providers").Scan(&n, &defaults); err != nil || n != 1 || defaults != 1 || auditCount() != before {
		t.Fatalf("rollback %d %d %v", n, defaults, err)
	}
	// Concurrent default creations serialize even when each targets a different row.
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func(i int) {
			body := strings.Replace(aiProviderBody, " Primary ", fmt.Sprintf("Concurrent %d", i), 1)
			req, err := http.NewRequest("POST", f.host.URL+base, strings.NewReader(body))
			if err != nil {
				results <- err
				return
			}
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			req.Header.Set("Content-Type", "application/json")
			resp, err := f.client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode != 201 {
					err = fmt.Errorf("status %d", resp.StatusCode)
				}
			}
			results <- err
		}(i)
	}
	for i := 0; i < 4; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.QueryRow("select count(*) from ai_providers where is_default").Scan(&defaults); err != nil || defaults != 1 {
		t.Fatalf("defaults %d %v", defaults, err)
	}
	deleted := call("DELETE", item, "", 200)
	if deleted["deleted"] != true {
		t.Fatalf("delete %+v", deleted)
	}
	before = auditCount()
	call("DELETE", item, "", 400)
	if auditCount() != before {
		t.Fatal("failed delete audited as success")
	}
	// Revoke admin between the middleware check and final transaction authorization.
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		_, err := f.db.Exec("update users set role='user' where email='admin@example.com'")
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, err
	}
	withURL := strings.Replace(aiProviderBody, `"name":" Primary "`, `"name":"Revoked","baseUrl":"https://ai.example/v1"`, 1)
	call("POST", base, withURL, 403)
	if auditCount() != before {
		t.Fatal("revoked write left audit")
	}
	call("GET", base, "", 403)
	call("DELETE", base+"/9999", "", 403)
}
