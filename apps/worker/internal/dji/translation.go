package dji

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/adapter"
)

var (
	ErrCommandUnauthorized   = errors.New("DJI_COMMAND_UNAUTHORIZED")
	ErrCapabilityUnsupported = errors.New("DJI_CAPABILITY_UNSUPPORTED")
	ErrInvalidVendorPayload  = errors.New("DJI_VENDOR_PAYLOAD_INVALID")
)

type Degradation struct {
	State       string `json:"state"`
	ReasonCode  string `json:"reasonCode"`
	SafeToRetry bool   `json:"safeToRetry"`
}

type VendorCommand struct {
	TransactionID string         `json:"tid"`
	DeviceSN      string         `json:"sn"`
	Method        string         `json:"method"`
	Data          map[string]any `json:"data"`
}

type telemetryCallback struct {
	EventID     string `json:"event_id"`
	DeviceSN    string `json:"sn"`
	TimestampMS int64  `json:"timestamp_ms"`
	Sequence    *int64 `json:"sequence,omitempty"`
	Data        struct {
		Longitude       float64 `json:"longitude"`
		Latitude        float64 `json:"latitude"`
		Height          float64 `json:"height"`
		HorizontalSpeed float64 `json:"horizontal_speed"`
		VerticalSpeed   float64 `json:"vertical_speed"`
		BatteryPercent  float64 `json:"battery_percent"`
		LinkQuality     int     `json:"link_quality"`
		ModeCode        int     `json:"mode_code"`
	} `json:"data"`
}

// MapTelemetry converts vendor fields before they can reach ingestion or UI code.
func MapTelemetry(projectID int, adapterID int64, receivedAt time.Time, payload []byte) ([]adapter.UpstreamEnvelope, error) {
	if projectID <= 0 || adapterID <= 0 || receivedAt.IsZero() {
		return nil, ErrInvalidVendorPayload
	}
	var callback telemetryCallback
	if err := json.Unmarshal(payload, &callback); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVendorPayload, err)
	}
	if callback.EventID == "" || callback.DeviceSN == "" || callback.TimestampMS <= 0 || callback.Data.BatteryPercent < 0 || callback.Data.BatteryPercent > 100 {
		return nil, ErrInvalidVendorPayload
	}
	capturedAt := time.UnixMilli(callback.TimestampMS).UTC()
	altitude := callback.Data.Height
	velocity := adapter.Vector3{X: callback.Data.HorizontalSpeed, Z: callback.Data.VerticalSpeed}
	pose, err := json.Marshal(adapter.Pose{
		DeviceType: "drone", CRS: "EPSG:4326", VerticalDatum: "vendor-relative",
		Longitude: callback.Data.Longitude, Latitude: callback.Data.Latitude, AltitudeMeters: &altitude,
		VelocityMetersPerSecond: &velocity,
		Quality:                 map[string]any{"linkQuality": callback.Data.LinkQuality, "modeCode": callback.Data.ModeCode},
	})
	if err != nil {
		return nil, err
	}
	battery, err := json.Marshal(map[string]any{
		"percent": callback.Data.BatteryPercent, "linkQuality": callback.Data.LinkQuality, "modeCode": callback.Data.ModeCode,
	})
	if err != nil {
		return nil, err
	}
	base := adapter.UpstreamEnvelope{
		SchemaVersion: adapter.SchemaVersionV1, AdapterID: adapterID, ProjectID: projectID,
		ExternalDeviceID: callback.DeviceSN, CapturedAt: capturedAt, ReceivedAt: receivedAt, Sequence: callback.Sequence,
	}
	poseEnvelope := base
	poseEnvelope.EventID, poseEnvelope.EventType, poseEnvelope.Payload = callback.EventID+":pose", "telemetry.pose", pose
	batteryEnvelope := base
	batteryEnvelope.EventID, batteryEnvelope.EventType, batteryEnvelope.Payload = callback.EventID+":battery", "telemetry.battery", battery
	return []adapter.UpstreamEnvelope{poseEnvelope, batteryEnvelope}, nil
}

