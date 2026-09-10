package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFDenialsPreserveSessionAndData(t *testing.T) {
	f := newAPIFixture(t)
	other, err := http.Get(f.host.URL + "/api/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	var alien struct {
		Token string `json:"csrfToken"`
	}
	if err = json.NewDecoder(other.Body).Decode(&alien); err != nil {
		t.Fatal(err)
	}
	other.Body.Close()
	var before, after int
	if err = f.db.QueryRow("select count(*) from teams").Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/auth/login", "/api/auth/logout", "/api/teams"} {
		for _, tc := range []struct{ token, origin string }{
			{"", "http://frontend.test"}, {"invalid-token", "http://frontend.test"}, {alien.Token, "http://frontend.test"}, {f.csrf, "http://evil.test"}, {f.csrf, "null"},
		} {
			body := `{"name":"must not exist"}`
			if path == "/api/auth/login" {
				body = `{"username":"admin@example.com","password":"admin"}`
			}
			r, _ := http.NewRequest("POST", f.host.URL+path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", tc.token)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-Forwarded-Host", "frontend.test")
			r.Header.Set("X-Forwarded-Proto", "https")
			res, err := f.client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			data := decodedResponse(t, res)
			if res.StatusCode != 403 || data["error"] != "CSRF_FAILED" {
				t.Fatalf("%s origin=%s status=%d %+v", path, tc.origin, res.StatusCode, data)
			}
			if res.Header.Get("X-Request-ID") == "" || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("denial missing boundary headers")
			}
		}
	}
	if err = f.db.QueryRow("select count(*) from teams").Scan(&after); err != nil || before != after {
		t.Fatalf("CSRF rejection mutated data %d %d %v", before, after, err)
	}
	res := f.request(t, "GET", "/api/auth/session", "")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("rejected logout destroyed session: %d", res.StatusCode)
	}
	res = f.request(t, "POST", "/api/teams", `{"name":"authorized write"}`)
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("valid write %d", res.StatusCode)
	}
	res = f.request(t, "POST", "/api/auth/logout", "")
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatalf("valid logout %d", res.StatusCode)
	}
}

func TestProductionCSRFCookiesAndLogin(t *testing.T) {
	f := newAPIFixture(t)
	cfg := f.server.cfg
	cfg.Development = false
	cfg.PublicOrigin = "https://frontend.test"
	s, err := New(f.db, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := httptest.NewTLSServer(s.Handler())
	defer ts.Close()
	client := ts.Client()
	client.Jar, _ = cookiejar.New(nil)
	res, err := client.Get(ts.URL + "/api/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	var token struct {
		Token string `json:"csrfToken"`
	}
	if err = json.NewDecoder(res.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(res.Cookies()) == 0 {
		t.Fatal("missing CSRF cookie")
	}
	for _, cookie := range res.Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" {
			t.Fatalf("insecure CSRF cookie %s", cookie.Name)
		}
	}
	r, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`{"username":"admin@example.com","password":"admin"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", cfg.PublicOrigin)
	r.Header.Set("X-CSRF-Token", token.Token)
	res, err = client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("TLS login %d", res.StatusCode)
	}
	found := false
	for _, cookie := range res.Cookies() {
		if cookie.Name == "aerosight_session" {
			found = true
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("insecure session cookie")
			}
		}
	}
	if !found {
		t.Fatal("session cookie absent")
	}
	res, err = client.Get(ts.URL + "/api/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("TLS session %d", res.StatusCode)
	}
}
