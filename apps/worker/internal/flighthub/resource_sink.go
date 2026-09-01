package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/heartbeat"
	"aerosight/worker/internal/telemetry"
)

type TelemetryBatchIngestor interface {
	IngestBatch(context.Context, []telemetry.Telemetry) (int, error)
}

type RemoteResourceWriter interface {
	ApplyRemoteResources(context.Context, connector.Instance, connector.RemoteResourceBatch) (connector.RemoteResourceApplyResult, error)
}

type DeviceFreshnessProjector interface {
	Record(context.Context, heartbeat.Signal) error
}

type DeviceHealthProjector interface {
	Apply(context.Context, connector.Instance, HealthPoll) error
}

type SQLResourceStreamSink struct {
	telemetry TelemetryBatchIngestor
	resources RemoteResourceWriter
	freshness DeviceFreshnessProjector
	health    DeviceHealthProjector
}

func NewSQLResourceStreamSink(telemetryIngestor TelemetryBatchIngestor, resources RemoteResourceWriter, freshness DeviceFreshnessProjector, health DeviceHealthProjector) (*SQLResourceStreamSink, error) {
	if telemetryIngestor == nil || resources == nil || freshness == nil || health == nil {
		return nil, errors.New("FlightHub resource stream sink dependencies are required")
	}
	return &SQLResourceStreamSink{telemetry: telemetryIngestor, resources: resources, freshness: freshness, health: health}, nil
}

func (sink *SQLResourceStreamSink) ApplyDeviceState(ctx context.Context, instance connector.Instance, poll DeviceStatePoll) error {
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.Device.DeviceID <= 0 || poll.Device.TeamID <= 0 || poll.ReceivedAt.IsZero() || poll.Snapshot.SN != poll.Device.Serial {
		return errors.New("FlightHub device state projection scope is invalid")
	}
	capturedAt, capturedSource := stateCapturedAt(poll.Snapshot.State, poll.ReceivedAt)
	mappedPayload, err := json.Marshal(poll.Mapped)
	if err != nil {
		return err
	}
	stateDigest := sha256.Sum256(mappedPayload)
	eventPrefix := fmt.Sprintf("flighthub-state:%d:%d:%d:%s", instance.ID, poll.Device.DeviceID, capturedAt.UnixMilli(), hex.EncodeToString(stateDigest[:8]))
	quality, err := json.Marshal(map[string]any{
		"source": "dji-flighthub-openapi", "mapperVersion": poll.Mapped.MapperVersion,
		"capturedAtSource": capturedSource, "coordinateReference": coordinateReference(poll.Mapped.Position),
		"unknownFields": poll.Mapped.Diagnostics,
	})
	if err != nil {
		return err
	}
	stateSequence := int64(1)
	batch := make([]telemetry.Telemetry, 0, 2)
	if poll.Mapped.Position != nil && poll.Mapped.Position.Validity == "valid" {
		posePayload, poseErr := mappedPosePayload(poll.Mapped)
		if poseErr != nil {
			return poseErr
		}
		poseSequence := int64(0)
		batch = append(batch, telemetry.Telemetry{
			ProjectID: instance.ProjectID, TeamID: poll.Device.TeamID, AdapterID: instance.ID, DeviceID: poll.Device.DeviceID,
			EventID: eventPrefix + ":pose", Type: "telemetry.pose", Sequence: &poseSequence,
			CapturedAt: capturedAt, ReceivedAt: poll.ReceivedAt, Payload: posePayload, Quality: quality,
		})
	}
	batch = append(batch, telemetry.Telemetry{
		ProjectID: instance.ProjectID, TeamID: poll.Device.TeamID, AdapterID: instance.ID, DeviceID: poll.Device.DeviceID,
		EventID: eventPrefix + ":state", Type: "dji.flighthub.state", Sequence: &stateSequence,
		CapturedAt: capturedAt, ReceivedAt: poll.ReceivedAt, Payload: mappedPayload, Quality: quality,
	})
	if _, err = sink.telemetry.IngestBatch(ctx, batch); err != nil {
		return err
	}
	heartbeatInterval := poll.FreshnessInterval
	if heartbeatInterval < 5*time.Second {
		heartbeatInterval = 30 * time.Second
	}
	err = sink.freshness.Record(ctx, heartbeat.Signal{
		ProjectID: instance.ProjectID, TeamID: poll.Device.TeamID, AdapterID: instance.ID, DeviceID: poll.Device.DeviceID,
		SessionKey: fmt.Sprintf("flighthub-state:%d:%d", instance.ID, poll.Device.DeviceID),
		ObservedAt: capturedAt, ReceivedAt: poll.ReceivedAt, HeartbeatIntervalSecond: int(heartbeatInterval / time.Second),
		RawStatusReference: poll.Mapped.MapperVersion,
	})
	if errors.Is(err, heartbeat.ErrStaleSignal) {
		return nil
	}
	return err
}

