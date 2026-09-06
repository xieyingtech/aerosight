package device

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testNetworkProfile() NetworkProfile {
	return NetworkProfile{Mode: "lan", Endpoints: map[string]string{"mqttEndpoint": "mqtt://device.test", "apiPublicBaseUrl": "http://device.test/api", "websocketPublicUrl": "ws://device.test/ws", "mediaIngestBaseUrl": "rtmp://device.test/live", "mediaPlaybackBaseUrl": "http://device.test/video"}}
}
func TestNetworkPolicyAndDiagnostics(t *testing.T) {
	profile := testNetworkProfile()
	var resolutions, probes atomic.Int32
	resolver := func(context.Context, string) ([]netip.Addr, error) {
		resolutions.Add(1)
		return []netip.Addr{netip.MustParseAddr("192.168.1.4")}, nil
	}
	probe := func(_ context.Context, e NetworkEndpoint) error {
		probes.Add(1)
		if e.Addresses[0].String() != "192.168.1.4" {
			return errors.New("wrong address")
		}
		return nil
	}
	result := CheckNetworkConnection(context.Background(), profile, resolver, probe)
	if !result.OK || resolutions.Load() != 1 || probes.Load() != 5 || result.DeviceVerification != "pending" || len(result.PolicyIssues) != 0 {
		t.Fatalf("lan %+v resolve=%d probe=%d", result, resolutions.Load(), probes.Load())
	}
	if result.Diagnostics[4].DeviceVerification != "not_applicable" {
		t.Fatal("playback device verification")
	}
	profile.Mode = "public"
	profile.MQTTAnonymous = true
	probes.Store(0)
	result = CheckNetworkConnection(context.Background(), profile, resolver, probe)
	if result.OK || probes.Load() != 0 || len(result.PolicyIssues) != 13 {
		t.Fatalf("public %+v probes=%d", result, probes.Load())
	}
	profile.TLSRequired = true
	profile.MQTTAnonymous = false
	profile.CredentialProvided = true
	for field, schemes := range endpointSchemes {
		profile.Endpoints[field] = schemes[1] + "://public.test"
	}
	result = CheckNetworkConnection(context.Background(), profile, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	}, func(context.Context, NetworkEndpoint) error { return nil })
	if !result.OK {
		t.Fatalf("valid public %+v", result)
	}
	profile = testNetworkProfile()
	profile.Endpoints["mqttEndpoint"] = "mqtt://user:private@device.test:1883/path?token=secret"
	result = CheckNetworkConnection(context.Background(), profile, resolver, probe)
	if result.OK || result.Diagnostics[0].Endpoint != "mqtt://device.test:1883" || result.Diagnostics[0].Code != "ENDPOINT_INLINE_CREDENTIALS_FORBIDDEN" {
		t.Fatalf("inline secret %+v", result.Diagnostics[0])
	}
	for _, address := range []string{"127.0.0.1", "0.0.0.0", "169.254.169.254", "::1", "fe80::1", "::ffff:127.0.0.1", "ff02::1"} {
		profile = testNetworkProfile()
		probes.Store(0)
		result = CheckNetworkConnection(context.Background(), profile, func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("192.168.1.4"), netip.MustParseAddr(address)}, nil
		}, probe)
		if result.OK || probes.Load() != 0 || result.Diagnostics[0].Code != "ENDPOINT_UNROUTABLE_ADDRESS" {
			t.Fatalf("unroutable %s %+v", address, result)
		}
	}
	profile = testNetworkProfile()
	profile.Endpoints["mqttEndpoint"] = "mqtt://sub.localhost."
	result = CheckNetworkConnection(context.Background(), profile, resolver, probe)
	if result.Diagnostics[0].Code != "ENDPOINT_LOOPBACK_FORBIDDEN" {
		t.Fatalf("localhost %+v", result)
	}
	profile = testNetworkProfile()
	result = CheckNetworkConnection(context.Background(), profile, func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("private dns details") }, probe)
	if result.Diagnostics[0].Code != "ENDPOINT_DNS_FAILED" {
		t.Fatalf("dns %+v", result)
	}
	result = CheckNetworkConnection(context.Background(), profile, resolver, func(context.Context, NetworkEndpoint) error { return errors.New("private probe details") })
	if result.OK || result.Diagnostics[0].Code != "ENDPOINT_PROBE_FAILED" {
		t.Fatalf("probe %+v", result)
	}
}

func TestNetworkProbePinsAddressAndDoesNotRedirect(t *testing.T) {
	var hits atomic.Int32
	var method, host string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		method = r.Method
		host = r.Host
		w.Header().Set("Location", "http://localhost/private")
		w.WriteHeader(302)
	}))
	defer server.Close()
	actual, _ := url.Parse(server.URL)
	target, _ := url.Parse("http://does-not-exist.invalid:" + actual.Port() + "/health")
	endpoint := NetworkEndpoint{URL: target, Addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	if err := ProbeNetworkEndpoint(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 || method != "HEAD" || !strings.HasPrefix(host, "does-not-exist.invalid:") {
		t.Fatalf("probe hits=%d %s %s", hits.Load(), method, host)
	}
	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer unhealthy.Close()
	endpoint.URL, _ = url.Parse(unhealthy.URL)
	if err := ProbeNetworkEndpoint(context.Background(), endpoint); err == nil {
		t.Fatal("unhealthy accepted")
	}
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer secure.Close()
	endpoint.URL, _ = url.Parse(secure.URL)
	if err := ProbeNetworkEndpoint(context.Background(), endpoint); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e == nil {
			defer conn.Close()
			io.Copy(io.Discard, conn)
		}
	}()
	endpoint.URL, _ = url.Parse("mqtts://" + listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := ProbeNetworkEndpoint(ctx, endpoint); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("TLS timeout ignored: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("probe connection not closed")
	}
}
