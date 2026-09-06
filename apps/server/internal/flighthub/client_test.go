package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
		Sleep: func(context.Context, time.Duration) error { return nil },
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
