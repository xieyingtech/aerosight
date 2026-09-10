package httpapi

import (
	"aerosight/server/internal/credentials"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDeviceAdapterLifecycleAndCredentials(t *testing.T) {
	f := newAPIFixture(t)
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/device-adapters", pid)
	res := f.request(t, "GET", path, "")
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || rows == nil || len(rows) != 0 {
		t.Fatalf("empty %d %+v", res.StatusCode, rows)
	}
	res = f.request(t, "POST", path, `{"name":"  DJI adapter  ","adapterType":"dji","credentials":{"mqttPassword":"original-secret","appKey":"keep-secret"},"config":{"gatewaySerials":["GW-1"]},"ignored":true}`)
	created := decodedResponse(t, res)
	if res.StatusCode != 201 || created["name"] != "DJI adapter" || created["vendor"] != nil || created["lastCheckedAt"] != nil || created["protocolVersion"] != "1" || created["status"] != "disabled" {
		t.Fatalf("create %d %+v", res.StatusCode, created)
	}
	id, ok := created["id"].(string)
	if !ok {
		t.Fatalf("id %+v", created)
	}
	item := path + "/" + id
	decodeSecrets := func() map[string]string {
		t.Helper()
		var raw []byte
		if err := f.db.QueryRow("select credential_envelope_json from device_adapters where id=$1", id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var envelope credentials.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		var values map[string]string
		if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("device-adapter", id, pid), &values); err != nil {
			t.Fatal(err)
		}
		if err := credentials.DecryptJSON(envelope, secret, credentials.AAD("device-adapter", id, pid+1), &map[string]string{}); err == nil {
			t.Fatal("scope not bound")
		}
		return values
	}
	if values := decodeSecrets(); values["mqttPassword"] != "original-secret" {
		t.Fatalf("initial credentials %+v", values)
	}
	res = f.request(t, "PATCH", item, `{"credentials":{"mqttPassword":" new-secret ","appKey":"   "}}`)
	updated := decodedResponse(t, res)
	if res.StatusCode != 200 || updated["updated"] != true {
		t.Fatalf("update %d %+v", res.StatusCode, updated)
	}
	if _, ok := updated["id"].(float64); !ok {
		t.Fatalf("numeric credential update id %+v", updated)
	}
	if values := decodeSecrets(); values["mqttPassword"] != " new-secret " || values["appKey"] != "keep-secret" {
		t.Fatalf("merge %+v", values)
	}
	for _, enabled := range []bool{false, true} {
		res = f.request(t, "PATCH", item, fmt.Sprintf(`{"enabled":%t}`, enabled))
		data := decodedResponse(t, res)
		expected := "disabled"
		if enabled {
			expected = "connecting"
		}
		if res.StatusCode != 200 || data["id"] != id || data["status"] != expected {
			t.Fatalf("enabled %d %+v", res.StatusCode, data)
		}
	}
	res = f.request(t, "PATCH", path+"/999999", `{"credentials":{"mqttPassword":" "}}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 200 || data["updated"] != false {
		t.Fatalf("empty no-op %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "GET", path, "")
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	raw, _ := json.Marshal(rows)
	if len(rows) != 1 || strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "credential") {
		t.Fatalf("list leaks %s", raw)
	}
	var audit string
	if err := f.db.QueryRow("select coalesce(jsonb_agg(to_jsonb(a))::text,'[]') from audit_events a where project_id=$1", pid).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"original-secret", "new-secret", "keep-secret"} {
		if strings.Contains(audit, value) {
			t.Fatal("audit contains credential")
		}
	}
	var count int
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1", pid).Scan(&count); err != nil || count != 4 {
		t.Fatalf("audit count %d %v", count, err)
	}
	_, other := f.project(t)
	res = f.request(t, "PATCH", fmt.Sprintf("/api/projects/%d/device-adapters/%s", other, id), `{"enabled":true}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 400 {
		t.Fatalf("scope %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'device:configure' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "GET", path, "")
	data = decodedResponse(t, res)
	if res.StatusCode != 403 {
		t.Fatalf("member list %d %+v", res.StatusCode, data)
	}
	res = f.request(t, "PATCH", item, `{"credentials":{"appKey":"denied"}}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 400 {
		t.Fatalf("member update %d %+v", res.StatusCode, data)
	}
}

func TestDeviceAdapterValidationAndAtomicCreation(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d/device-adapters", pid)
	for _, body := range []string{
		`{"name":"","adapterType":"dji"}`,
		`{"name":"x","adapterType":"ros2"}`,
		`{"name":"x","adapterType":"dji","vendor":null}`,
		`{"name":"x","adapterType":"dji","config":{"nested":[{"api-key":"secret"}]}}`,
		`{"name":"x","adapterType":"dji","config":[]}`,
		`{"name":"x","adapterType":"dji","credentials":{"password":12}}`,
		// No encryption key attached: creation must roll back the new adapter and its audit.
		`{"name":"x","adapterType":"dji","credentials":{"password":"secret"}}`,
	} {
		res := f.request(t, "POST", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != 400 || data["error"] != "Invalid or unauthorized device adapter request" {
			t.Fatalf("invalid %d %+v", res.StatusCode, data)
		}
	}
	for _, table := range []string{"device_adapters", "audit_events"} {
		var count int
		if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("atomic %s %d %v", table, count, err)
		}
	}
	res := f.request(t, "POST", path, `{"name":"sim","adapterType":"simulator"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("simulator %d %+v", res.StatusCode, data)
	}
	item := path + "/" + data["id"].(string)
	for _, body := range []string{`{"credentials":{"appKey":"dji only"}}`, `{"credentials":{"unknown":"bad"}}`, `{"enabled":"yes"}`} {
		res = f.request(t, "PATCH", item, body)
		data = decodedResponse(t, res)
		if res.StatusCode != 400 {
			t.Fatalf("invalid update %d %+v", res.StatusCode, data)
		}
	}
}
