package httpapi

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAccountSettings(t *testing.T) {
	f := newAPIFixture(t)
	f.server.cfg.RequestTimeout = 10 * time.Second
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		res := f.request(t, method, path, body)
		if res.StatusCode == 204 && status == 204 {
			res.Body.Close()
			return nil
		}
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("%s %s: %d want %d: %v", method, path, res.StatusCode, status, data)
		}
		return data
	}
	call("PATCH", "/api/profile", `{"name":"   "}`, 400)
	call("PATCH", "/api/profile", `{"name":"Alice","id":999}`, 400)
	call("PATCH", "/api/profile", `{"name":"Alice","role":"admin"}`, 400)
	call("PATCH", "/api/profile", `{"name":"`+strings.Repeat("名", 101)+`"}`, 400)
	result := call("PATCH", "/api/profile", `{"name":" 新名称 "}`, 200)
	if result["user"].(map[string]any)["name"] != "新名称" {
		t.Fatal(result)
	}
	result = call("GET", "/api/auth/session", "", 200)
	if result["user"].(map[string]any)["name"] != "新名称" || result["user"].(map[string]any)["email"] != "admin@example.com" {
		t.Fatal(result)
	}
	call("POST", "/api/auth/password", `{"currentPassword":"wrong","newPassword":"new-password"}`, 400)
	for _, password := range []string{"short", strings.Repeat("x", 73), strings.Repeat("密", 25)} {
		call("POST", "/api/auth/password", fmt.Sprintf(`{"currentPassword":"admin","newPassword":%q}`, password), 400)
	}
	call("POST", "/api/auth/password", `{"currentPassword":"admin","newPassword":"new-password","id":999}`, 400)
	// Capture the first session, then log in again to test independent session revocation.
	parsed, _ := url.Parse(f.host.URL)
	oldCookies := f.client.Jar.Cookies(parsed)
	jar, _ := cookiejar.New(nil)
	second := &http.Client{Jar: jar}
	oldClient := f.client
	f.client = second
	res := f.request(t, "GET", "/api/auth/csrf", "")
	csrf := decodedResponse(t, res)["csrfToken"].(string)
	originalCSRF := f.csrf
	f.csrf = csrf
	call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`, 200)
	call("POST", "/api/auth/password", `{"currentPassword":"admin","newPassword":"new-password"}`, 204)
	call("GET", "/api/auth/session", "", 401)
	f.client = oldClient
	f.csrf = originalCSRF
	call("GET", "/api/auth/session", "", 401)
	// Both session tokens are rejected by the fresh realtime session check.
	for _, cookie := range oldCookies {
		if cookie.Name == "aerosight_session" && f.server.streamSessionActive(t.Context(), cookie.Value, 1) {
			t.Fatal("old realtime session survived password change")
		}
	}
	call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`, 401)
	call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"new-password"}`, 200)
	call("GET", "/api/auth/session", "", 200)
	var auditCount int
	if err := f.db.QueryRow(`select count(*) from audit_events where action in ('account.name.update','account.password.update') and status='completed'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audits=%d err=%v", auditCount, err)
	}
}

func TestProjectRenameAuthorization(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	path := fmt.Sprintf("/api/projects/%d", pid)
	call := func(body string, status int) {
		t.Helper()
		res := f.request(t, "PATCH", path, body)
		data := decodedResponse(t, res)
		if res.StatusCode != status {
			t.Fatalf("rename %d want %d: %v", res.StatusCode, status, data)
		}
	}
	call(`{"name":" "}`, 400)
	call(`{"name":"New","teamId":999}`, 400)
	call(`{"name":"`+strings.Repeat("项", 101)+`"}`, 400)
	call(`{"name":" 项目新名称 "}`, 200)
	res := f.request(t, "GET", path, "")
	if decodedResponse(t, res)["name"] != "项目新名称" {
		t.Fatal("name not persisted")
	}
	if _, err := f.db.Exec(`update team_members set role='member' where team_id=$1`, team); err != nil {
		t.Fatal(err)
	}
	call(`{"name":"Denied"}`, 403)
	if _, err := f.db.Exec(`update team_members set role='admin' where team_id=$1`, team); err != nil {
		t.Fatal(err)
	}
	call(`{"name":"Admin edit"}`, 200)
	if _, err := f.db.Exec(`delete from team_members where team_id=$1`, team); err != nil {
		t.Fatal(err)
	}
	call(`{"name":"Outside"}`, 403)
	var name string
	var audits int
	if err := f.db.QueryRow(`select name,(select count(*) from audit_events where project_id=$1 and action='project.name.update') from projects where id=$1`, pid).Scan(&name, &audits); err != nil || name != "Admin edit" || audits != 2 {
		t.Fatalf("name=%s audits=%d err=%v", name, audits, err)
	}
}

func TestSettingsNameUnicode(t *testing.T) {
	for _, input := range []string{"", strings.Repeat("🙂", 101)} {
		if validSettingsName(input) {
			t.Fatal("invalid name accepted")
		}
	}
	if !validSettingsName(strings.Repeat("🙂", 100)) {
		t.Fatal("Unicode code points rejected")
	}
}

func TestSettingsCSRF(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	f.csrf = ""
	for _, target := range []struct{ method, path, body string }{
		{"PATCH", "/api/profile", `{"name":"forged"}`},
		{"POST", "/api/auth/password", `{"currentPassword":"admin","newPassword":"forged-password"}`},
		{"PATCH", fmt.Sprintf("/api/projects/%d", pid), `{"name":"forged"}`},
	} {
		res := f.request(t, target.method, target.path, target.body)
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("missing CSRF %s: %d", target.path, res.StatusCode)
		}
	}
}
