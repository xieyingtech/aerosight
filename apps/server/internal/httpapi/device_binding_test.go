package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDeviceBindingConcurrencyAndScope(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, _ := f.device(t, team, pid)
	var identity int64
	if err := f.db.QueryRow(`insert into device_external_identities(project_id,team_id,adapter_id,external_device_id,identity_json) values($1,$2,$3,'discovered','{"deviceTypeKey":"unregistered.type","capabilities":["stream.telemetry",12,"stream.telemetry","state.read"]}') returning id`, pid, team, adapter).Scan(&identity); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/device-adapters/discoveries/%d/bind", pid, identity)
	type result struct {
		status int
		body   map[string]any
		err    error
	}
	results := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			req, _ := http.NewRequest("POST", f.host.URL+path, strings.NewReader(`{"name":"  Bound device  ","deviceType":" drone "}`))
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			req.Header.Set("Content-Type", "application/json")
			res, err := f.client.Do(req)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer res.Body.Close()
			var body map[string]any
			err = json.NewDecoder(res.Body).Decode(&body)
			results <- result{res.StatusCode, body, err}
		}()
	}
	var deviceID float64
	created := 0
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil || r.status != 200 {
			t.Fatalf("bind %+v", r)
		}
		if r.body["replayed"] == false {
			created++
		}
		id, ok := r.body["deviceId"].(float64)
		if !ok || (deviceID != 0 && deviceID != id) {
			t.Fatalf("different devices %+v", r)
		}
		deviceID = id
	}
	if created != 1 {
		t.Fatalf("created %d", created)
	}
	var name, status, typeKey string
	var metadata []byte
	if err := f.db.QueryRow(`select d.name,d.status,t.type_key,d.metadata_json from devices d join device_types t on t.id=d.device_type_id where d.id=$1`, deviceID).Scan(&name, &status, &typeKey, &metadata); err != nil {
		t.Fatal(err)
	}
	if name != "Bound device" || status != "unknown" || typeKey != "legacy.device" || !strings.Contains(string(metadata), "unregistered.type") {
		t.Fatalf("projection %s %s %s %s", name, status, typeKey, metadata)
	}
	var version, count int
	if err := f.db.QueryRow("select version_number from device_capabilities where device_id=$1 and capability_code='stream.telemetry'", deviceID).Scan(&version); err != nil || version != 2 {
		t.Fatalf("capability duplicate %d %v", version, err)
	}
	if err := f.db.QueryRow("select count(*) from audit_events where project_id=$1 and action='device_identity.bind' and status='completed'", pid).Scan(&count); err != nil || count != 4 {
		t.Fatalf("audit %d %v", count, err)
	}
	_, otherPID := f.project(t)
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/device-adapters/discoveries/%d/bind", otherPID, identity), `{"name":"Other","deviceType":"drone"}`)
	data := decodedResponse(t, res)
	if res.StatusCode != 400 || data["error"] != "Invalid or unauthorized device binding" {
		t.Fatalf("cross project %d %+v", res.StatusCode, data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'device:configure' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, `{"name":"Retry","deviceType":"drone"}`)
	data = decodedResponse(t, res)
	if res.StatusCode != 400 {
		t.Fatalf("member replay %d %+v", res.StatusCode, data)
	}
}

func TestDeviceBindingRollback(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, _ := f.device(t, team, pid)
	var identity int64
	if err := f.db.QueryRow(`insert into device_external_identities(project_id,team_id,adapter_id,external_device_id,identity_json) values($1,$2,$3,'rollback','{"capabilities":["state.read"]}') returning id`, pid, team, adapter).Scan(&identity); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`create function reject_binding_capability() returns trigger language plpgsql as $$ begin raise exception 'private fault details'; end $$; create trigger reject_binding_capability before insert on device_capabilities for each row execute function reject_binding_capability()`); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/device-adapters/discoveries/%d/bind", pid, identity)
	for _, body := range []string{`{"name":"Rollback","deviceType":"drone"}`, `{"name":3,"deviceType":"drone"}`, `{"name":"x","deviceType":"invalid"}`, `{"name":"x","deviceType":"drone"} {}`} {
		res := f.request(t, "POST", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != 400 || data["error"] != "Invalid or unauthorized device binding" {
			t.Fatalf("safe error %d %+v", res.StatusCode, data)
		}
	}
	for _, query := range []string{"select count(*) from devices where project_id=$1 and name='Rollback'", "select count(*) from device_external_identities where project_id=$1 and (device_id is not null or bound_at is not null)", "select count(*) from audit_events where project_id=$1"} {
		var count int
		if err := f.db.QueryRow(query, pid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s %d %v", query, count, err)
		}
	}
}
