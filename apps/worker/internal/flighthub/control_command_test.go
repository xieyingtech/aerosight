package flighthub

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestExecuteDiscreteControlTreatsHTTPAsAcceptanceOnly(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/openapi/v2.0/device/AIRCRAFT_REDACTED/command" {
			t.Fatalf("unexpected control request %s %s", request.Method, request.URL.Path)
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != `{"device_command":"return_home"}` {
			t.Fatalf("unexpected control body %s", body)
		}
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
	}), nil)
	definition, err := client.ExecuteDiscreteControl(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "return_home", "AIRCRAFT_REDACTED")
	if err != nil || calls != 1 {
		t.Fatalf("unexpected execution definition=%#v calls=%d err=%v", definition, calls, err)
	}
	if definition.ResultSemantics != ControlResultAcceptanceOnly || definition.FinalOnHTTPSuccess {
		t.Fatalf("HTTP acceptance became final: %#v", definition)
	}
}

func TestExecuteDiscreteControlNackAndTimeoutNeverRetry(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name      string
		transport http.RoundTripper
		safeCode  string
	}{
		{"business nack", roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(`{"code":200403,"message":"denied","data":{}}`), nil), nil
		}), "scope_forbidden"},
		{"transport timeout", roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}), "upstream_unavailable"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				return testCase.transport.RoundTrip(request)
			})
			client := testClient(t, transport, func(config *Config) { config.MaxRetries = 3 })
			_, err := client.ExecuteDiscreteControl(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "flighttask_pause", "AIRCRAFT_REDACTED")
			if calls != 1 || !IsSafeCode(err, testCase.safeCode) {
				t.Fatalf("calls=%d err=%v, want one call and %s", calls, err, testCase.safeCode)
			}
		})
	}
}

func TestReconcileDiscreteControlRequiresFreshMatchingTelemetry(t *testing.T) {
	t.Parallel()
	acceptedAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	deadline := acceptedAt.Add(30 * time.Second)
	active, inactive := true, false
	cases := []struct {
		action                 string
		evidence               ControlTelemetryEvidence
		wantStatus, wantReason string
	}{
		{"return_home", ControlTelemetryEvidence{CapturedAt: acceptedAt.Add(time.Second), ReturnHomeActive: &active}, "acknowledged", "telemetry_confirmed"},
		{"return_home_cancel", ControlTelemetryEvidence{CapturedAt: acceptedAt.Add(time.Second), ReturnHomeActive: &inactive}, "sent", "awaiting_fresh_evidence"},
		{"flighttask_pause", ControlTelemetryEvidence{CapturedAt: acceptedAt.Add(time.Second)}, "sent", "awaiting_fresh_evidence"},
		{"flighttask_recovery", ControlTelemetryEvidence{CapturedAt: acceptedAt.Add(time.Second)}, "sent", "awaiting_fresh_evidence"},
		{"return_home", ControlTelemetryEvidence{CapturedAt: acceptedAt, ReturnHomeActive: &active}, "sent", "awaiting_fresh_evidence"},
		{"return_home", ControlTelemetryEvidence{CapturedAt: acceptedAt.Add(time.Second), ReturnHomeActive: &inactive}, "sent", "awaiting_fresh_evidence"},
	}
	for _, testCase := range cases {
		status, reason := ReconcileDiscreteControl(testCase.action, acceptedAt, deadline, acceptedAt.Add(2*time.Second), testCase.evidence)
		if status != testCase.wantStatus || reason != testCase.wantReason {
			t.Errorf("%s => %s/%s, want %s/%s", testCase.action, status, reason, testCase.wantStatus, testCase.wantReason)
		}
	}
	status, reason := ReconcileDiscreteControl("return_home", acceptedAt, deadline, deadline, ControlTelemetryEvidence{})
	if status != "unknown" || reason != "deadline_without_fresh_evidence" {
		t.Fatalf("deadline result = %s/%s", status, reason)
	}
}

func TestSafeControlErrorNeverReturnsUpstreamText(t *testing.T) {
	t.Parallel()
	secret := errors.New("upstream leaked TOKEN_REDACTED")
	if code := safeControlError(secret); code != "result_unknown" || strings.Contains(code, "TOKEN") {
		t.Fatalf("unsafe control error %q", code)
	}
}

func TestDefinitiveControlNackExcludesUnknownSchemaAndTransportResults(t *testing.T) {
	t.Parallel()
	if !definitiveControlNack(&APIError{SafeCode: "scope_forbidden", HTTPStatus: http.StatusOK}) {
		t.Fatal("explicit business rejection was not classified as NACK")
	}
	for _, apiError := range []*APIError{
		{SafeCode: "schema_incompatible", HTTPStatus: http.StatusOK},
		{SafeCode: "upstream_unavailable", HTTPStatus: http.StatusBadGateway, Retryable: true},
		{SafeCode: "upstream_unavailable"},
	} {
		if definitiveControlNack(apiError) {
			t.Fatalf("uncertain result classified as NACK: %#v", apiError)
		}
	}
}
