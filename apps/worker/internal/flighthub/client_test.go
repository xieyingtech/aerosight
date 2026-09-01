package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func response(status int, body []byte, headers map[string]string) *http.Response {
	values := http.Header{}
	for key, value := range headers {
		values.Set(key, value)
	}
	return &http.Response{StatusCode: status, Header: values, Body: io.NopCloser(strings.NewReader(string(body)))}
}

func fixtureCases(t *testing.T) map[string]contractCase {
	fixture, _ := loadContractFixture(t)
	result := make(map[string]contractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		result[item.Name] = item
	}
	return result
}

func testClient(t *testing.T, transport http.RoundTripper, configure func(*Config)) *Client {
	t.Helper()
	config := Config{
		Timeout: 500 * time.Millisecond, MaxRetries: 0, MaxProjectPages: 3, MaxResponseBytes: 32 << 10,
		HTTPClient: &http.Client{Transport: transport}, RequestID: func() string { return "request-redacted" },
		Sleep:  func(context.Context, time.Duration) error { return nil },
		Jitter: func(time.Duration) time.Duration { return 0 }, MaxConcurrent: 4, RequestsPerSecond: 100, RequestBurst: 100,
	}
	if configure != nil {
		configure(&config)
	}
	client, err := NewChinaClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientUsesOfficialScopedHeaders(t *testing.T) {
	cases := fixtureCases(t)
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Scheme+"://"+request.URL.Host != ChinaAPIOrigin || request.URL.Path != "/openapi/v2.0/project/device" {
			t.Fatalf("unexpected upstream target %s", request.URL)
		}
		if request.Header.Get("X-User-Token") != "secret-token" || request.Header.Get("X-Project-Uuid") != "project-redacted" ||
			request.Header.Get("X-Request-Id") == "" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing FlightHub request headers: %#v", request.Header)
		}
		if strings.Contains(request.URL.String(), "secret-token") {
			t.Fatal("token leaked into FlightHub URL")
		}
		return response(cases["device-directory"].HTTPStatus, cases["device-directory"].Body, nil), nil
	}), nil)
	topologies, err := client.ListDevices(context.Background(), "secret-token", "project-redacted")
	if err != nil || len(topologies) != 1 || topologies[0].Gateway == nil || topologies[0].Drone == nil {
		t.Fatalf("unexpected device directory: %#v, %v", topologies, err)
	}
}

func TestSharedTransportSupportsConstrainedJSONMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != method || request.URL.Path != "/openapi/v2.0/model/item-redacted" || request.URL.Query().Get("page") != "2" {
					t.Fatalf("unexpected request %s %s", request.Method, request.URL)
				}
				var contents []byte
				if request.Body != nil {
					var err error
					contents, err = io.ReadAll(request.Body)
					if err != nil {
						t.Fatal(err)
					}
				}
				if method == http.MethodGet {
					if len(contents) != 0 || request.Header.Get("Content-Type") != "" {
						t.Fatalf("GET carried JSON body or content type: %q %#v", contents, request.Header)
					}
				} else if string(contents) != `{"enabled":true}` || request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected JSON body/header: %q %#v", contents, request.Header)
				}
				return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
			}), nil)
			path, err := resolvePathTemplate("/openapi/v2.0/model/{model_id}", map[string]string{"model_id": "item-redacted"})
			if err != nil {
				t.Fatal(err)
			}
			var body any
			if method != http.MethodGet {
				body = map[string]any{"enabled": true}
			}
			if _, err := client.request(context.Background(), "redacted", "project-redacted", requestSpec{
				Method: method, Path: path, Query: url.Values{"page": {"2"}}, Body: body,
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSharedTransportRejectsUntrustedPathsAndRedirects(t *testing.T) {
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached transport")
		return nil, nil
	}), nil)
	for _, path := range []string{
		"https://attacker.example/openapi/v2.0/project", "/openapi/v2.0/../secret",
		"/openapi/v2.0/project?next=https://attacker.example", "/openapi/v2.0/{unresolved}",
	} {
		if _, err := client.request(context.Background(), "redacted", "", requestSpec{Method: http.MethodGet, Path: path}); !IsSafeCode(err, "request_invalid") {
			t.Fatalf("path %q error=%v", path, err)
		}
	}
	if _, err := resolvePathTemplate("/openapi/v2.0/device/{device_sn}/state", map[string]string{"device_sn": "bad/value"}); !IsSafeCode(err, "request_invalid") {
		t.Fatalf("unsafe template value error=%v", err)
	}
	redirect := &http.Request{URL: &url.URL{Scheme: "https", Host: "attacker.example"}}
	if err := client.httpClient.CheckRedirect(redirect, nil); err == nil || !strings.Contains(err.Error(), "REDIRECT_FORBIDDEN") {
		t.Fatalf("redirect policy error=%v", err)
	}
}

