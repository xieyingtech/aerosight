package httpapi

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/database"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginCSRFRestartAndLogout(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	cfg := config.HTTP{PublicOrigin: "http://frontend.test", Development: true, CSRFKey: bytes.Repeat([]byte{1}, 32), SessionLifetime: time.Hour, SessionIdle: time.Hour, LoginLimit: 10, RequestTimeout: time.Second}
	newServer := func() (*Server, *httptest.Server) {
		s, err := New(db, cfg, slog.New(slog.NewJSONHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		ts := httptest.NewServer(s.Handler())
		return s, ts
	}
	s, ts := newServer()
	defer func() { ts.Close(); s.Close() }()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(method, path, body, token, origin string) *http.Response {
		r, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("X-CSRF-Token", token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`, "", cfg.PublicOrigin)
	if response.StatusCode != 403 {
		t.Fatalf("missing csrf: %d", response.StatusCode)
	}
	response.Body.Close()
	response = call("GET", "/api/auth/csrf", "", "", "")
	var token struct {
		Token string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response = call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`, token.Token, "http://evil.test")
	if response.StatusCode != 403 {
		t.Fatalf("wrong origin: %d", response.StatusCode)
	}
	response.Body.Close()
	response = call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`, token.Token, cfg.PublicOrigin)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("login: %d %s", response.StatusCode, body)
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "aerosight_session" && (!cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode) {
			t.Fatal("incorrect development session cookie policy")
		}
	}
	response = call("GET", "/api/auth/session", "", "", "")
	if response.StatusCode != 200 {
		t.Fatalf("session %d", response.StatusCode)
	}
	response.Body.Close()
	ts.Close()
	s.Close()
	s, ts = newServer()
	response = call("GET", "/api/auth/session", "", "", "")
	if response.StatusCode != 200 {
		t.Fatalf("session lost after restart: %d", response.StatusCode)
	}
	response.Body.Close()
	response = call("POST", "/api/auth/logout", "", token.Token, cfg.PublicOrigin)
	if response.StatusCode != 204 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("logout %d %s", response.StatusCode, body)
	}
	response.Body.Close()
	response = call("GET", "/api/auth/session", "", "", "")
	if response.StatusCode != 401 {
		t.Fatalf("logout still authorized: %d", response.StatusCode)
	}
	response.Body.Close()
	if strings.Contains(logs.String(), "password") || strings.Contains(logs.String(), token.Token) {
		t.Fatal("sensitive data logged")
	}
}

func TestDirectoryIsolationAndJSONContracts(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.HTTP{PublicOrigin: "http://frontend.test", Development: true, CSRFKey: bytes.Repeat([]byte{2}, 32), SessionLifetime: time.Hour, SessionIdle: time.Hour, LoginLimit: 10, RequestTimeout: time.Second}
	s, err := New(db, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	var token string
	call := func(method, path, body string) (int, map[string]any, []byte) {
		r, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("X-CSRF-Token", token)
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var data map[string]any
		_ = json.Unmarshal(raw, &data)
		return res.StatusCode, data, raw
	}
	_, data, _ := call("GET", "/api/auth/csrf", "")
	token = data["csrfToken"].(string)
	status, _, raw := call("POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`)
	if status != 200 {
		t.Fatalf("login %d %s", status, raw)
	}
	status, _, raw = call("GET", "/api/teams", "")
	if status != 200 || string(raw) != "[]" {
		t.Fatalf("empty list %d %s", status, raw)
	}
	status, data, raw = call("POST", "/api/teams", `{"name":"Team A"}`)
	if status != 201 {
		t.Fatalf("team %d %s", status, raw)
	}
	team := int(data["id"].(float64))
	body, _ := json.Marshal(map[string]any{"teamId": team, "name": "Project A"})
	status, data, raw = call("POST", "/api/projects", string(body))
	if status != 201 {
		t.Fatalf("project %d %s", status, raw)
	}
	project := int(data["id"].(float64))
	status, data, raw = call("GET", fmt.Sprintf("/api/projects/%d", project), "")
	if status != 200 || data["description"] != nil || data["teamId"] != float64(team) || data["teamName"] != "Team A" {
		t.Fatalf("project contract %d %s", status, raw)
	}
	var outside int
	if err = db.QueryRow("insert into teams(name) values('outside') returning id").Scan(&outside); err != nil {
		t.Fatal(err)
	}
	var outsideProject int
	if err = db.QueryRow("insert into projects(team_id,name) values($1,'outside') returning id", outside).Scan(&outsideProject); err != nil {
		t.Fatal(err)
	}
	status, _, _ = call("GET", fmt.Sprintf("/api/projects/%d", outsideProject), "")
	if status != 404 {
		t.Fatalf("cross-tenant read %d", status)
	}
	body, _ = json.Marshal(map[string]any{"teamId": outside, "name": "forbidden"})
	status, _, _ = call("POST", "/api/projects", string(body))
	if status != 403 {
		t.Fatalf("cross-tenant write %d", status)
	}
}
func TestEffectivePermissions(t *testing.T) {
	if !effectivePermissions("member", []string{"event:handle"})["issue:handle"] {
		t.Fatal("lost alias")
	}
	if effectivePermissions("member", []string{"unknown"})["device:configure"] {
		t.Fatal("member elevated")
	}
	if !effectivePermissions("owner", nil)["mission:approve"] {
		t.Fatal("owner denied")
	}
}

func TestSessionRotationExpiryAndLegacyCookieRejection(t *testing.T) {
	f := newAPIFixture(t)
	parsed, _ := url.Parse(f.host.URL)
	var prior *http.Cookie
	for _, cookie := range f.client.Jar.Cookies(parsed) {
		if cookie.Name == "aerosight_session" {
			prior = cookie
		}
	}
	if prior == nil {
		t.Fatal("missing session cookie")
	}
	// Produced by the original bcryptjs dependency, not Go's password hasher.
	if _, err := f.db.Exec("update users set password=$1 where email='admin@example.com'", "$2b$10$pQBwZtOtVLmuThsrNyQ0je.caGV4ivUyJvS7nMk3vppZVUGQVZozK"); err != nil {
		t.Fatal(err)
	}
	response := f.request(t, "POST", "/api/auth/login", `{"username":"admin@example.com","password":"legacy-test-password"}`)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("second login %d", response.StatusCode)
	}
	req, _ := http.NewRequest("GET", f.host.URL+"/api/auth/session", nil)
	req.AddCookie(prior)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("old token still valid %d", response.StatusCode)
	}
	req, _ = http.NewRequest("GET", f.host.URL+"/api/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "authjs.session-token", Value: prior.Value})
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("legacy cookie accepted %d", response.StatusCode)
	}
	if _, err = f.db.Exec("update sessions set expiry=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	response = f.request(t, "GET", "/api/auth/session", "")
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("expired session accepted %d", response.StatusCode)
	}
}
