package httpapi

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

const providerBody = `{"name":" Provider ","providerType":"http-json","baseUrl":"https://algorithm.example/v1","authType":"bearer","credential":" secret-token ","timeoutSeconds":30,"concurrencyLimit":4,"rateLimitPerMinute":120}`

func TestAlgorithmProviderPolicy(t *testing.T) {
	var raw map[string]any
	json.Unmarshal([]byte(providerBody), &raw)
	input, err := parseAlgorithmProvider(raw)
	if err != nil || input.Credential["token"] != "secret-token" || input.Audit["credential"] != nil || input.Params.Name != "Provider" {
		t.Fatalf("input %+v %v", input, err)
	}
	for _, body := range []string{strings.Replace(providerBody, `"authType":"bearer"`, `"authType":"basic"`, 1), strings.Replace(providerBody, `"timeoutSeconds":30`, `"timeoutSeconds":0`, 1), strings.Replace(providerBody, `"concurrencyLimit":4`, `"concurrencyLimit":1.5`, 1), strings.Replace(providerBody, `"name":" Provider "`, `"name":"Provider","apiKey":"leak"`, 1)} {
		var raw map[string]any
		json.Unmarshal([]byte(body), &raw)
		if _, err := parseAlgorithmProvider(raw); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	s := &Server{}
	s.cfg.AlgorithmAllowedHosts = []string{"*.example", "algorithm.example"}
	s.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	if _, n, err := s.validateAlgorithmURL(context.Background(), "https://algorithm.example/v1"); err != nil || n != 1 {
		t.Fatalf("public %d %v", n, err)
	}
	for _, target := range []string{"http://algorithm.example", "https://user:secret@algorithm.example", "https://example", "https://evil.example.net"} {
		if _, _, err := s.validateAlgorithmURL(context.Background(), target); err == nil {
			t.Fatalf("accepted %s", target)
		}
	}
	for _, address := range []string{"127.0.0.1", "10.1.1.1", "100.64.0.1", "192.0.0.1", "198.18.0.1", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1"} {
		s.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(address)}, nil
		}
		if _, _, err := s.validateAlgorithmURL(context.Background(), "https://algorithm.example"); err == nil {
			t.Fatalf("mixed DNS %s", address)
		}
	}
}

func TestAlgorithmProviderManagement(t *testing.T) {
	f := newAPIFixture(t)
	base := "/api/admin/algorithm-providers"
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	f.server.cfg.AlgorithmAllowedHosts = []string{"algorithm.example"}
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("status %d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	data := call("POST", base, providerBody, 201)
	id := data["id"].(string)
	if data["status"] != "disabled" {
		t.Fatalf("status %+v", data)
	}
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "secret-token") || data["credentialEnvelope"] != nil {
		t.Fatalf("leaked credential %s", encoded)
	}
	var raw []byte
	if err := f.db.QueryRow("select credential_envelope_json from algorithm_providers where id=$1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var envelope credentials.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("algorithm-provider", id, 0), &payload); err != nil || payload["token"] != "secret-token" {
		t.Fatalf("credential %+v %v", payload, err)
	}
	item := base + "/" + id
	active := strings.Replace(providerBody, `"name":" Provider "`, `"status":"active","name":" Provider "`, 1)
	if got := call("PATCH", item, active, 200); got["status"] != "active" {
		t.Fatalf("active %+v", got)
	}
	blank := strings.Replace(active, " secret-token ", " ", 1)
	call("PATCH", item, blank, 200)
	var retained []byte
	if err := f.db.QueryRow("select credential_envelope_json from algorithm_providers where id=$1", id).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(raw) {
		t.Fatal("blank credential replaced old secret")
	}
	tested := call("POST", item+"/test", "", 200)
	if tested["safe"] != true {
		t.Fatalf("test %+v", tested)
	}
	call("PATCH", item, strings.Replace(active, `"status":"active"`, `"status":"invalid"`, 1), 400)
	call("DELETE", item, "", 200)
}