func TestEndpointBusinessCodeProfiles(t *testing.T) {
	cases := fixtureCases(t)
	for _, item := range []struct {
		name      string
		profile   string
		wantCode  string
		wantEmpty bool
	}{
		{name: "live-share-empty", profile: "live-share-list", wantEmpty: true},
		{name: "control-context-required", wantCode: "configuration_required"},
	} {
		t.Run(item.name, func(t *testing.T) {
			fixture := cases[item.name]
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(fixture.HTTPStatus, fixture.Body, nil), nil
			}), nil)
			payload, err := client.request(context.Background(), "redacted", "project-redacted", requestSpec{
				Method: http.MethodGet, Path: fixture.Endpoint, Profile: item.profile,
			})
			if item.wantCode != "" {
				if !IsSafeCode(err, item.wantCode) {
					t.Fatalf("error=%v want=%s", err, item.wantCode)
				}
				return
			}
			if err != nil || payload.Empty != item.wantEmpty {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
		})
	}
}

func TestTemporaryLinkPurposeHostAndExpiryValidation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("temporary link validation must not perform network I/O")
		return nil, nil
	}), func(config *Config) {
		config.Now = func() time.Time { return now }
		config.AllowedLinkHosts = []string{"objects.vendor.example", "media.vendor.example"}
	})
	validated, err := client.ValidateTemporaryLink(LinkDownload, "https://objects.vendor.example/item?signature=redacted", now.Add(15*time.Minute))
	if err != nil || validated.Hostname() != "objects.vendor.example" {
		t.Fatalf("valid temporary link=%v err=%v", validated, err)
	}
	for _, item := range []struct {
		name    string
		purpose LinkPurpose
		raw     string
		expires time.Time
		code    string
	}{
		{name: "unknown purpose", purpose: "other", raw: "https://objects.vendor.example/item", expires: now.Add(time.Minute), code: "temporary_link_invalid"},
		{name: "http", purpose: LinkUpload, raw: "http://objects.vendor.example/item", expires: now.Add(time.Minute), code: "temporary_link_invalid"},
		{name: "userinfo", purpose: LinkModel, raw: "https://user@objects.vendor.example/item", expires: now.Add(time.Minute), code: "temporary_link_invalid"},
		{name: "forbidden host", purpose: LinkLive, raw: "https://attacker.example/item", expires: now.Add(time.Minute), code: "temporary_link_host_forbidden"},
		{name: "expired", purpose: LinkDownload, raw: "https://objects.vendor.example/item", expires: now, code: "temporary_link_expired"},
		{name: "too long", purpose: LinkDownload, raw: "https://objects.vendor.example/item", expires: now.Add(25 * time.Hour), code: "temporary_link_expired"},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := client.ValidateTemporaryLink(item.purpose, item.raw, item.expires); !IsSafeCode(err, item.code) || strings.Contains(err.Error(), item.raw) {
				t.Fatalf("error=%v want=%s", err, item.code)
			}
		})
	}
}

func TestClientConsumesSharedErrorAndSchemaFixtures(t *testing.T) {
	cases := fixtureCases(t)
	expected := map[string]string{
		"http-401": "credential_invalid", "business-200401": "credential_invalid",
		"http-403": "scope_forbidden", "http-404": "scope_not_found",
		"http-429": "rate_limited", "http-503": "upstream_unavailable",
		"malformed-response": "schema_incompatible",
	}
	for name, safeCode := range expected {
		t.Run(name, func(t *testing.T) {
			item := cases[name]
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(item.HTTPStatus, item.Body, item.Headers), nil
			}), nil)
			_, err := client.ListDevices(context.Background(), "redacted", "project-redacted")
			if !IsSafeCode(err, safeCode) || strings.Contains(err.Error(), "redacted") {
				t.Fatalf("case %s error = %v, want safe code %s", name, err, safeCode)
			}
		})
	}
}

func TestClientRejectsDeviceDirectoryAtOfficialLimit(t *testing.T) {
	cases := fixtureCases(t)
	var template envelope
	if err := json.Unmarshal(cases["device-directory"].Body, &template); err != nil {
		t.Fatal(err)
	}
	var data struct {
		List []json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(template.Data, &data); err != nil || len(data.List) != 1 {
		t.Fatalf("invalid device fixture template: %v", err)
	}
	list := make([]json.RawMessage, DeviceDirectoryLimit)
	for index := range list {
		list[index] = data.List[0]
	}
	template.Data, _ = json.Marshal(map[string]any{"list": list})
	body, _ := json.Marshal(template)
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, body, nil), nil
	}), func(config *Config) { config.MaxResponseBytes = 2 << 20 })
	_, err := client.ListDevices(context.Background(), "redacted", "project-redacted")
	if !IsSafeCode(err, "directory_limit_reached") {
		t.Fatalf("limit response error = %v", err)
	}
}