// MapCommand fails closed when authorization or capability declarations are missing.
func MapCommand(command adapter.CommandEnvelope, authorized bool, declaredCapabilities map[string]bool, now time.Time) (VendorCommand, *Degradation, error) {
	if command.ProjectID <= 0 || command.AdapterID <= 0 {
		return VendorCommand{}, degraded("DJI_COMMAND_INVALID"), errors.New("DJI command scope is required")
	}
	if err := command.ValidateForScope(command.ProjectID, command.AdapterID, now); err != nil {
		return VendorCommand{}, degraded("DJI_COMMAND_INVALID"), err
	}
	if !authorized {
		return VendorCommand{}, degraded("DJI_COMMAND_UNAUTHORIZED"), ErrCommandUnauthorized
	}
	if !declaredCapabilities[command.CapabilityCode] {
		return VendorCommand{}, degraded("DJI_CAPABILITY_UNSUPPORTED"), ErrCapabilityUnsupported
	}
	var parameters map[string]any
	if err := json.Unmarshal(command.Parameters, &parameters); err != nil {
		return VendorCommand{}, degraded("DJI_COMMAND_PARAMETERS_INVALID"), err
	}
	method, err := vendorMethod(command.CapabilityCode, parameters)
	if err != nil {
		return VendorCommand{}, degraded("DJI_CAPABILITY_UNSUPPORTED"), err
	}
	return VendorCommand{TransactionID: command.CommandID, DeviceSN: command.ExternalDeviceID, Method: method, Data: parameters}, nil, nil
}

func vendorMethod(capability string, parameters map[string]any) (string, error) {
	switch capability {
	case "flight.navigate", "flight.route":
		return "flighttask_execute", nil
	case "flight.takeoff":
		return "takeoff_to_point", nil
	case "flight.land":
		return "land", nil
	case "flight.return_home", "command.rth":
		return "return_home", nil
	case "camera.capture", "camera.photo":
		return "camera_photo_take", nil
	case "camera.video":
		if parameters["action"] == "stop" {
			return "camera_video_stop", nil
		}
		return "camera_video_start", nil
	case "gimbal.control":
		return "gimbal_control", nil
	case "camera.live", "live.video":
		switch parameters["action"] {
		case "start":
			return "live_start_push", nil
		case "stop":
			return "live_stop_push", nil
		default:
			return "", fmt.Errorf("%w: live action must be start or stop", ErrCapabilityUnsupported)
		}
	default:
		return "", ErrCapabilityUnsupported
	}
}

type commandResult struct {
	TransactionID string `json:"tid"`
	DeviceSN      string `json:"sn"`
	TimestampMS   int64  `json:"timestamp_ms"`
	Result        int    `json:"result"`
	ErrorCode     int    `json:"error_code"`
}

func MapCommandResult(projectID int, adapterID int64, receivedAt time.Time, payload []byte) (adapter.UpstreamEnvelope, error) {
	var result commandResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return adapter.UpstreamEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidVendorPayload, err)
	}
	if projectID <= 0 || adapterID <= 0 || receivedAt.IsZero() || result.TransactionID == "" || result.DeviceSN == "" || result.TimestampMS <= 0 {
		return adapter.UpstreamEnvelope{}, ErrInvalidVendorPayload
	}
	outcome := "ack"
	if result.Result != 0 {
		outcome = "nack"
	}
	canonical, _ := json.Marshal(map[string]any{
		"commandId": result.TransactionID, "outcome": outcome,
		"code": fmt.Sprintf("DJI_%d", result.ErrorCode), "retryable": false,
	})
	return adapter.UpstreamEnvelope{
		SchemaVersion: adapter.SchemaVersionV1, EventID: result.TransactionID + ":result",
		AdapterID: adapterID, ProjectID: projectID, ExternalDeviceID: result.DeviceSN,
		EventType: "command.ack", CapturedAt: time.UnixMilli(result.TimestampMS).UTC(), ReceivedAt: receivedAt, Payload: canonical,
	}, nil
}

func degraded(reason string) *Degradation {
	return &Degradation{State: "degraded", ReasonCode: reason, SafeToRetry: false}
}
