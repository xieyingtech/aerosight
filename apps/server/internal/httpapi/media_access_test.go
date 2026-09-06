package httpapi

import (
	"aerosight/server/internal/media"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaAccessHTTPRangeAndAuthorization(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	root := t.TempDir()
	secret := "0123456789abcdef0123456789abcdef"
	f.server.AttachDeviceCredentials(secret)
	f.server.AttachMediaStorage(root)
	dir := filepath.Join(root, "projects", fmt.Sprint(pid))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	var asset int
	if err := f.db.QueryRow("insert into assets(project_id,team_id,kind,storage_key,logical_key,mime_type) values($1,$2,'video',$3,'video.mp4','video/mp4') returning id", pid, team, fmt.Sprintf("projects/%d/video.mp4", pid)).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/projects/%d/assets/%d", pid, asset)
	issue := func(action string, status int) map[string]any {
		t.Helper()
		res := f.request(t, "GET", base+"/access?action="+action, "")
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("access %d %+v", res.StatusCode, data)
		}
		return data
	}
	url := issue("play", 200)["url"].(string)
	fetch := func(method, target, rng string, status int, want string) {
		t.Helper()
		req, _ := http.NewRequest(method, f.host.URL+target, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		res, err := f.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status || (want != "" && string(body) != want) {
			t.Fatalf("content %d %q want %d %q", res.StatusCode, body, status, want)
		}
		if status == 200 || status == 206 {
			if res.Header.Get("Accept-Ranges") != "bytes" || (status == 206 && !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes ")) || (method == "HEAD" && res.Header.Get("Content-Length") != "10") {
				t.Fatalf("range/HEAD headers %+v", res.Header)
			}
			if res.Header.Get("Content-Type") != "video/mp4" || res.Header.Get("Cache-Control") != "private, no-store" || res.Header.Get("Content-Encoding") != "" {
				t.Fatalf("headers %+v", res.Header)
			}
			if method == "HEAD" && len(body) != 0 {
				t.Fatal("HEAD body")
			}
		}
	}
	fetch("GET", url, "", 200, "0123456789")
	fetch("GET", url, "bytes=2-5", 206, "2345")
	fetch("GET", url, "bytes=-3", 206, "789")
	fetch("HEAD", url, "", 200, "")
	fetch("GET", url, "bytes=99-100", 416, "")
	fetch("GET", strings.Replace(url, "action=play", "action=download", 1), "", 403, "")
	expired, _ := media.IssueAccess(secret, int32(pid), int32(asset), "play", time.Now().Add(-time.Hour), 120)
	fetch("GET", expired.URL, "", 403, "")
	// Published evidence turns the same asset into a sensitive download.
	if _, err := f.db.Exec("insert into evidence_links(project_id,team_id,target_type,target_id,asset_id,asset_version,asset_checksum_sha256,is_published) values($1,$2,'issue','case',$3,1,$4,true)", pid, team, asset, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	download := issue("download", 200)["url"].(string)
	fetch("GET", download, "", 200, "0123456789")
	var n int
	if err := f.db.QueryRow("select count(*) from audit_events where action='media.sensitive_download' and status='completed'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit %d %v", n, err)
	}
	f.server.credentialSecret = ""
	issue("download", 403)
	f.server.credentialSecret = secret
	if err := f.db.QueryRow("select count(*) from audit_events where action='media.sensitive_download'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rollback %d %v", n, err)
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	issue("preview", 200)
	issue("download", 403)
	fetch("GET", download, "", 403, "")
	if _, err := f.db.Exec("insert into project_permissions(project_id,team_id,user_id,permission) select $1,$2,id,'event:handle' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	issue("download", 200)
	fetch("GET", download, "", 200, "0123456789")
	if _, err := f.db.Exec("update assets set storage_key=$2 where id=$1", asset, fmt.Sprintf("projects/%d/../other/video.mp4", pid)); err != nil {
		t.Fatal(err)
	}
	fetch("GET", url, "", 403, "")
	if _, err := f.db.Exec("update assets set storage_key=$2 where id=$1", asset, fmt.Sprintf("projects/%d/video.mp4", pid)); err != nil {
		t.Fatal(err)
	}
	_, other := f.project(t)
	fetch("GET", strings.Replace(url, fmt.Sprintf("projects/%d/", pid), fmt.Sprintf("projects/%d/", other), 1), "", 403, "")
	if _, err := f.db.Exec("delete from team_members where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	fetch("GET", url, "", 403, "")
	issue("play", 403)
}
