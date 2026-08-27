package dji

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/adapter"
)

func TestCallbackSignatureRejectsForgeryReplayAndExpiredTimestamp(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	body := []byte(`{"event":"device_online","sn":"SANDBOX-UAV-001"}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	secret := []byte("fixture-only-secret")
	signature := SignCallback(secret, timestamp, "nonce-1", body)
	store := NewMemoryNonceStore()
	if err := VerifyCallback(secret, timestamp, "nonce-1", signature, body, now, store); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(VerifyCallback(secret, timestamp, "nonce-1", signature, body, now, store), ErrReplayedCallback) {
		t.Fatal("replayed callback accepted")
	}
	if !errors.Is(VerifyCallback(secret, timestamp, "nonce-2", signature, body, now, NewMemoryNonceStore()), ErrInvalidSignature) {
		t.Fatal("forged nonce accepted")
	}
	expired := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)
	if !errors.Is(VerifyCallback(secret, expired, "nonce-3", SignCallback(secret, expired, "nonce-3", body), body, now, NewMemoryNonceStore()), ErrExpiredCallback) {
		t.Fatal("expired callback accepted")
	}
}

func TestRedactedTopologyMapsDockAircraftAndCapabilities(t *testing.T) {
	payload, err := os.ReadFile("../../../../test/fixtures/dji-topology-redacted.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "tenant") || strings.Contains(string(payload), "secret") {
		t.Fatal("fixture contains tenant credentials")
	}
	discoveries, err := MapTopology(17, 8, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(discoveries) != 2 || discoveries[0].ExternalDeviceType != "dock" || discoveries[1].ExternalDeviceType != "drone" {
		t.Fatalf("unexpected topology: %+v", discoveries)
	}
	capabilities := discoveries[1].Identity["capabilities"].([]string)
	expected := map[string]bool{
		"flight.navigate": true, "flight.route": true, "flight.return_home": true, "command.rth": true,
		"camera.capture": true, "camera.live": true, "camera.photo": true, "live.video": true,
	}
	for _, capability := range capabilities {
		delete(expected, capability)
	}
	if len(expected) != 0 {
		t.Fatalf("missing capabilities: %v", expected)
	}
	if discoveries[1].Identity["parentExternalDeviceId"] != "SANDBOX-DOCK-001" {
		t.Fatal("aircraft topology parent was lost")
	}
}

type vendorFixture struct {
	Telemetry      json.RawMessage   `json:"telemetry"`
	CommandResults []json.RawMessage `json:"command_results"`
}

func loadVendorFixture(t *testing.T) vendorFixture {
	t.Helper()
	payload, err := os.ReadFile("../../../../test/fixtures/dji-vendor-responses-redacted.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "tenant") || strings.Contains(string(payload), "secret") {
		t.Fatal("fixture contains tenant credentials")
	}
	var fixture vendorFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestVendorTelemetryMapsToCanonicalEnvelopes(t *testing.T) {
	fixture := loadVendorFixture(t)
	receivedAt := time.Date(2026, 8, 27, 8, 0, 1, 0, time.UTC)
	events, err := MapTelemetry(17, 8, receivedAt, fixture.Telemetry)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventType != "telemetry.pose" || events[1].EventType != "telemetry.battery" {
		t.Fatalf("unexpected canonical telemetry: %+v", events)
	}
	for _, event := range events {
		if err := event.ValidateForScope(17, 8); err != nil {
			t.Fatalf("invalid canonical envelope: %v", err)
		}
	}
	var pose adapter.Pose
	if err := json.Unmarshal(events[0].Payload, &pose); err != nil || pose.Validate() != nil {
		t.Fatalf("invalid mapped pose: %+v, %v", pose, err)
	}
}

func TestCommandMappingFailsClosedAndMapsLiveAndSafeFlight(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	command := adapter.CommandEnvelope{
		SchemaVersion: adapter.SchemaVersionV1, CommandID: "SANDBOX-CMD-101", IdempotencyKey: "sandbox:101",
		AdapterID: 8, ProjectID: 17, ExternalDeviceID: "SANDBOX-UAV-001", CapabilityCode: "flight.route",
		Parameters: json.RawMessage(`{"routeId":"SANDBOX-ROUTE-1"}`), Deadline: now.Add(time.Minute), SafetyContext: json.RawMessage(`{"preflight":"passed"}`),
	}
	if _, degradation, err := MapCommand(command, false, map[string]bool{"flight.route": true}, now); !errors.Is(err, ErrCommandUnauthorized) || degradation.ReasonCode != "DJI_COMMAND_UNAUTHORIZED" {
		t.Fatalf("unauthorized command did not degrade explicitly: degradation=%+v err=%v", degradation, err)
	}
	if _, degradation, err := MapCommand(command, true, map[string]bool{}, now); !errors.Is(err, ErrCapabilityUnsupported) || degradation.ReasonCode != "DJI_CAPABILITY_UNSUPPORTED" {
		t.Fatalf("unsupported command did not degrade explicitly: degradation=%+v err=%v", degradation, err)
	}
	vendor, degradation, err := MapCommand(command, true, map[string]bool{"flight.route": true}, now)
	if err != nil || degradation != nil || vendor.Method != "flighttask_execute" || vendor.TransactionID != command.CommandID {
		t.Fatalf("safe flight command was not mapped: vendor=%+v degradation=%+v err=%v", vendor, degradation, err)
	}
	command.CapabilityCode = "camera.live"
	command.Parameters = json.RawMessage(`{"action":"start","url":"rtmp://media-gateway.invalid/live/sandbox"}`)
	vendor, degradation, err = MapCommand(command, true, map[string]bool{"camera.live": true}, now)
	if err != nil || degradation != nil || vendor.Method != "live_start_push" {
		t.Fatalf("live command was not mapped: vendor=%+v degradation=%+v err=%v", vendor, degradation, err)
	}
}

func TestVendorErrorFixtureMapsNACKWithoutRawMessage(t *testing.T) {
	fixture := loadVendorFixture(t)
	receivedAt := time.Date(2026, 8, 27, 8, 0, 3, 0, time.UTC)
	outcomes := map[string]bool{}
	for _, raw := range fixture.CommandResults {
		event, err := MapCommandResult(17, 8, receivedAt, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := event.ValidateForScope(17, 8); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Outcome string `json:"outcome"`
			Code    string `json:"code"`
		}
		if err := json.Unmarshal(event.Payload, &result); err != nil {
			t.Fatal(err)
		}
		outcomes[result.Outcome] = true
		if result.Outcome == "nack" && result.Code != "DJI_314001" {
			t.Fatalf("vendor NACK code was lost: %+v", result)
		}
	}
	if !outcomes["ack"] || !outcomes["nack"] {
		t.Fatalf("fixture did not exercise ACK and NACK: %v", outcomes)
	}
}
