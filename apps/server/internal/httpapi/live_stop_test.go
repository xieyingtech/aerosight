package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestLiveStopAtomicityAndConcurrency(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, did := f.device(t, team, pid)
	create := func(source, status, key string) int64 {
		t.Helper()
		var id int64
		if err := f.db.QueryRow("insert into live_streams(project_id,team_id,device_id,adapter_id,stream_key,source_type,status,vendor_stream_ref,playback_ref,lease_owner,lease_expires_at) values($1,$2,$3,$4,$5,$6,$7,'device/1-0-0/normal-0','demo/aerosight/test','test',now()+interval '1 hour') returning id", pid, team, did, adapter, key, source, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	path := func(id int64) string { return fmt.Sprintf("/api/projects/%d/live-streams/%d/stop", pid, id) }
	call := func(id int64, status int) map[string]any {
		t.Helper()
		res := f.request(t, "POST", path(id), "")
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	count := func(query string, want int) {
		t.Helper()
		var n int
		if err := f.db.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("%s = %d want %d", query, n, want)
		}
	}
	sim := create("simulator", "live", "sim")
	data := call(sim, 200)
	if data["replayed"] != false || data["session"].(map[string]any)["status"] != "stopped" || data["session"].(map[string]any)["playbackRef"] != nil {
		t.Fatalf("sim %+v", data)
	}
	if data = call(sim, 200); data["replayed"] != true {
		t.Fatalf("replay %+v", data)
	}
	count("select count(*) from device_commands", 0)
	count("select count(*) from live_streams where status='stopped' and lease_owner is null and lease_expires_at is null and ended_at is not null", 1)
	dji := create("dji", "live", "dji")
	results := make(chan error, 4)
	for range 4 {
		go func() {
			req, err := http.NewRequest("POST", f.host.URL+path(dji), nil)
			if err != nil {
				results <- err
				return
			}
			req.Header.Set("Origin", "http://frontend.test")
			req.Header.Set("X-CSRF-Token", f.csrf)
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
	count("select count(*) from device_commands where command_key='stop' and priority=30 and capability_code='stream.video.control' and status='dispatchable'", 1)
	count("select count(*) from outbox_events where event_type='device.command.dispatch'", 1)
	count("select count(*) from project_events where event_type='live_stream.stop_requested'", 1)
	count("select count(*) from live_streams where status='stopping' and lease_expires_at>now() and ended_at is null and playback_ref is not null", 1)
	// A session reactivated after its existing stop command must not dispatch a
	// nonexistent new command UUID or duplicate the outbox event.
	if _, err := f.db.Exec("update live_streams set status='live' where id=$1", dji); err != nil {
		t.Fatal(err)
	}
	call(dji, 200)
	count("select count(*) from device_commands", 1)
	count("select count(*) from outbox_events where event_type='device.command.dispatch'", 1)
	failed := create("dji", "failed", "failed")
	call(failed, 200)
	count("select count(*) from device_commands", 1)
	rollback := create("dji", "live", "rollback")
	if _, err := f.db.Exec(`create function fail_live_stop() returns trigger language plpgsql as $$ begin raise exception 'private failure'; end $$; create trigger fail_live_stop before insert on outbox_events for each row execute function fail_live_stop()`); err != nil {
		t.Fatal(err)
	}
	call(rollback, 400)
	count("select count(*) from device_commands", 1)
	count(fmt.Sprintf("select count(*) from live_streams where id=%d and status='live'", rollback), 1)
	count("select count(*) from audit_events where status<>'completed'", 0)
	if _, err := f.db.Exec("drop trigger fail_live_stop on outbox_events"); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "POST", path(rollback)+"?mode=replay", "")
	data = decodedResponse(t, res)
	if res.StatusCode != 409 || data["error"] != "REPLAY_CONTROL_FORBIDDEN" {
		t.Fatalf("replay mode %+v", data)
	}
	_, other := f.project(t)
	res = f.request(t, "POST", fmt.Sprintf("/api/projects/%d/live-streams/%d/stop", other, rollback), "")
	data = decodedResponse(t, res)
	if res.StatusCode != 400 {
		t.Fatalf("scope %+v", data)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call(rollback, 400)
	count(fmt.Sprintf("select count(*) from live_streams where id=%d and status='live'", rollback), 1)
}
