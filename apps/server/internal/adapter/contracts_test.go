package adapter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validEnvelope(t *testing.T, deviceType string) UpstreamEnvelope {
	t.Helper()
	payload, err := json.Marshal(Pose{
		DeviceType: deviceType,
		CRS:        "EPSG:4326",
		Longitude:  120.1,
		Latitude:   30.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	sequence := int64(1)
	return UpstreamEnvelope{
		SchemaVersion:    SchemaVersionV1,
		EventID:          "event-1",
		AdapterID:        8,
		ProjectID:        9,
		ExternalDeviceID: "device-1",
		EventType:        "telemetry.pose",
		CapturedAt:       time.Now().UTC(),
		ReceivedAt:       time.Now().UTC(),
		Sequence:         &sequence,
		Payload:          payload,
	}
}

func TestDroneAndGroundRobotUseSamePoseContract(t *testing.T) {
	for _, deviceType := range []string{"drone", "ground_robot"} {
		envelope := validEnvelope(t, deviceType)
		if err := envelope.ValidateForScope(9, 8); err != nil {
			t.Fatalf("%s envelope failed: %v", deviceType, err)
		}
		var pose Pose
		if err := json.Unmarshal(envelope.Payload, &pose); err != nil {
			t.Fatal(err)
		}
		if err := pose.Validate(); err != nil {
			t.Fatalf("%s pose failed: %v", deviceType, err)
		}
	}
}

func TestEnvelopeRejectsUnknownVersionMissingFieldsAndCrossProjectScope(t *testing.T) {
	envelope := validEnvelope(t, "drone")
	envelope.SchemaVersion = "adapter.aerosight.dev/v2"
	if err := envelope.ValidateForScope(9, 8); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported version, got %v", err)
	}
	envelope = validEnvelope(t, "drone")
	envelope.EventID = ""
	if err := envelope.ValidateForScope(9, 8); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing field, got %v", err)
	}
	envelope = validEnvelope(t, "drone")
	if err := envelope.ValidateForScope(99, 8); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("expected scope mismatch, got %v", err)
	}
}

func TestCommandRequiresFutureDeadlineAndMatchingScope(t *testing.T) {
	now := time.Now().UTC()
	command := CommandEnvelope{
		SchemaVersion:    SchemaVersionV1,
		CommandID:        "command-1",
		IdempotencyKey:   "idempotency-1",
		AdapterID:        8,
		ProjectID:        9,
		ExternalDeviceID: "device-1",
		CapabilityCode:   "flight.route",
		Parameters:       json.RawMessage(`{}`),
		Deadline:         now.Add(time.Minute),
		SafetyContext:    json.RawMessage(`{"policyVersion":1}`),
	}
	if err := command.ValidateForScope(9, 8, now); err != nil {
		t.Fatal(err)
	}
	if err := command.ValidateForScope(10, 8, now); err == nil {
		t.Fatal("cross-project command should fail")
	}
}
