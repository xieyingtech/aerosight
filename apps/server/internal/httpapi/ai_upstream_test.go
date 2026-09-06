package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

type aiTestTransport func(*http.Request) (*http.Response, error)

func (f aiTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type unreadAIProbeBody struct{ closed bool }

func (*unreadAIProbeBody) Read([]byte) (int, error) {
	panic("models health check must not read upstream body")
}
func (b *unreadAIProbeBody) Close() error { b.closed = true; return nil }

func TestAIModelsSDKProbe(t *testing.T) {
	for _, status := range []int{200, 204, 401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			body := &unreadAIProbeBody{}
			client := &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != "https://ai.example/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatalf("request %s %s", r.Method, r.URL)
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("missing bounded deadline")
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: r}, nil
			})}
			ok, code := probeAIModels(context.Background(), client, "https://ai.example/v1/", "test-key")
			if ok != (status < 300) || calls != 1 || !body.closed {
				t.Fatalf("probe %v %s calls %d closed %v", ok, code, calls, body.closed)
			}
			if status >= 300 && code != "HTTP_"+fmt.Sprint(status) {
				t.Fatalf("code %s", code)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	if ok, code := probeAIModels(ctx, client, "https://ai.example/v1", "test-key"); ok || code != "AI_PROVIDER_CONNECTION_FAILED" {
		t.Fatalf("cancel %v %s", ok, code)
	}
}

func TestPinnedAITransport(t *testing.T) {
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Location", "https://evil.example/secret")
			w.WriteHeader(302)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	local, _ := url.Parse(upstream.URL)
	target, _ := url.Parse("https://example.com:" + local.Port() + "/v1")
	client := pinnedAIHTTPClient(target, []netip.Addr{netip.MustParseAddr("127.0.0.1")})
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = upstream.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if transport.Proxy != nil {
		t.Fatal("proxy must not bypass pinned DNS")
	}
	if _, err := transport.DialContext(context.Background(), "tcp", "evil.example:"+local.Port()); err == nil {
		t.Fatal("changed destination accepted")
	}
	res, err := client.Get("https://example.com:" + local.Port() + "/ok")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if ok, code := probeAIModels(context.Background(), client, target.String(), "test-key"); ok || code != "AI_PROVIDER_CONNECTION_FAILED" || calls != 2 {
		t.Fatalf("redirect %v %s calls %d", ok, code, calls)
	}
}

func TestAIProviderHealth(t *testing.T) {
	f := newAPIFixture(t)
	f.server.credentialSecret = "ai-health-test-secret"
	res := f.request(t, "POST", "/api/admin/ai-providers", aiProviderBody)
	provider := decodedResponse(t, res)
	if res.StatusCode != 201 {
		t.Fatalf("create %+v", provider)
	}
	id := provider["id"].(string)
	path := "/api/admin/ai-providers/" + id + "/test"
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	calls, status := 0, 200
	connectionFail := false
	f.server.aiHTTPClientFactory = func(target *url.URL, addresses []netip.Addr) *http.Client {
		if target.String() != "https://api.openai.com/v1" || addresses[0].String() != "8.8.8.8" {
			t.Fatalf("target %s %v", target, addresses)
		}
		return &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Authorization") != "Bearer secret-ai-key" {
				t.Fatal("credential mismatch")
			}
			if connectionFail {
				return nil, errors.New("upstream-secret-error")
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("secret-upstream-body")), Request: r}, nil
		})}
	}
	check := func(want string) {
		t.Helper()
		res := f.request(t, "POST", path, "")
		health := decodedResponse(t, res)
		if res.StatusCode != 200 || health["code"] != want || health["checkedAt"] == nil {
			t.Fatalf("health %d %+v", res.StatusCode, health)
		}
		var saved, status string
		if err := f.db.QueryRow("select health_json->>'code',status from ai_providers where id=$1", id).Scan(&saved, &status); err != nil || saved != want || (status == "healthy") != (want == "OK") {
			t.Fatalf("saved %s %s %v", saved, status, err)
		}
	}
	check("OK")
	status = 401
	check("HTTP_401")
	status = 500
	check("HTTP_500")
	connectionFail = true
	check("AI_PROVIDER_CONNECTION_FAILED")
	if calls != 4 {
		t.Fatalf("SDK retried %d", calls)
	}
	var audits int
	if err := f.db.QueryRow("select count(*) from platform_audit_events where action='ai_provider.test'").Scan(&audits); err != nil || audits != 4 {
		t.Fatalf("audits %d %v", audits, err)
	}
	f.server.networkResolver = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	res = f.request(t, "POST", path, "")
	denied := decodedResponse(t, res)
	if res.StatusCode != 400 || denied["error"] != "OUTBOUND_ADDRESS_RESTRICTED" || calls != 4 {
		t.Fatalf("SSRF %d %+v", res.StatusCode, denied)
	}
	if _, err := f.db.Exec("update users set role='user' where email='admin@example.com'"); err != nil {
		t.Fatal(err)
	}
	res = f.request(t, "POST", path, "")
	denied = decodedResponse(t, res)
	if res.StatusCode != 403 || calls != 4 {
		t.Fatalf("denied %d %+v", res.StatusCode, denied)
	}
}
