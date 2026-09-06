package httpapi

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/database"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type apiFixture struct {
	db     *sql.DB
	server *Server
	host   *httptest.Server
	client *http.Client
	csrf   string
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	db := testdb.New(t)
	if _, err := migrations.Embedded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := database.Bootstrap(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cfg := config.HTTP{PublicOrigin: "http://frontend.test", Development: true, CSRFKey: bytes.Repeat([]byte{3}, 32), SessionLifetime: time.Hour, SessionIdle: time.Hour, LoginLimit: 10, RequestTimeout: time.Second, MetricsToken: "test-metrics"}
	s, err := New(db, cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	jar, _ := cookiejar.New(nil)
	f := &apiFixture{db: db, server: s, host: ts, client: &http.Client{Jar: jar, Timeout: 25 * time.Second}}
	res := f.request(t, "GET", "/api/auth/csrf", "")
	var token map[string]string
	if err = json.NewDecoder(res.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	f.csrf = token["csrfToken"]
	res = f.request(t, "POST", "/api/auth/login", `{"username":"admin@example.com","password":"admin"}`)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("login %d", res.StatusCode)
	}
	return f
}
func (f *apiFixture) request(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	r, _ := http.NewRequest(method, f.host.URL+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://frontend.test")
	r.Header.Set("X-CSRF-Token", f.csrf)
	res, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
func (f *apiFixture) project(t *testing.T) (int, int) {
	t.Helper()
	var team, project int
	if err := f.db.QueryRow("insert into teams(name) values('stream test') returning id").Scan(&team); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into team_members(team_id,user_id,role) select $1,id,'owner' from users where email='admin@example.com'", team); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into projects(team_id,name) values($1,'stream test') returning id", team).Scan(&project); err != nil {
		t.Fatal(err)
	}
	return team, project
}

func (f *apiFixture) device(t *testing.T, team, pid int) (int, int) {
	t.Helper()
	var adapter, device int
	if err := f.db.QueryRow("insert into device_adapters(project_id,team_id,name,adapter_type) values($1,$2,'sim','simulator') returning id", pid, team).Scan(&adapter); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow("insert into devices(project_id,name,type,adapter_id,device_type_id,status,last_seen_at) select $1,'test device','drone',$2,id,'online',now() from device_types where type_key='legacy.device' returning id", pid, adapter).Scan(&device); err != nil {
		t.Fatal(err)
	}
	return adapter, device
}

func TestChannelResumeBackpressureAndRevocation(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	adapter, device := f.device(t, team, pid)
	if _, err := f.db.Exec("insert into device_capabilities(device_id,project_id,capability_code) values($1,$2,'stream.telemetry')", device, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_stream_channels(project_id,team_id,device_id,stable_channel_id,capability_code,channel_key,display_name,data_type) values($1,$2,$3,'test-channel','stream.telemetry','test','Test','telemetry')", pid, team, device); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("insert into device_telemetry(project_id,team_id,adapter_id,device_id,event_id,telemetry_type,captured_at,payload_json) select $1,$2,$3,$4,'sample-'||n,'position',now(),jsonb_build_object('sequence',n) from generate_series(1,502) n", pid, team, adapter, device); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/projects/%d/realtime-channels/test-channel/events", pid)
	res := f.request(t, "GET", path, "")
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !strings.Contains(string(body), "backpressure_limit_exceeded") {
		t.Fatalf("channel overflow %s %v", body, err)
	}
	res = f.request(t, "GET", path+"?cursor=501", "")
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	found := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"eventId":"sample-502"`) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sample resume: %v", scanner.Err())
	}
	if f.db.Stats().InUse != 0 {
		t.Fatal("channel holds pool connection")
	}
	if _, err := f.db.Exec("insert into device_capability_grants(project_id,team_id,user_id,scope_type,action_pattern,effect) select $1,$2,id,'project','stream.*','deny' from users where email='admin@example.com'", pid, team); err != nil {
		t.Fatal(err)
	}
	found = false
	for scanner.Scan() {
		if scanner.Text() == "event: access.revoked" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("channel deny not rechecked: %v", scanner.Err())
	}
	denied := f.request(t, "GET", path, "")
	denied.Body.Close()
	if denied.StatusCode != 404 {
		t.Fatalf("channel scope leak %d", denied.StatusCode)
	}
}
func TestProjectStreamResumeOverflowAndLogout(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	if _, err := f.db.Exec("insert into project_events(project_id,team_id,event_id,event_type) select $1,$2,'event-'||n,'device.updated' from generate_series(1,502) n", pid, team); err != nil {
		t.Fatal(err)
	}
	res := f.request(t, "GET", fmt.Sprintf("/api/projects/%d/events", pid), "")
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || !strings.Contains(string(body), "snapshot.required") || !strings.Contains(string(body), `"resumeCursor":"501"`) {
		t.Fatalf("overflow %d %s %v", res.StatusCode, body, err)
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/projects/%d/events?cursor=0", f.host.URL, pid), nil)
	req.Header.Set("Last-Event-ID", "501")
	res, err = f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	found := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"eventId":"event-502"`) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("resume failed %v", scanner.Err())
	}
	if f.db.Stats().InUse != 0 {
		t.Fatalf("stream holds connection: %+v", f.db.Stats())
	}
	logout := f.request(t, "POST", "/api/auth/logout", "")
	logout.Body.Close()
	revoked := false
	for scanner.Scan() {
		if scanner.Text() == "event: access.revoked" {
			revoked = true
			break
		}
	}
	if !revoked {
		t.Fatalf("logout stream not revoked: %v", scanner.Err())
	}
}

func TestMetricsAuthorizationAndRouteLabels(t *testing.T) {
	f := newAPIFixture(t)
	res, err := http.Get(f.host.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("public metrics %d", res.StatusCode)
	}
	res = f.request(t, "GET", "/api/projects/123456", "")
	res.Body.Close()
	req, _ := http.NewRequest("GET", f.host.URL+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-metrics")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `route="/api/projects/:id"`) || strings.Contains(string(body), "123456") || !strings.Contains(string(body), "go_goroutines") {
		t.Fatalf("metrics contract %d %s", res.StatusCode, body)
	}
}
