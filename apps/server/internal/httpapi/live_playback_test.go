package httpapi

import (
	"aerosight/server/internal/media"
	"fmt"
	"net/url"
	"testing"
	"time"
)

func TestLivePlaybackAuthorizationAndTokens(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, did := f.device(t, team, pid)
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	var sid, profile, channel int64
	if err := f.db.QueryRow("insert into live_streams(project_id,team_id,device_id,adapter_id,stream_key,source_type,status) values($1,$2,$3,$4,'main','simulator','requested') returning id", pid, team, did, adapter).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/live-streams/%d/playback", pid, sid)
	call := func(status int) map[string]any {
		t.Helper()
		res := f.request(t, "GET", path, "")
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%d want %d %+v", res.StatusCode, status, data)
		}
		return data
	}
	data := call(200)
	if data["available"] != false || data["reason"] != "stream-requested" || data["session"].(map[string]any)["lastActiveAt"] != nil {
		t.Fatalf("requested %+v", data)
	}
	if _, err := f.db.Exec("update live_streams set status='live' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	if data = call(200); data["reason"] != "playback-unavailable" {
		t.Fatalf("no ref %+v", data)
	}
	if _, err := f.db.Exec("update live_streams set playback_ref='simulator://devices/9/main',playback_locator_expires_at=now()-interval '1 hour' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	data = call(200)
	if data["available"] != true || data["locator"] == nil {
		t.Fatalf("simulator %+v", data)
	}
	var fresh bool
	if err := f.db.QueryRow("select playback_locator_expires_at>now() from live_streams where id=$1", sid).Scan(&fresh); err != nil || !fresh {
		t.Fatalf("expiry %v %v", fresh, err)
	}
	if _, err := f.db.Exec("update live_streams set source_type='dji',playback_ref='demo/aerosight/session' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	if data = call(200); data["reason"] != "playback-protocol-unavailable" {
		t.Fatalf("no protocol %+v", data)
	}
	if err := f.db.QueryRow(`insert into device_network_profiles(project_id,team_id,name,mode,media_playback_base_url,config_json) values($1,$2,'Media','lan','https://media.example/hls/','{"webrtcPlaybackBaseUrl":"https://media.example/rtc/"}') returning id`, pid, team).Scan(&profile); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update device_adapters set network_profile_id=$2 where id=$1", adapter, profile); err != nil {
		t.Fatal(err)
	}
	data = call(200)
	candidates := data["playback"].(map[string]any)["candidates"].([]any)
	if len(candidates) != 2 || candidates[0].(map[string]any)["protocol"] != "webrtc" {
		t.Fatalf("order %+v", data)
	}
	parsed, err := url.Parse(candidates[1].(map[string]any)["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	token := parsed.Query().Get("token")
	claims, valid := media.VerifyPlaybackToken(secret, token, "demo/aerosight/session", "hls", time.Now())
	if !valid || claims.StreamID != sid || claims.ProjectID != int64(pid) {
		t.Fatalf("claims %+v %v", claims, valid)
	}
	res := f.request(t, "POST", "/api/media-auth", fmt.Sprintf(`{"action":"read","path":"demo/aerosight/session","protocol":"hls","token":%q}`, token))
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatalf("media auth %d", res.StatusCode)
	}
	if _, err := f.db.Exec("insert into device_capabilities(project_id,device_id,capability_code) values($1,$2,'stream.video')", pid, did); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into device_stream_channels(project_id,team_id,device_id,stable_channel_id,capability_code,channel_key,display_name,data_type) values($1,$2,$3,'video-main','stream.video','main','Video','video') returning id", pid, team, did).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update live_streams set stream_channel_id=$2 where id=$1", sid, channel); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_capability_grants(project_id,team_id,user_id,scope_type,action_pattern,effect) select $1,$2,id,'project','stream.*','deny' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	call(403)
	if _, err := f.db.Exec("delete from device_capability_grants where project_id=$1", pid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call(403)
	if _, err := f.db.Exec("insert into device_capability_grants(project_id,team_id,user_id,scope_type,action_pattern,effect) select $1,$2,id,'project','stream.video','allow' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	call(200)
	if _, err := f.db.Exec("update live_streams set status='stopped' where id=$1", sid); err != nil {
		t.Fatal(err)
	}
	if data = call(200); data["reason"] != "stream-stopped" || data["playback"] != nil {
		t.Fatalf("stopped %+v", data)
	}
	_, other := f.project(t)
	res = f.request(t, "GET", fmt.Sprintf("/api/projects/%d/live-streams/%d/playback", other, sid), "")
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross project")
	}
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	call(403)
}
