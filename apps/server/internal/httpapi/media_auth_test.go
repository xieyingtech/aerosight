package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/media"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMediaAuthMachineContract(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, device := f.device(t, team, pid)
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	f.server.cfg.MediaAdminUser = "admin"
	f.server.cfg.MediaAdminPassword = " password "
	call := func(body string, status int) {
		t.Helper()
		req, _ := http.NewRequest("POST", f.host.URL+"/api/media-auth", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// No browser cookie or CSRF header: this is a MediaMTX machine callback.
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		response, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status {
			t.Fatalf("status %d want %d: %s", res.StatusCode, status, response)
		}
		if (status == 204 && len(response) != 0) || (status == 401 && string(response) != `{"error":"MEDIA_AUTH_DENIED"}`) {
			t.Fatalf("response %q", response)
		}
		if len(res.Cookies()) != 0 {
			t.Fatal("machine auth created browser session")
		}
	}
	call(`{"action":"api","user":"admin","password":" password ","id":"mediamtx-request"}`, 204)
	call(`{"action":"api","user":"admin","password":"password"}`, 401)
	call(`null`, 401)
	call(`{`, 401)
	call(`{"action":"publish"}`, 401)
	path := "demo/aerosight/test-publish"
	var stream int64
	if err := f.db.QueryRow("insert into live_streams(project_id,team_id,device_id,adapter_id,stream_key,source_type,status,ingest_ref) values($1,$2,$3,$4,'main','dji','requested',$5) returning id", pid, team, device, adapter, path).Scan(&stream); err != nil {
		t.Fatal(err)
	}
	envelope, err := credentials.EncryptJSON(map[string]string{"mediaPublishUser": "publisher", "mediaPublishPassword": "secret-publish"}, secret, credentials.AAD("device-adapter", adapter, pid))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(envelope)
	if _, err := f.db.Exec("update device_adapters set credential_envelope_json=$2 where id=$1", adapter, encoded); err != nil {
		t.Fatal(err)
	}
	publish := fmt.Sprintf(`{"action":"publish","path":%q,"protocol":"rtmp","user":"publisher","password":"secret-publish"}`, path)
	call(publish, 204)
	call(strings.Replace(publish, "secret-publish", "wrong", 1), 401)
	call(strings.Replace(publish, path, "other/path", 1), 401)
	if _, err := f.db.Exec("update live_streams set status='stopped' where id=$1", stream); err != nil {
		t.Fatal(err)
	}
	call(publish, 401)
	if _, err := f.db.Exec("update live_streams set status='live' where id=$1", stream); err != nil {
		t.Fatal(err)
	}
	wrong, err := credentials.EncryptJSON(map[string]string{"mediaPublishUser": "publisher", "mediaPublishPassword": "secret-publish"}, secret, credentials.AAD("device-adapter", adapter, pid+1))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(wrong)
	if _, err := f.db.Exec("update device_adapters set credential_envelope_json=$2 where id=$1", adapter, encoded); err != nil {
		t.Fatal(err)
	}
	call(publish, 401)
	token, err := media.IssuePlaybackToken(secret, media.PlaybackClaims{ProjectID: int64(pid), StreamID: stream, Path: path, Protocols: []string{"hls"}}, time.Now(), 60)
	if err != nil {
		t.Fatal(err)
	}
	call(fmt.Sprintf(`{"action":"read","path":%q,"protocol":"hls","token":%q}`, path, token.Token), 204)
	call(fmt.Sprintf(`{"action":"playback","path":%q,"protocol":"hls","query":%q}`, path, "token="+token.Token), 204)
	call(fmt.Sprintf(`{"action":"read","path":%q,"protocol":"webrtc","token":%q}`, path, token.Token), 401)
	call(fmt.Sprintf(`{"action":"read","path":"wrong","protocol":"hls","token":%q}`, token.Token), 401)
	expired, _ := media.IssuePlaybackToken(secret, media.PlaybackClaims{ProjectID: int64(pid), StreamID: stream, Path: path, Protocols: []string{"hls"}}, time.Now().Add(-time.Minute), 1)
	call(fmt.Sprintf(`{"action":"read","path":%q,"protocol":"hls","token":%q}`, path, expired.Token), 401)
	f.server.cfg.MediaAdminPassword = ""
	call(`{"action":"api","user":"admin","password":""}`, 401)
}
