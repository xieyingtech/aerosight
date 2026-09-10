package httpapi

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"fmt"
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
	team, pid := f.project(t)
	base := fmt.Sprintf("/api/projects/%d/algorithm-providers", pid)
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
	encoded, _ := json.Marshal(data)
	if data["status"] != "disabled" || strings.Contains(string(encoded), "secret-token") || data["credentialEnvelope"] != nil || len(data["allowedHeaders"].([]any)) != 0 {
		t.Fatalf("public %s", encoded)
	}
	item := base + "/" + id
	readEnvelope := func() []byte {
		t.Helper()
		var raw []byte
		if err := f.db.QueryRow("select credential_envelope_json from algorithm_providers where id=$1", id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	initial := readEnvelope()
	var envelope credentials.Envelope
	if err := json.Unmarshal(initial, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("algorithm-provider", id, pid), &payload); err != nil || payload["token"] != "secret-token" {
		t.Fatalf("envelope %+v %v", payload, err)
	}
	blank := strings.Replace(providerBody, " secret-token ", " ", 1)
	call("PATCH", item, blank, 200)
	if string(readEnvelope()) != string(initial) {
		t.Fatal("blank replaced credential")
	}
	switchAuth := strings.Replace(blank, `"authType":"bearer"`, `"authType":"basic"`, 1)
	data = call("PATCH", item, switchAuth, 400)
	if data["error"] != "ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED" {
		t.Fatalf("switch %+v", data)
	}
	basic := strings.Replace(providerBody, `"authType":"bearer"`, `"authType":"basic","username":" service "`, 1)
	call("PATCH", item, basic, 200)
	json.Unmarshal(readEnvelope(), &envelope)
	payload = nil
	if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("algorithm-provider", id, pid), &payload); err != nil || payload["username"] != "service" || payload["password"] != "secret-token" {
		t.Fatalf("basic %+v %v", payload, err)
	}
	data = call("POST", item+"/test", "", 200)
	if data["safe"] != true || data["resolvedAddressCount"] != float64(1) || data["capability"].(map[string]any)["unavailableReason"] != nil {
		t.Fatalf("test %+v", data)
	}
	unavailable := strings.Replace(basic, "http-json", "kserve-v2", 1)
	call("PATCH", item, unavailable, 200)
	data = call("POST", item+"/test", "", 400)
	if data["error"] != "ALGORITHM_ADAPTER_UNAVAILABLE:kserve-v2" {
		t.Fatalf("adapter %+v", data)
	}
	call("PATCH", item, basic, 200)
	_, other := f.project(t)
	call("PATCH", fmt.Sprintf("/api/projects/%d/algorithm-providers/%s", other, id), basic, 400)
	// No key means creation and its audit must both roll back.
	f.server.credentialSecret = ""
	call("POST", base, strings.Replace(providerBody, " Provider ", "Other", 1), 400)
	f.server.credentialSecret = secret
	var n int
	if err := f.db.QueryRow("select count(*) from algorithm_providers where project_id=$1", pid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rollback %d %v", n, err)
	}
	// Test-only DNS hook revokes access while validation is in flight. The final
	// transaction authorization must reject this even though the precheck passed.
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		_, err := f.db.Exec("update team_members set role='member' where team_id=$1", team)
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, err
	}
	call("PATCH", item, basic, 403)
	call("POST", item+"/test", "", 403)
	res := f.request(t, "GET", base, "")
	denied := decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("list access %d %+v", res.StatusCode, denied)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'algorithm:manage' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", base, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || len(rows) != 1 || rows[0]["id"] != id {
		t.Fatalf("list %d %+v", res.StatusCode, rows)
	}
}