func TestClientRetriesRateLimitAndServerFailure(t *testing.T) {
	cases := fixtureCases(t)
	sequence := []contractCase{cases["http-429"], cases["http-503"], cases["project-empty"]}
	var mu sync.Mutex
	requests := 0
	delays := []time.Duration{}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		item := sequence[requests]
		requests++
		return response(item.HTTPStatus, item.Body, item.Headers), nil
	}), func(config *Config) {
		config.MaxRetries = 2
		config.Sleep = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	})
	projects, err := client.ListProjects(context.Background(), "redacted")
	if err != nil || len(projects) != 0 || requests != 3 {
		t.Fatalf("retry result projects=%#v requests=%d error=%v", projects, requests, err)
	}
	if len(delays) != 2 || delays[0] != 3*time.Second || delays[1] != 500*time.Millisecond {
		t.Fatalf("unexpected retry delays %#v", delays)
	}
}

func TestSharedRequestGateUsesBurstRateAndConcurrencyLimits(t *testing.T) {
	t.Run("token bucket with fake clock", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		delays := []time.Duration{}
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
		}), func(config *Config) {
			config.Now = func() time.Time { return now }
			config.RequestsPerSecond = 2
			config.RequestBurst = 1
			config.MaxConcurrent = 1
			config.Sleep = func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				now = now.Add(delay)
				return nil
			}
		})
		for range 3 {
			if _, err := client.request(context.Background(), "redacted", "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/health"}); err != nil {
				t.Fatal(err)
			}
		}
		if len(delays) != 2 || delays[0] != 500*time.Millisecond || delays[1] != 500*time.Millisecond {
			t.Fatalf("token bucket delays=%v", delays)
		}
	})

	t.Run("maximum concurrency", func(t *testing.T) {
		entered := make(chan struct{}, 2)
		release := make(chan struct{})
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			entered <- struct{}{}
			<-release
			return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
		}), func(config *Config) { config.MaxConcurrent = 1 })
		done := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := client.request(context.Background(), "redacted", "", requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/health"})
				done <- err
			}()
		}
		<-entered
		select {
		case <-entered:
			t.Fatal("second request bypassed maximum concurrency")
		case <-time.After(20 * time.Millisecond):
		}
		close(release)
		<-entered
		for range 2 {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestProjectPaginationFailsClosedOnDuplicateAndPartialPages(t *testing.T) {
	page := make([]Project, ProjectPageSize)
	for index := range page {
		page[index] = Project{UUID: "project-" + strconv.Itoa(index), OrganizationUUID: "organization-redacted", Name: "项目"}
	}
	body, _ := json.Marshal(map[string]any{"code": 0, "message": "", "data": map[string]any{"list": page}})
	t.Run("duplicate page", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, body, nil), nil
		}), nil)
		if _, err := client.ListProjects(context.Background(), "redacted"); !IsSafeCode(err, "schema_incompatible") {
			t.Fatalf("duplicate page error=%v", err)
		}
	})
	t.Run("partial page failure", func(t *testing.T) {
		requests := 0
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return response(http.StatusOK, body, nil), nil
			}
			return response(http.StatusServiceUnavailable, []byte(`{"code":210500,"message":"unavailable","data":{}}`), nil), nil
		}), nil)
		projects, err := client.ListProjects(context.Background(), "redacted")
		if projects != nil || !IsSafeCode(err, "upstream_unavailable") {
			t.Fatalf("partial page returned projects=%#v err=%v", projects, err)
		}
	})
}

func TestClientTimeoutResponseLimitAndSchemaDrift(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}), nil)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := client.ListProjects(ctx, "redacted")
		if !IsSafeCode(err, "request_timeout") {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("response limit", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(`{"code":0,"data":{"list":[],"padding":"`+strings.Repeat("x", 2000)+`"}}`), nil), nil
		}), func(config *Config) { config.MaxResponseBytes = 1024 })
		_, err := client.ListProjects(context.Background(), "redacted")
		if !IsSafeCode(err, "response_too_large") {
			t.Fatalf("response limit error = %v", err)
		}
	})

	t.Run("schema drift", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(`{"code":0,"data":{"devices":[]}}`), nil), nil
		}), nil)
		_, err := client.ListDevices(context.Background(), "redacted", "project-redacted")
		if !IsSafeCode(err, "schema_incompatible") {
			t.Fatalf("schema drift error = %v", err)
		}
	})

	t.Run("transport", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("sensitive network detail")
		}), nil)
		_, err := client.ListProjects(context.Background(), "redacted")
		if !IsSafeCode(err, "upstream_unavailable") || strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("transport error = %v", err)
		}
	})
}
