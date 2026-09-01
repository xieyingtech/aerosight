package flighthub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDefaultLiveSupplierRegistryNormalizesProviderMatrixWithoutSerializingSecrets(t *testing.T) {
	cases, _ := loadLiveFixture(t)
	ctx := context.Background()
	for _, item := range []struct {
		name           string
		supplier       string
		protocol       string
		credentialKind string
	}{
		{name: "live-start-volc", supplier: "volc", protocol: "volc-rtc", credentialKind: "sdk-query"},
		{name: "live-start-agora", supplier: "agora", protocol: "agora-rtc", credentialKind: "sdk-credential"},
		{name: "live-start-srs", supplier: "srs", protocol: "hls", credentialKind: "signed-url"},
	} {
		t.Run(item.supplier, func(t *testing.T) {
			fixture := cases[item.name]
			var request LiveStreamStartRequest
			if err := json.Unmarshal(fixture.RequestBody, &request); err != nil {
				t.Fatal(err)
			}
			client := liveFixtureClient(t, fixture)
			authorization, err := client.StartLiveStream(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", request)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := NewDefaultLiveSupplierRegistry(client)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(registry.Suppliers(), ","); got != "agora,srs,volc" {
				t.Fatalf("suppliers=%q", got)
			}
			normalized, err := registry.Normalize(authorization)
			if err != nil || normalized.Description.Supplier != item.supplier || normalized.Description.Protocol != item.protocol ||
				normalized.Description.CredentialKind != item.credentialKind || normalized.Description.AdapterVersion != "v1" ||
				len(normalized.Description.ReferenceDigest) != 64 || !normalized.Description.ExpiresAt.Equal(authorization.ExpiresAt) {
				t.Fatalf("normalized=%#v err=%v", normalized, err)
			}
			if normalized.Secret.Reveal() != authorization.URL || fmt.Sprint(normalized.Secret) != "[REDACTED]" || fmt.Sprintf("%#v", normalized.Secret) != "[REDACTED]" {
				t.Fatal("supplier secret was unavailable or had an unsafe string representation")
			}
			serialized, err := json.Marshal(normalized)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(serialized), authorization.URL) || strings.Contains(string(serialized), "TOKEN_REDACTED") ||
				!strings.Contains(string(serialized), `"secret":{"redacted":true}`) {
				t.Fatalf("normalized playback leaked or omitted secret marker: %s", serialized)
			}
		})
	}
}

func TestLiveSupplierRegistryFailsClosedForUnknownMalformedForbiddenAndExpiredInputs(t *testing.T) {
	now := time.Unix(1779440000, 0).UTC()
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("supplier normalization must not call upstream")
		return nil, nil
	}), func(config *Config) {
		config.Now = func() time.Time { return now }
		config.AllowedLinkHosts = []string{"media.vendor.example"}
	})
	registry, err := NewDefaultLiveSupplierRegistry(client)
	if err != nil {
		t.Fatal(err)
	}
	validExpiry := now.Add(time.Hour)
	for _, item := range []struct {
		name  string
		input LiveStreamAuthorization
		code  string
	}{
		{
			name:  "unknown supplier",
			input: LiveStreamAuthorization{ExpireTimestamp: validExpiry.Unix(), ExpiresAt: validExpiry, URL: "UNKNOWN_SECRET_REDACTED", URLType: "future-provider-secret"},
			code:  "live_supplier_unsupported",
		},
		{
			name:  "malformed volc credential",
			input: LiveStreamAuthorization{ExpireTimestamp: validExpiry.Unix(), ExpiresAt: validExpiry, URL: "app_id=APP_REDACTED&room_id=ROOM_REDACTED&user_id=USER_REDACTED&expire_time=1779443600", URLType: "volc"},
			code:  "live_supplier_schema_incompatible",
		},
		{
			name:  "malformed agora credential",
			input: LiveStreamAuthorization{ExpireTimestamp: validExpiry.Unix(), ExpiresAt: validExpiry, URL: `{"app_id":"APP_REDACTED","token":["TOKEN_REDACTED"]}`, URLType: "agora"},
			code:  "live_supplier_schema_incompatible",
		},
		{
			name:  "forbidden SRS host",
			input: LiveStreamAuthorization{ExpireTimestamp: validExpiry.Unix(), ExpiresAt: validExpiry, URL: "https://attacker.example/live/SECRET_REDACTED.m3u8", URLType: "srs"},
			code:  "temporary_link_host_forbidden",
		},
		{
			name:  "expired SRS URL",
			input: LiveStreamAuthorization{ExpireTimestamp: now.Unix(), ExpiresAt: now, URL: "https://media.vendor.example/live/SECRET_REDACTED.m3u8", URLType: "srs"},
			code:  "temporary_link_expired",
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := registry.Normalize(item.input)
			if !IsSafeCode(err, item.code) || strings.Contains(err.Error(), item.input.URL) || strings.Contains(err.Error(), item.input.URLType) {
				t.Fatalf("error=%v want=%s", err, item.code)
			}
		})
	}
}

func TestSRSAdapterClassifiesBrowserProtocolsWithoutExposingURL(t *testing.T) {
	now := time.Unix(1779440000, 0).UTC()
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("supplier normalization must not call upstream")
		return nil, nil
	}), func(config *Config) {
		config.Now = func() time.Time { return now }
		config.AllowedLinkHosts = []string{"media.vendor.example"}
	})
	registry, err := NewDefaultLiveSupplierRegistry(client)
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	for _, item := range []struct {
		raw      string
		protocol string
	}{
		{raw: "https://media.vendor.example/live/STREAM_REDACTED.m3u8", protocol: "hls"},
		{raw: "https://media.vendor.example/live/STREAM_REDACTED.flv", protocol: "http-flv"},
		{raw: "https://media.vendor.example/live/STREAM_REDACTED?schema=webrtc", protocol: "webrtc"},
		{raw: "https://media.vendor.example/live/STREAM_REDACTED", protocol: "srs-https"},
	} {
		normalized, err := registry.Normalize(LiveStreamAuthorization{
			ExpireTimestamp: expires.Unix(), ExpiresAt: expires, URL: item.raw, URLType: "srs",
		})
		if err != nil || normalized.Description.Protocol != item.protocol || normalized.Secret.Reveal() != item.raw {
			t.Fatalf("raw=%q normalized=%#v err=%v", item.raw, normalized, err)
		}
	}
}

type liveSupplierAdapterFixture struct{ supplier string }

func (fixture liveSupplierAdapterFixture) Supplier() string { return fixture.supplier }

func (fixture liveSupplierAdapterFixture) Normalize(*Client, LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	return NormalizedLivePlayback{}, nil
}

func TestLiveSupplierRegistryRejectsEmptyDuplicateAndUnsafeRegistrations(t *testing.T) {
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil }), nil)
	for _, adapters := range [][]LiveSupplierAdapter{
		nil,
		{liveSupplierAdapterFixture{supplier: "volc"}, liveSupplierAdapterFixture{supplier: "volc"}},
		{liveSupplierAdapterFixture{supplier: "unsafe supplier"}},
	} {
		if _, err := NewLiveSupplierRegistry(client, adapters...); !IsSafeCode(err, "live_supplier_registry_invalid") {
			t.Fatalf("registry error=%v adapters=%#v", err, adapters)
		}
	}
	if _, err := NewLiveSupplierRegistry(nil, liveSupplierAdapterFixture{supplier: "volc"}); !IsSafeCode(err, "live_supplier_registry_invalid") {
		t.Fatalf("nil client registry error=%v", err)
	}
}
