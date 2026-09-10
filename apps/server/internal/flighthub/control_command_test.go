package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type governedControlClientFixture struct{ calls int }

func (client *governedControlClientFixture) ExecuteDeviceControl(_ context.Context, _, _, actionCode, _ string, _ json.RawMessage) (ControlActionDefinition, error) {
	client.calls++
	return ControlActionDefinition{Code: actionCode, ResultSemantics: ControlResultAcceptanceOnly}, nil
}

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

func TestCameraControlPreflightBlocksUnsupportedOfflineAndStaleDevicesBeforeUpstream(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	policy := discreteControlPolicies["camera.change_lens"]
	valid := loadedControlCommand{
		CommandKey: "camera.change_lens", CapabilityCode: "camera.lens.change",
		RecordedConnectorCapabilityCode: "device.lens.change", RecordedFeatureFlag: FlightHubLensChangeFeatureFlag,
		DeviceSN: "AIRCRAFT_REDACTED", DeviceTypeKey: "dji.matrice4td", ProjectUUID: "PROJECT_REDACTED",
		ConnectorStatus: "connected", FeatureEnabled: true, CapabilityVerified: true, DeviceOnline: true, StateFresh: true,
		ApprovalValid: true, SafetyPolicyCurrent: true, Deadline: now.Add(time.Minute),
		Parameters: json.RawMessage(`{"cameraIndex":"CAMERA_REDACTED","lensType":"wide"}`),
	}
	for _, testCase := range []struct {
		name string
		edit func(*loadedControlCommand)
	}{
		{"unsupported model", func(command *loadedControlCommand) { command.DeviceTypeKey = "dji.unknown" }},
		{"offline", func(command *loadedControlCommand) { command.DeviceOnline = false }},
		{"stale", func(command *loadedControlCommand) { command.StateFresh = false }},
		{"firmware evidence mismatch", func(command *loadedControlCommand) { command.CapabilityVerified = false }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := valid
			testCase.edit(&command)
			client := &governedControlClientFixture{}
			if _, err := invokeGovernedDeviceControl(context.Background(), client, "TOKEN_REDACTED", command, policy, now); err == nil {
				t.Fatal("unsafe camera command was accepted")
			}
			if client.calls != 0 {
				t.Fatalf("upstream calls=%d, want 0", client.calls)
			}
		})
	}
	client := &governedControlClientFixture{}
	if _, err := invokeGovernedDeviceControl(context.Background(), client, "TOKEN_REDACTED", valid, policy, now); err != nil || client.calls != 1 {
		t.Fatalf("valid camera command calls=%d err=%v", client.calls, err)
	}
}

func TestExecuteCameraAndLensControlUsesBoundSerialAndNeverRetries(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct{ action, path, parameters, body string }{
		{"camera.change", "/openapi/v2.0/device/change-camera", `{"cameraIndex":"CAMERA_REDACTED","cameraPosition":"outdoor"}`, `{"sn":"DOCK_REDACTED","camera_index":"CAMERA_REDACTED","camera_position":"outdoor"}`},
		{"camera.change_lens", "/openapi/v2.0/device/change-lens", `{"cameraIndex":"CAMERA_REDACTED","lensType":"wide"}`, `{"sn":"DOCK_REDACTED","camera_index":"CAMERA_REDACTED","lens_type":"wide"}`},
	} {
		t.Run(testCase.action, func(t *testing.T) {
			calls := 0
			client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(request.Body)
				if request.URL.Path != testCase.path || string(body) != testCase.body {
					t.Fatalf("unexpected request %s %s", request.URL.Path, body)
				}
				return response(http.StatusOK, []byte(`{"code":0,"message":"","data":null}`), nil), nil
			}), func(config *Config) { config.MaxRetries = 3 })
			definition, err := client.ExecuteDeviceControl(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", testCase.action, "DOCK_REDACTED", json.RawMessage(testCase.parameters))
			if err != nil || calls != 1 || definition.FinalOnHTTPSuccess || definition.ResultSemantics != ControlResultAcceptanceOnly {
				t.Fatalf("definition=%#v calls=%d err=%v", definition, calls, err)
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
