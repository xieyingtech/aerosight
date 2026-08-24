package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"aerosight/worker/internal/adapter"
)

const SchemaVersionV1 = "simulator.aerosight.dev/v1"

type Device struct {
	ExternalDeviceID string   `json:"externalDeviceId"`
	Name             string   `json:"name"`
	DeviceType       string   `json:"deviceType"`
	Capabilities     []string `json:"capabilities"`
}

type Step struct {
	OffsetMilliseconds        int64           `json:"offsetMilliseconds"`
	ReceivedDelayMilliseconds int64           `json:"receivedDelayMilliseconds,omitempty"`
	ExternalDeviceID          string          `json:"externalDeviceId"`
	EventType                 string          `json:"eventType"`
	Payload                   json.RawMessage `json:"payload"`
}

type Scenario struct {
	SchemaVersion string    `json:"schemaVersion"`
	Name          string    `json:"name"`
	ProjectID     int       `json:"projectId"`
	AdapterID     int64     `json:"adapterId"`
	StartedAt     time.Time `json:"startedAt"`
	Devices       []Device  `json:"devices"`
	Steps         []Step    `json:"steps"`
}

func DecodeScenario(reader io.Reader) (Scenario, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var scenario Scenario
	if err := decoder.Decode(&scenario); err != nil {
		return Scenario{}, err
	}
	return scenario, scenario.Validate()
}

func (scenario Scenario) Validate() error {
	if scenario.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported simulator schema version %q", scenario.SchemaVersion)
	}
	if scenario.Name == "" || scenario.ProjectID <= 0 || scenario.AdapterID <= 0 || scenario.StartedAt.IsZero() {
		return errors.New("simulator scenario is missing identity, scope, or start time")
	}
	devices := map[string]bool{}
	for _, device := range scenario.Devices {
		if device.ExternalDeviceID == "" || device.Name == "" {
			return errors.New("simulator device is missing identity or name")
		}
		if device.DeviceType != "drone" && device.DeviceType != "ground_robot" {
			return fmt.Errorf("unsupported simulator device type %q", device.DeviceType)
		}
		if devices[device.ExternalDeviceID] {
			return fmt.Errorf("duplicate simulator device %q", device.ExternalDeviceID)
		}
		devices[device.ExternalDeviceID] = true
	}
	if len(devices) == 0 {
		return errors.New("simulator scenario requires at least one device")
	}
	for _, step := range scenario.Steps {
		if step.OffsetMilliseconds < 0 || step.ReceivedDelayMilliseconds < 0 {
			return errors.New("simulator step delays must be non-negative")
		}
		if !devices[step.ExternalDeviceID] {
			return fmt.Errorf("simulator step references unknown device %q", step.ExternalDeviceID)
		}
		if len(step.Payload) == 0 || !json.Valid(step.Payload) {
			return errors.New("simulator step payload must be valid JSON")
		}
		testEnvelope := adapter.UpstreamEnvelope{
			SchemaVersion: adapter.SchemaVersionV1, EventID: "validation", AdapterID: scenario.AdapterID,
			ProjectID: scenario.ProjectID, ExternalDeviceID: step.ExternalDeviceID, EventType: step.EventType,
			CapturedAt: scenario.StartedAt, ReceivedAt: scenario.StartedAt, Payload: step.Payload,
		}
		if err := testEnvelope.ValidateForScope(scenario.ProjectID, scenario.AdapterID); err != nil {
			return err
		}
	}
	return nil
}

type Sink interface {
	Publish(context.Context, adapter.UpstreamEnvelope) error
}

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

type RealTimeSleeper struct{}

func (RealTimeSleeper) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type NoWaitSleeper struct{}

func (NoWaitSleeper) Sleep(context.Context, time.Duration) error { return nil }

type Simulator struct {
	sleeper Sleeper
}

func New(sleeper Sleeper) *Simulator {
	if sleeper == nil {
		sleeper = NoWaitSleeper{}
	}
	return &Simulator{sleeper: sleeper}
}

func (simulator *Simulator) Run(ctx context.Context, scenario Scenario, sink Sink) error {
	if err := scenario.Validate(); err != nil {
		return err
	}
	sequence := map[string]int64{}
	eventNumber := int64(0)
	publish := func(deviceID, eventType string, capturedAt, receivedAt time.Time, payload json.RawMessage) error {
		eventNumber++
		sequence[deviceID]++
		value := sequence[deviceID]
		envelope := adapter.UpstreamEnvelope{
			SchemaVersion: adapter.SchemaVersionV1,
			EventID:       fmt.Sprintf("sim-%s-%04d", scenario.Name, eventNumber), AdapterID: scenario.AdapterID,
			ProjectID: scenario.ProjectID, ExternalDeviceID: deviceID, EventType: eventType,
			CapturedAt: capturedAt, ReceivedAt: receivedAt, Sequence: &value, Payload: payload,
		}
		if err := envelope.ValidateForScope(scenario.ProjectID, scenario.AdapterID); err != nil {
			return err
		}
		return sink.Publish(ctx, envelope)
	}

	for _, device := range scenario.Devices {
		payload, _ := json.Marshal(map[string]any{
			"name": device.Name, "deviceType": device.DeviceType, "capabilities": device.Capabilities,
		})
		if err := publish(device.ExternalDeviceID, "device.discovered", scenario.StartedAt, scenario.StartedAt, payload); err != nil {
			return err
		}
	}
	steps := append([]Step(nil), scenario.Steps...)
	sort.SliceStable(steps, func(left, right int) bool {
		return steps[left].OffsetMilliseconds < steps[right].OffsetMilliseconds
	})
	previousOffset := int64(0)
	for _, step := range steps {
		if err := simulator.sleeper.Sleep(ctx, time.Duration(step.OffsetMilliseconds-previousOffset)*time.Millisecond); err != nil {
			return err
		}
		capturedAt := scenario.StartedAt.Add(time.Duration(step.OffsetMilliseconds) * time.Millisecond)
		receivedAt := capturedAt.Add(time.Duration(step.ReceivedDelayMilliseconds) * time.Millisecond)
		if err := publish(step.ExternalDeviceID, step.EventType, capturedAt, receivedAt, step.Payload); err != nil {
			return err
		}
		previousOffset = step.OffsetMilliseconds
	}
	return nil
}

type JSONSink struct {
	Encoder *json.Encoder
}

func (sink JSONSink) Publish(_ context.Context, envelope adapter.UpstreamEnvelope) error {
	return sink.Encoder.Encode(envelope)
}
