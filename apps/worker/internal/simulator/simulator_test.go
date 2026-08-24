package simulator

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"aerosight/worker/internal/adapter"
)

type collectingSink struct {
	events []adapter.UpstreamEnvelope
}

func (sink *collectingSink) Publish(_ context.Context, event adapter.UpstreamEnvelope) error {
	sink.events = append(sink.events, event)
	return nil
}

func TestAirGroundScenarioCoversOperationalFailures(t *testing.T) {
	file, err := os.Open("../../testdata/inspection-scenario.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scenario, err := DecodeScenario(file)
	if err != nil {
		t.Fatal(err)
	}
	sink := &collectingSink{}
	if err := New(NoWaitSleeper{}).Run(context.Background(), scenario, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != len(scenario.Devices)+len(scenario.Steps) {
		t.Fatalf("got %d events want %d", len(sink.events), len(scenario.Devices)+len(scenario.Steps))
	}

	poseByDevice := map[string]int{}
	seen := map[string]bool{}
	outcomes := map[string]bool{}
	for index, event := range sink.events {
		if event.EventID == "" || event.Sequence == nil {
			t.Fatalf("event %d lacks deterministic identity or sequence", index)
		}
		seen[event.EventType] = true
		if event.EventType == "telemetry.pose" {
			var pose adapter.Pose
			if err := json.Unmarshal(event.Payload, &pose); err != nil || pose.Validate() != nil {
				t.Fatalf("invalid canonical pose for %s: %v", event.ExternalDeviceID, err)
			}
			poseByDevice[event.ExternalDeviceID]++
		}
		if event.EventType == "command.ack" {
			var payload struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			outcomes[payload.Outcome] = true
		}
	}
	if poseByDevice["sim-drone-01"] < 2 || poseByDevice["sim-ground-01"] < 2 {
		t.Fatalf("both vehicle types must emit a trajectory: %#v", poseByDevice)
	}
	for _, eventType := range []string{"telemetry.battery", "live.status", "media.reference", "device.connection"} {
		if !seen[eventType] {
			t.Fatalf("scenario lacks %s", eventType)
		}
	}
	for _, outcome := range []string{"ack", "nack", "timeout"} {
		if !outcomes[outcome] {
			t.Fatalf("scenario lacks command outcome %s", outcome)
		}
	}
}

func TestScenarioRejectsUnknownDeviceAndEvent(t *testing.T) {
	base := Scenario{
		SchemaVersion: SchemaVersionV1, Name: "invalid", ProjectID: 1, AdapterID: 1,
		StartedAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		Devices:   []Device{{ExternalDeviceID: "drone", Name: "Drone", DeviceType: "drone"}},
	}
	base.Steps = []Step{{ExternalDeviceID: "missing", EventType: "telemetry.pose", Payload: json.RawMessage(`{}`)}}
	if err := base.Validate(); err == nil {
		t.Fatal("unknown device should fail scenario validation")
	}
	base.Steps = []Step{{ExternalDeviceID: "drone", EventType: "pretend.success", Payload: json.RawMessage(`{}`)}}
	if err := base.Validate(); err == nil {
		t.Fatal("unknown event should fail scenario validation")
	}
}
