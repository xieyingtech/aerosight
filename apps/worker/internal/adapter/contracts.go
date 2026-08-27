package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const SchemaVersionV1 = "adapter.aerosight.dev/v1"

var upstreamEventTypes = map[string]bool{
	"device.discovered": true,
	"device.heartbeat":  true,
	"device.connection": true,
	"telemetry.pose":    true,
	"telemetry.battery": true,
	"live.status":       true,
	"media.reference":   true,
	"command.ack":       true,
	"device.topology":   true,
	"device.state":      true,
	"device.telemetry":  true,
	"device.event":      true,
	"device.request":    true,
	"command.reply":     true,
}

type UpstreamEnvelope struct {
	SchemaVersion    string          `json:"schemaVersion"`
	EventID          string          `json:"eventId"`
	AdapterID        int64           `json:"adapterId"`
	ProjectID        int             `json:"projectId"`
	ExternalDeviceID string          `json:"externalDeviceId"`
	EventType        string          `json:"eventType"`
	CapturedAt       time.Time       `json:"capturedAt"`
	ReceivedAt       time.Time       `json:"receivedAt"`
	Sequence         *int64          `json:"sequence,omitempty"`
	Payload          json.RawMessage `json:"payload"`
	SignatureContext json.RawMessage `json:"signatureContext,omitempty"`
}

type CommandEnvelope struct {
	SchemaVersion    string          `json:"schemaVersion"`
	CommandID        string          `json:"commandId"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	AdapterID        int64           `json:"adapterId"`
	ProjectID        int             `json:"projectId"`
	ExternalDeviceID string          `json:"externalDeviceId"`
	CapabilityCode   string          `json:"capabilityCode"`
	Parameters       json.RawMessage `json:"parameters"`
	Deadline         time.Time       `json:"deadline"`
	SafetyContext    json.RawMessage `json:"safetyContext"`
}

type Vector3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Quaternion struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	W float64 `json:"w"`
}

type Pose struct {
	DeviceType               string         `json:"deviceType"`
	CRS                      string         `json:"crs"`
	VerticalDatum            string         `json:"verticalDatum,omitempty"`
	Longitude                float64        `json:"longitude"`
	Latitude                 float64        `json:"latitude"`
	AltitudeMeters           *float64       `json:"altitudeMeters,omitempty"`
	Orientation              *Quaternion    `json:"orientation,omitempty"`
	VelocityMetersPerSecond  *Vector3       `json:"velocityMetersPerSecond,omitempty"`
	HorizontalAccuracyMeters *float64       `json:"horizontalAccuracyMeters,omitempty"`
	VerticalAccuracyMeters   *float64       `json:"verticalAccuracyMeters,omitempty"`
	AttitudeAccuracyDegrees  *float64       `json:"attitudeAccuracyDegrees,omitempty"`
	Quality                  map[string]any `json:"quality,omitempty"`
}

func (envelope UpstreamEnvelope) ValidateForScope(projectID int, adapterID int64) error {
	if envelope.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported adapter schema version %q", envelope.SchemaVersion)
	}
	if envelope.ProjectID != projectID || envelope.AdapterID != adapterID {
		return errors.New("adapter envelope scope does not match authenticated adapter")
	}
	if envelope.EventID == "" || envelope.ExternalDeviceID == "" || len(envelope.Payload) == 0 {
		return errors.New("adapter envelope is missing required fields")
	}
	if !upstreamEventTypes[envelope.EventType] {
		return fmt.Errorf("unsupported adapter event type %q", envelope.EventType)
	}
	if envelope.CapturedAt.IsZero() || envelope.ReceivedAt.IsZero() {
		return errors.New("adapter envelope requires capturedAt and receivedAt")
	}
	if envelope.Sequence != nil && *envelope.Sequence < 0 {
		return errors.New("adapter sequence must be non-negative")
	}
	return nil
}

func (command CommandEnvelope) ValidateForScope(projectID int, adapterID int64, now time.Time) error {
	if command.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported adapter schema version %q", command.SchemaVersion)
	}
	if command.ProjectID != projectID || command.AdapterID != adapterID {
		return errors.New("adapter command scope does not match target adapter")
	}
	if command.CommandID == "" || command.IdempotencyKey == "" || command.ExternalDeviceID == "" || command.CapabilityCode == "" {
		return errors.New("adapter command is missing required fields")
	}
	if len(command.Parameters) == 0 || len(command.SafetyContext) == 0 {
		return errors.New("adapter command requires parameters and safetyContext")
	}
	if command.Deadline.IsZero() || !command.Deadline.After(now) {
		return errors.New("adapter command deadline must be in the future")
	}
	return nil
}

func (pose Pose) Validate() error {
	if pose.DeviceType == "" || pose.CRS == "" {
		return errors.New("pose requires deviceType and crs")
	}
	if pose.Longitude < -180 || pose.Longitude > 180 || pose.Latitude < -90 || pose.Latitude > 90 {
		return errors.New("pose longitude or latitude is out of range")
	}
	return nil
}
