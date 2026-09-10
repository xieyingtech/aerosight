package httpapi

import (
	"aerosight/server/internal/credentials"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestLiveStartSimulatorConcurrency(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	_, did := f.device(t, team, pid)
	path := fmt.Sprintf("/api/projects/%d/devices/%d/live-streams", pid, did)
	call := func(body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, "POST", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	if data := call(`{}`, 409); data["error"] != "LIVE_STREAM_NOT_SUPPORTED" {
		t.Fatalf("unsupported %+v", data)
	}
	if _, err := f.db.Exec("insert into device_capabilities(project_id,device_id,capability_code) values($1,$2,'camera.live')", pid, did); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 4)
	for range 4 {
		go func() {
			req, _ := http.NewRequest("POST", f.host.URL+path, strings.NewReader(`{}`))
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			req.Header.Set("Content-Type", "application/json")
			res, err := f.client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				results <- fmt.Errorf("status %d", res.StatusCode)
				return
			}
			results <- nil
		}()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := f.db.QueryRow("select count(*) from live_streams").Scan(&n); err != nil || n != 1 {
		t.Fatalf("concurrent %d %v", n, err)
	}
	data := call(`{}`, 200)
	session := data["session"].(map[string]any)
	if data["replayed"] != true || session["status"] != "live" || session["streamKey"] != "camera.main" || session["lastActiveAt"] == nil {
		t.Fatalf("session %+v", data)
	}
	if data := call(`{"streamKey":"second"}`, 409); data["error"] != "LIVE_STREAM_CONCURRENCY_CONFLICT" {
		t.Fatalf("limit %+v", data)
	}
	if _, err := f.db.Exec(`update device_capabilities set constraints_json='{"maxConcurrentSessions":2}' where device_id=$1`, did); err != nil {
		t.Fatal(err)
	}
	call(`{"streamKey":"second"}`, 200)
	if _, err := f.db.Exec("update devices set status='offline' where id=$1", did); err != nil {
		t.Fatal(err)
	}
	if data := call(`{}`, 409); data["error"] != "LIVE_STREAM_DEVICE_OFFLINE" {
		t.Fatalf("offline replay %+v", data)
	}
	if _, err := f.db.Exec("update devices set status='online' where id=$1", did); err != nil {
		t.Fatal(err)
	}
	call(`{"streamKey":"../unsafe"}`, 409)
	res := f.request(t, "POST", path+"?mode=replay", `{}`)
	denied := decodedResponse(t, res)
	if res.StatusCode != 409 {
		t.Fatalf("replay %+v", denied)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call(`{}`, 409)
}

func TestLiveStartDJIAtomicDispatch(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, did := f.device(t, team, pid)
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	path := fmt.Sprintf("/api/projects/%d/devices/%d/live-streams", pid, did)
	call := func(status int) map[string]any {
		t.Helper()
		res := f.request(t, "POST", path, `{"streamKey":"main","taskRunId":999999}`)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, want int) {
		t.Helper()
		var n int
		if err := f.db.QueryRow(query).Scan(&n); err != nil || n != want {
			t.Fatalf("%s = %d want %d: %v", query, n, want, err)
		}
	}
	exec("update device_adapters set adapter_type='dji' where id=$1", adapter)
	exec("insert into device_capabilities(project_id,device_id,capability_code) values($1,$2,'stream.video.control')", pid, did)
	if data := call(409); data["error"] != "LIVE_STREAM_CHANNEL_NOT_FOUND" {
		t.Fatalf("channel %+v", data)
	}
	exec("insert into device_stream_channels(project_id,team_id,device_id,stable_channel_id,capability_code,channel_key,display_name,data_type) values($1,$2,$3,'main','stream.video.control','main','Main','video')", pid, team, did)
	if data := call(409); data["error"] != "DJI_LIVE_TOPOLOGY_NOT_FOUND" {
		t.Fatalf("topology %+v", data)
	}
	var parent int
	var profile int64
	if err := f.db.QueryRow("insert into devices(project_id,name,type,adapter_id,device_type_id,status) select $1,'Parent','drone',$2,id,'online' from device_types where type_key='legacy.device' returning id", pid, adapter).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,identity_json) values($1,$2,$3,$4,'camera','{"productType":1,"productSubtype":2}'),($1,$2,$3,$5,'aircraft','{}')`, pid, team, adapter, did, parent)
	exec("insert into device_relationships(project_id,team_id,from_device_id,to_device_id,relation_type) values($1,$2,$3,$4,'contains')", pid, team, parent, did)
	if err := f.db.QueryRow("insert into device_network_profiles(project_id,team_id,name,mode,media_ingest_base_url) values($1,$2,'Media','lan','rtmp://mediamtx:1935/') returning id", pid, team).Scan(&profile); err != nil {
		t.Fatal(err)
	}
	exec("update device_adapters set network_profile_id=$2 where id=$1", adapter, profile)
	if data := call(409); data["error"] != "LIVE_STREAM_PUBLISH_CREDENTIALS_REQUIRED" {
		t.Fatalf("credentials %+v", data)
	}
	envelope, err := credentials.EncryptJSON(map[string]string{"mediaPublishUser": "publisher", "mediaPublishPassword": "publish-secret"}, secret, credentials.AAD("device-adapter", adapter, pid))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	exec("update device_adapters set credential_envelope_json=$2 where id=$1", adapter, raw)
	exec(`create function fail_live_start() returns trigger language plpgsql as $$ begin raise exception 'private failure'; end $$; create trigger fail_live_start before insert on outbox_events for each row execute function fail_live_start()`)
	call(409)
	count("select count(*) from live_streams", 0)
	count("select count(*) from device_commands", 0)
	count("select count(*) from audit_events where action='live_stream.start'", 0)
	exec("drop trigger fail_live_start on outbox_events")
	data := call(200)
	if data["replayed"] != false || data["session"].(map[string]any)["status"] != "requested" {
		t.Fatalf("start %+v", data)
	}
	count("select count(*) from live_streams where status='requested' and task_run_id is null and lease_expires_at>now() and ingest_ref like 'demo/aerosight/%'", 1)
	var parameters []byte
	if err := f.db.QueryRow("select parameters_json from device_commands where command_key='start' and priority=20").Scan(&parameters); err != nil {
		t.Fatal(err)
	}
	var command map[string]any
	json.Unmarshal(parameters, &command)
	if command["video_id"] != "aircraft/1-2-0/normal-0" || !strings.HasPrefix(command["url"].(string), "rtmp://mediamtx:1935/demo/aerosight/") || strings.Contains(string(parameters), "publish-secret") {
		t.Fatalf("command %s", parameters)
	}
	count("select count(*) from outbox_events where event_type='device.command.dispatch'", 1)
	if data = call(200); data["replayed"] != true {
		t.Fatalf("replay %+v", data)
	}
	count("select count(*) from device_commands", 1)
	// Start and stop share the device -> session order. Every request should
	// complete; the final stopping session must retain only one stop command.
	sid := data["session"].(map[string]any)["id"].(float64)
	results := make(chan error, 4)
	for i := range 4 {
		target := path
		if i%2 == 1 {
			target = fmt.Sprintf("/api/projects/%d/live-streams/%.0f/stop", pid, sid)
		}
		go func(target string) {
			req, _ := http.NewRequest("POST", f.host.URL+target, strings.NewReader(`{"streamKey":"main"}`))
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
			req.Header.Set("Content-Type", "application/json")
			res, err := f.client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				results <- fmt.Errorf("concurrent start/stop status %d", res.StatusCode)
				return
			}
			results <- nil
		}(target)
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	count("select count(*) from device_commands where command_key='stop'", 1)
	exec("insert into device_capability_grants(project_id,team_id,user_id,scope_type,action_pattern,effect) select $1,$2,id,'project','stream.*','deny' from users where email='admin@example.com'", pid, team)
	if data = call(409); data["error"] != "DEVICE_CAPABILITY_EXPLICITLY_DENIED" {
		t.Fatalf("deny replay %+v", data)
	}
	_, other := f.project(t)
	res := f.request(t, "POST", fmt.Sprintf("/api/projects/%d/devices/%d/live-streams", other, did), `{}`)
	denied := decodedResponse(t, res)
	if res.StatusCode != 409 {
		t.Fatalf("scope %+v", denied)
	}
}