func coordinateReference(position *PositionState) string {
	if position == nil {
		return "missing"
	}
	return position.CoordinateReference
}

func stateCapturedAt(state map[string]json.RawMessage, fallback time.Time) (time.Time, string) {
	for _, field := range []string{"device_data_update_time", "timestamp", "update_time"} {
		value, ok := numberValue(state[field])
		if !ok || value == nil || *value <= 0 {
			continue
		}
		integer := int64(*value)
		if integer < 10_000_000_000 {
			return time.Unix(integer, 0).UTC(), field + ":seconds"
		}
		return time.UnixMilli(integer).UTC(), field + ":milliseconds"
	}
	return fallback.UTC(), "received_at_fallback"
}

func mappedPosePayload(mapped MappedDeviceState) (json.RawMessage, error) {
	position := mapped.Position
	if position == nil || position.Validity != "valid" {
		return nil, errors.New("FlightHub mapped pose is invalid")
	}
	crs := "dji-flighthub:unverified"
	if position.CoordinateReference == "EPSG:4326" {
		crs = "EPSG:4326"
	}
	pose := adapter.Pose{
		DeviceType: mapped.DeviceKind, CRS: crs, TransformVersion: mapped.MapperVersion,
		Longitude: position.Longitude, Latitude: position.Latitude, AltitudeMeters: position.HeightMeters,
		Quality: map[string]any{"source": "dji-flighthub-openapi", "coordinateReference": position.CoordinateReference},
	}
	if mapped.Attitude != nil {
		pose.Orientation = eulerQuaternion(mapped.Attitude.RollDegrees, mapped.Attitude.PitchDegrees, mapped.Attitude.HeadingDegrees)
	}
	if mapped.HorizontalSpeedMPS != nil || mapped.VerticalSpeedMPS != nil {
		pose.VelocityMetersPerSecond = &adapter.Vector3{}
		if mapped.HorizontalSpeedMPS != nil {
			pose.VelocityMetersPerSecond.X = *mapped.HorizontalSpeedMPS
		}
		if mapped.VerticalSpeedMPS != nil {
			pose.VelocityMetersPerSecond.Z = *mapped.VerticalSpeedMPS
		}
	}
	return json.Marshal(pose)
}

func eulerQuaternion(roll, pitch, heading *float64) *adapter.Quaternion {
	if roll == nil && pitch == nil && heading == nil {
		return nil
	}
	r, p, y := 0.0, 0.0, 0.0
	if roll != nil {
		r = *roll * math.Pi / 180
	}
	if pitch != nil {
		p = *pitch * math.Pi / 180
	}
	if heading != nil {
		y = *heading * math.Pi / 180
	}
	cy, sy := math.Cos(y/2), math.Sin(y/2)
	cp, sp := math.Cos(p/2), math.Sin(p/2)
	cr, sr := math.Cos(r/2), math.Sin(r/2)
	return &adapter.Quaternion{
		W: cr*cp*cy + sr*sp*sy,
		X: sr*cp*cy - cr*sp*sy,
		Y: cr*sp*cy + sr*cp*sy,
		Z: cr*cp*sy - sr*sp*cy,
	}
}

