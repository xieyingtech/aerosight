package httpapi

import (
	"aerosight/server/internal/connector"
	"aerosight/server/internal/flighthub"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type projectClientFunc func(context.Context, string) ([]flighthub.Project, error)

func (f projectClientFunc) ListProjects(ctx context.Context, token string) ([]flighthub.Project, error) {
	return f(ctx, token)
}

const testFlightHubProject = "12345678-1234-4234-8234-123456789abc"
const testFlightHubSecret = "http-flighthub-test-secret-32-bytes"

func fixtureFlightHub(t *testing.T) (*apiFixture, int, int, string) {
	t.Helper()
	f := newAPIFixture(t)
	team, pid := f.project(t)
	f.server.AttachFlightHub(projectClientFunc(func(_ context.Context, token string) ([]flighthub.Project, error) {
		if token == "bad-token" {
			return nil, &flighthub.APIError{SafeCode: "credential_invalid"}
		}
		return []flighthub.Project{{UUID: testFlightHubProject, Name: "测试项目", OrganizationUUID: "org-1"}}, nil
	}), true, testFlightHubSecret)
	return f, team, pid, fmt.Sprintf("/api/projects/%d/connectors/dji-flighthub", pid)
}
func decodedResponse(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	defer res.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	return data
}
func TestFlightHubLifecycleAuditCredentialsAndHistory(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	res := f.request(t, "POST", path+"/projects", `{"token":"ephemeral-token"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || data["projects"].([]any)[0].(map[string]any)["organizationUuid"] != "org-1" {
		t.Fatalf("discovery %d %+v", res.StatusCode, data)
	}
	input := `{"token":"stored-token","projectUuid":"` + testFlightHubProject + `"}`
	res = f.request(t, "POST", path, input)
	data = decodedResponse(t, res)
	if res.StatusCode != 201 || data["initialSyncQueued"] != true {
		t.Fatalf("create %d %+v", res.StatusCode, data)
	}
	id, ok := data["id"].(string)
	if !ok {
		t.Fatal("connector ID must remain string")
	}
	numericID, _ := strconv.ParseInt(id, 10, 64)
	res = f.request(t, "POST", path, input)
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"].(map[string]any)["code"] != "duplicate_connection" {
		t.Fatalf("duplicate %d %+v", res.StatusCode, data)
	}
	checkToken := func(expected string) {
		t.Helper()
		var envelope json.RawMessage
		if err := f.db.QueryRow("select credential_envelope_json from device_adapters where id=$1", id).Scan(&envelope); err != nil {
			t.Fatal(err)
		}
		resolver := flighthub.EncryptedTokenResolver{AuthSecret: testFlightHubSecret}
		token, err := resolver.ResolveToken(context.Background(), connector.Instance{ID: numericID, ProjectID: pid, CredentialEnvelope: envelope})
		if err != nil || token != expected {
			t.Fatalf("worker cannot decrypt: %v", err)
		}
		if strings.Contains(string(envelope), expected) {
			t.Fatal("token stored unencrypted")
		}
	}
	checkToken("stored-token")
	_, outsidePID := f.project(t)
	outside := fmt.Sprintf("/api/projects/%d/connectors/dji-flighthub/%s", outsidePID, id)
	for _, operation := range []struct{ method, path, body string }{{"POST", outside + "/sync", ""}, {"PUT", outside + "/token", `{"token":"updated-token"}`}, {"DELETE", outside, ""}} {
		denied := f.request(t, operation.method, operation.path, operation.body)
		deniedData := decodedResponse(t, denied)
		if denied.StatusCode != 404 {
			t.Fatalf("cross-project connector %d %+v", denied.StatusCode, deniedData)
		}
	}
	res = f.request(t, "POST", path+"/"+id+"/sync", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 202 || data["deduplicated"] != true {
		t.Fatalf("sync %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "PUT", path+"/"+id+"/token", `{"token":"updated-token"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["tokenUpdated"] != true || data["deduplicated"] != true {
		t.Fatalf("token %d %+v", res.StatusCode, data)
	}
	checkToken("updated-token")
	_, device := f.device(t, team, pid)
	var identity int
	if err := f.db.QueryRow("insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,discovery_status) values($1,$2,$3,$4,'external-test','managed') returning id", pid, team, id, device).Scan(&identity); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_connector_bindings(project_id,team_id,device_id,connector_instance_id,external_identity_id) values($1,$2,$3,$4,$5)", pid, team, device, id, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into connector_sync_runs(project_id,team_id,connector_instance_id,discovery_mode,status,started_at) values($1,$2,$3,'poll','pending',null),($1,$2,$3,'poll','running',now()),($1,$2,$3,'poll','succeeded',now())", pid, team, id); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["connectors"].([]any)) != 1 || len(data["identities"].([]any)) != 1 || len(data["syncRuns"].([]any)) != 3 {
		t.Fatalf("inventory %d %+v", res.StatusCode, data)
	}
	identityJSON := data["identities"].([]any)[0].(map[string]any)
	if identityJSON["connectorId"] != id || identityJSON["serialNumber"] != nil {
		t.Fatalf("identity contract %+v", identityJSON)
	}
	res = f.request(t, "DELETE", path+"/"+id, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 200 || data["historyPreserved"] != true {
		t.Fatalf("disconnect %d %+v", res.StatusCode, data)
	}
	for query, want := range map[string]int{
		"select count(*) from device_adapters where id=$1 and status='disabled'":                              1,
		"select count(*) from device_connector_bindings where connector_instance_id=$1 and status='disabled'": 1,
		"select count(*) from connector_sync_runs where connector_instance_id=$1 and status='cancelled'":      2,
		"select count(*) from connector_sync_runs where connector_instance_id=$1 and status='succeeded'":      1,
		"select count(*) from device_external_identities where adapter_id=$1":                                 1,
		"select count(*) from outbox_events where payload_json->>'connectorInstanceId'=$1 and status='dead'":  1,
	} {
		var count int
		if err := f.db.QueryRow(query, id).Scan(&count); err != nil || count != want {
			t.Fatalf("history query %s: %d %v", query, count, err)
		}
	}
	res = f.request(t, "POST", path+"/"+id+"/sync", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 409 {
		t.Fatalf("disabled sync %d %+v", res.StatusCode, data)
	}
	var auditText string
	if err := f.db.QueryRow("select coalesce(jsonb_agg(to_jsonb(a)),'[]')::text from audit_events a where project_id=$1", pid).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"ephemeral-token", "stored-token", "updated-token"} {
		if strings.Contains(auditText, secret) {
			t.Fatal("secret in audit")
		}
	}
	if !strings.Contains(auditText, "connector.flighthub.disconnect") || strings.Contains(auditText, `"status": "accepted"`) {
		t.Fatal("audit incomplete")
	}
}

func TestFlightHubValidationErrorsRateAndRevocation(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	for _, body := range []string{`{"token":"x","projectUuid":null}`, `{"token":"x","extra":1}`, `{"token":" "}`, `{"token":"x"} {}`} {
		res := f.request(t, "POST", path+"/projects", body)
		data := decodedResponse(t, res)
		if res.StatusCode != 400 {
			t.Fatalf("validation %s %d %+v", body, res.StatusCode, data)
		}
	}
	res := f.request(t, "POST", path+"/projects", `{"token":"bad-token"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 401 || data["error"].(map[string]any)["code"] != "credential_invalid" {
		t.Fatalf("upstream credential %d %+v", res.StatusCode, data)
	}
	for n := 0; n < 5; n++ {
		res = f.request(t, "POST", path+"/projects", `{"token":"ok-token"}`)
		data = decodedResponse(t, res)
	}
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" {
		t.Fatalf("rate %d %+v", res.StatusCode, data)
	}
	var failures int
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and policy_result_json->>'status'='failed'", pid).Scan(&failures); err != nil || failures != 2 {
		t.Fatalf("failure audit %d %v", failures, err)
	}
	f.server.flightHub.client = projectClientFunc(func(ctx context.Context, _ string) ([]flighthub.Project, error) {
		_, err := f.db.ExecContext(ctx, "delete from team_members where team_id=$1", team)
		return []flighthub.Project{{UUID: testFlightHubProject, Name: "revoked"}}, err
	})
	res = f.request(t, "POST", path, `{"token":"ok-token","projectUuid":"`+testFlightHubProject+`"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("revocation during upstream %d %+v", res.StatusCode, data)
	}
	var adapters int
	if err := f.db.QueryRow("select count(*) from device_adapters where project_id=$1", pid).Scan(&adapters); err != nil || adapters != 0 {
		t.Fatalf("revoked write persisted %d %v", adapters, err)
	}
}

type flightHubTransport func(*http.Request) (*http.Response, error)

func (f flightHubTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFlightHubHTTPClientWiring(t *testing.T) {
	f, _, _, path := fixtureFlightHub(t)
	client, err := flighthub.NewChinaClient(flighthub.Config{RequestID: func() string { return "http-test" }, Timeout: time.Second, HTTPClient: &http.Client{Transport: flightHubTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "es-flight-api-cn.djigate.com" || r.Header.Get("X-User-Token") != "wire-token" {
			t.Errorf("incorrect upstream request: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"list":[]}}`))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	f.server.AttachFlightHub(client, true, testFlightHubSecret)
	res := f.request(t, "POST", path+"/projects", `{"token":"wire-token"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || len(data["projects"].([]any)) != 0 {
		t.Fatalf("wire %d %+v", res.StatusCode, data)
	}
}

func TestFlightHubManagerRoleAndProjectRevalidation(t *testing.T) {
	f, team, pid, path := fixtureFlightHub(t)
	res := f.request(t, "POST", path, `{"token":"stored-token","projectUuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"].(map[string]any)["code"] != "project_access_changed" {
		t.Fatalf("changed upstream project %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'device:configure' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		res = f.request(t, method, path, `{"token":"stored-token","projectUuid":"`+testFlightHubProject+`"}`)
		data = decodedResponse(t, res)
		if res.StatusCode != 403 {
			t.Fatalf("member must not manage %d %+v", res.StatusCode, data)
		}
	}
}