func (sink *SQLResourceStreamSink) ApplyHealth(ctx context.Context, instance connector.Instance, poll HealthPoll) error {
	bySerial := make(map[string]connector.ManagedConnectorDevice, len(poll.Devices))
	for _, device := range poll.Devices {
		bySerial[device.Serial] = device
	}
	resources := make([]connector.RemoteResource, 0)
	for _, deviceHMS := range poll.HMS {
		device, ok := bySerial[deviceHMS.SN]
		if !ok {
			return errors.New("FlightHub HMS device is outside managed scope")
		}
		for _, alert := range deviceHMS.Alerts.List {
			resources = appendHMSResources(resources, device, alert)
		}
	}
	if _, err := sink.resources.ApplyRemoteResources(ctx, instance, connector.RemoteResourceBatch{Kind: "hms", Resources: resources, CompleteSnapshot: true}); err != nil {
		return err
	}
	topologyResources, err := topologyRemoteResources(poll)
	if err != nil {
		return err
	}
	if _, err := sink.resources.ApplyRemoteResources(ctx, instance, connector.RemoteResourceBatch{Kind: "topology", Resources: topologyResources, CompleteSnapshot: true}); err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(poll.AutoRecord.Items))
	for _, item := range poll.AutoRecord.Items {
		device, ok := bySerial[item.SN]
		if !ok {
			return errors.New("FlightHub recording device is outside managed scope")
		}
		items = append(items, map[string]any{"deviceId": device.DeviceID, "cameraIndex": item.CameraIndex, "strategy": item.RecordingStrategy})
	}
	if _, err := sink.resources.ApplyRemoteResources(ctx, instance, connector.RemoteResourceBatch{Kind: "auto-record", Resources: []connector.RemoteResource{{
		RemoteID: "project", RemoteVersion: poll.AutoRecord.UpdatedAt,
		Summary: map[string]any{
			"autoCleaningDays": poll.AutoRecord.AutoCleaningDays, "autoCleaningDisabled": poll.AutoRecord.AutoCleaningDisabled,
			"dockRecordingDisabled": poll.AutoRecord.DockRecordingDisabled, "droneRecordingDisabled": poll.AutoRecord.DroneRecordingDisabled,
			"baseStationRecordingDisabled": poll.AutoRecord.BaseStationRecordingDisabled, "rtmpRecordingDisabled": poll.AutoRecord.RTMPRecordingDisabled,
			"items": items,
		},
	}}, CompleteSnapshot: true}); err != nil {
		return err
	}
	return sink.health.Apply(ctx, instance, poll)
}

func topologyRemoteResources(poll HealthPoll) ([]connector.RemoteResource, error) {
	bySerial := make(map[string]connector.ManagedConnectorDevice, len(poll.Devices))
	for _, device := range poll.Devices {
		bySerial[device.Serial] = device
	}
	resources := make([]connector.RemoteResource, 0, len(poll.Topologies))
	for _, topology := range poll.Topologies {
		summary := map[string]any{"source": "dji-flighthub-openapi", "parentDeviceIds": []int{}}
		if topology.Host != nil {
			host, ok := bySerial[topology.Host.SN]
			if !ok {
				return nil, errors.New("FlightHub topology host is outside managed scope")
			}
			summary["hostDeviceId"] = host.DeviceID
		}
		parents := make([]int, 0, len(topology.Parents))
		for _, parent := range topology.Parents {
			device, ok := bySerial[parent.SN]
			if !ok {
				return nil, errors.New("FlightHub topology parent is outside managed scope")
			}
			parents = append(parents, device.DeviceID)
		}
		summary["parentDeviceIds"] = parents
		resources = append(resources, connector.RemoteResource{RemoteID: secureRemoteKey(topology.Index), Summary: summary})
	}
	return resources, nil
}

func appendHMSResources(resources []connector.RemoteResource, device connector.ManagedConnectorDevice, alert HMSAlert) []connector.RemoteResource {
	remoteID := strconv.Itoa(device.DeviceID) + "/" + secureRemoteKey(alert.ID)
	resources = append(resources, connector.RemoteResource{
		RemoteID: remoteID,
		Summary: map[string]any{
			"deviceId": device.DeviceID, "level": alert.Level, "module": alert.Module, "code": alert.Code,
			"status": alert.Status, "beginTime": alert.BeginTime, "endTime": alert.EndTime,
		},
		Canonical: &connector.CanonicalResourceLink{TargetType: "device", TargetID: strconv.Itoa(device.DeviceID)},
	})
	for _, child := range alert.SubAlerts {
		resources = appendHMSResources(resources, device, child)
	}
	return resources
}

func secureRemoteKey(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:16])
}
