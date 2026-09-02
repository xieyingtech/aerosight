package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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
	AssertWritable(context.Context, connector.Instance) error
	ApplyRemoteResources(context.Context, connector.Instance, connector.RemoteResourceBatch) (connector.RemoteResourceApplyResult, error)
}

type DeviceFreshnessProjector interface {
	Record(context.Context, heartbeat.Signal) error
}

type DeviceHealthProjector interface {
	Apply(context.Context, connector.Instance, HealthPoll) error
}

type FlightCatalogProjector interface {
	ApplyWaylines(context.Context, connector.Instance, []WaylineSummary) error
	ApplyFlightTasks(context.Context, connector.Instance, []FlightTaskSummary) error
	ListArtifactTargets(context.Context, connector.Instance, int) ([]FlightArtifactTarget, error)
	ApplyFlightArtifacts(context.Context, connector.Instance, FlightArtifactPoll) error
	ApplyFlightExports(context.Context, connector.Instance, FlightExportPoll) error
	ApplyFlightAlerts(context.Context, connector.Instance, FlightAlertPoll) error
	ApplyAirSense(context.Context, connector.Instance, AirSensePoll) error
	ApplyModels(context.Context, connector.Instance, ModelCatalogPoll) error
}

type SQLResourceStreamSink struct {
	telemetry TelemetryBatchIngestor
	resources RemoteResourceWriter
	freshness DeviceFreshnessProjector
	health    DeviceHealthProjector
	flights   FlightCatalogProjector
}

func NewSQLResourceStreamSink(telemetryIngestor TelemetryBatchIngestor, resources RemoteResourceWriter, freshness DeviceFreshnessProjector, health DeviceHealthProjector, flights FlightCatalogProjector) (*SQLResourceStreamSink, error) {
	if telemetryIngestor == nil || resources == nil || freshness == nil || health == nil || flights == nil {
		return nil, errors.New("FlightHub resource stream sink dependencies are required")
	}
	return &SQLResourceStreamSink{telemetry: telemetryIngestor, resources: resources, freshness: freshness, health: health, flights: flights}, nil
}

func (sink *SQLResourceStreamSink) ApplyDeviceState(ctx context.Context, instance connector.Instance, poll DeviceStatePoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
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
			RequireActiveAdapter: true, AdapterLeaseOwner: instance.LeaseOwner, AdapterLeaseEpoch: instance.LeaseEpoch,
		})
	}
	batch = append(batch, telemetry.Telemetry{
		ProjectID: instance.ProjectID, TeamID: poll.Device.TeamID, AdapterID: instance.ID, DeviceID: poll.Device.DeviceID,
		EventID: eventPrefix + ":state", Type: "dji.flighthub.state", Sequence: &stateSequence,
		CapturedAt: capturedAt, ReceivedAt: poll.ReceivedAt, Payload: mappedPayload, Quality: quality,
		RequireActiveAdapter: true, AdapterLeaseOwner: instance.LeaseOwner, AdapterLeaseEpoch: instance.LeaseEpoch,
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
		LeaseOwner:         instance.LeaseOwner, LeaseEpoch: instance.LeaseEpoch, RequireActiveAdapter: true,
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
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
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
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	return sink.health.Apply(ctx, instance, poll)
}

func (sink *SQLResourceStreamSink) ApplyCatalog(ctx context.Context, instance connector.Instance, poll CatalogPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub catalog projection scope is invalid")
	}
	var resources []connector.RemoteResource
	var err error
	switch poll.Kind {
	case "wayline":
		resources, err = waylineRemoteResources(poll.Waylines)
	case "flight-task":
		resources, err = flightTaskRemoteResources(poll.FlightTasks)
	default:
		return errors.New("FlightHub catalog projection kind is invalid")
	}
	if err != nil {
		return err
	}
	_, err = sink.resources.ApplyRemoteResources(ctx, instance, connector.RemoteResourceBatch{
		Kind: poll.Kind, Resources: resources, CompleteSnapshot: poll.CompleteSnapshot,
	})
	if err != nil {
		return err
	}
	if poll.Kind == "wayline" {
		return sink.flights.ApplyWaylines(ctx, instance, poll.Waylines)
	}
	return sink.flights.ApplyFlightTasks(ctx, instance, poll.FlightTasks)
}

func (sink *SQLResourceStreamSink) ApplyLiveCatalog(ctx context.Context, instance connector.Instance, poll LiveCatalogPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub live catalog projection scope is invalid")
	}
	recordings, err := recordingRemoteResources(poll.Recordings)
	if err != nil {
		return err
	}
	shares, err := liveShareRemoteResources(poll.Shares)
	if err != nil {
		return err
	}
	converters, err := streamConverterRemoteResources(poll.Devices, poll.Converters)
	if err != nil {
		return err
	}
	batches := []connector.RemoteResourceBatch{
		{Kind: "recording", Resources: recordings, CompleteSnapshot: poll.RecordingComplete},
		{Kind: "live-share", Resources: shares, CompleteSnapshot: poll.ShareComplete},
		{Kind: "stream-converter", Resources: converters, CompleteSnapshot: poll.ConverterComplete},
	}
	var applyErrors []error
	for _, batch := range batches {
		if _, applyErr := sink.resources.ApplyRemoteResources(ctx, instance, batch); applyErr != nil {
			applyErrors = append(applyErrors, applyErr)
		}
	}
	return errors.Join(applyErrors...)
}

func (sink *SQLResourceStreamSink) ApplyModelCatalog(ctx context.Context, instance connector.Instance, poll ModelCatalogPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub model catalog projection scope is invalid")
	}
	if poll.ModelComplete && poll.Models == nil {
		poll.Models = []ModelSummary{}
	}
	if poll.OpenModelsComplete && poll.OpenModels == nil {
		poll.OpenModels = []OpenModel{}
	}
	models, err := modelRemoteResources(poll.Models)
	if err != nil {
		return err
	}
	openResources, err := openModelRemoteResources(poll.OpenModels, poll.Resources)
	if err != nil {
		return err
	}
	var applyErrors []error
	for _, batch := range []connector.RemoteResourceBatch{
		{Kind: "model", Resources: models, CompleteSnapshot: poll.ModelComplete},
		// Running open models are not a complete historical model/resource
		// directory. Never mark unseen resources missing from this view.
		{Kind: "model-resource", Resources: openResources, CompleteSnapshot: false},
	} {
		if _, applyErr := sink.resources.ApplyRemoteResources(ctx, instance, batch); applyErr != nil {
			applyErrors = append(applyErrors, applyErr)
		}
	}
	if len(applyErrors) == 0 {
		if applyErr := sink.flights.ApplyModels(ctx, instance, poll); applyErr != nil {
			applyErrors = append(applyErrors, applyErr)
		}
	}
	return errors.Join(applyErrors...)
}

var opaqueResourceIdentityFields = []string{
	"id", "uuid", "task_id", "task_uuid", "stream_id", "stream_uuid", "share_id", "share_uuid", "sn",
}

func opaqueResourceIdentity[T ~map[string]json.RawMessage](item T) (string, error) {
	for _, field := range opaqueResourceIdentityFields {
		raw := item[field]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		digest := sha256.Sum256(append([]byte(field+":"), raw...))
		return hex.EncodeToString(digest[:]), nil
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func opaqueRemoteResource[T ~map[string]json.RawMessage](scope string, deviceID int, item T) (connector.RemoteResource, error) {
	identity, err := opaqueResourceIdentity(item)
	if err != nil {
		return connector.RemoteResource{}, err
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return connector.RemoteResource{}, err
	}
	version := sha256.Sum256(raw)
	fields := make([]string, 0, len(item))
	for field := range item {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	summary := map[string]any{"scope": scope, "fieldNames": fields}
	if deviceID > 0 {
		summary["deviceId"] = deviceID
	}
	return connector.RemoteResource{
		RemoteID:      fmt.Sprintf("%s:%d:%s", scope, deviceID, identity),
		RemoteVersion: hex.EncodeToString(version[:]), Summary: summary,
	}, nil
}

func recordingRemoteResources(items []ScopedRecordingTask) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	for _, item := range items {
		if !validEnum(item.Scope, "project", "organization") || item.Device.DeviceID <= 0 {
			return nil, errors.New("FlightHub recording projection scope is invalid")
		}
		resource, err := opaqueRemoteResource(item.Scope, item.Device.DeviceID, item.Task)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func liveShareRemoteResources(items []LiveShare) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	for _, item := range items {
		resource, err := opaqueRemoteResource("project", 0, item)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func streamConverterRemoteResources(devices []connector.ManagedConnectorDevice, items []StreamConverter) ([]connector.RemoteResource, error) {
	bySerial := make(map[string]int, len(devices))
	for _, device := range devices {
		bySerial[device.Serial] = device.DeviceID
	}
	resources := make([]connector.RemoteResource, 0, len(items))
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		version := sha256.Sum256(raw)
		summary := map[string]any{
			"name": item.Name, "state": item.State, "code": item.Code, "schema": item.Schema,
			"cameraIndex": item.CameraIndex, "videoType": item.VideoType,
			"autoPushStream": item.AutoPushStream, "deviceOnline": item.DeviceOnline,
		}
		if deviceID := bySerial[item.SN]; deviceID > 0 {
			summary["deviceId"] = deviceID
		} else {
			summary["managedDevice"] = false
		}
		resource := connector.RemoteResource{
			RemoteID: item.ID, RemoteVersion: hex.EncodeToString(version[:]), Summary: summary,
		}
		if updatedAt, parseErr := time.Parse(time.RFC3339, item.UpdatedAt); parseErr == nil {
			updatedAt = updatedAt.UTC()
			resource.RemoteUpdatedAt = &updatedAt
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (sink *SQLResourceStreamSink) ListFlightArtifactTargets(ctx context.Context, instance connector.Instance, limit int) ([]FlightArtifactTarget, error) {
	return sink.flights.ListArtifactTargets(ctx, instance, limit)
}

func (sink *SQLResourceStreamSink) ApplyFlightArtifacts(ctx context.Context, instance connector.Instance, poll FlightArtifactPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	return sink.flights.ApplyFlightArtifacts(ctx, instance, poll)
}

func (sink *SQLResourceStreamSink) ApplyFlightExports(ctx context.Context, instance connector.Instance, poll FlightExportPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	return sink.flights.ApplyFlightExports(ctx, instance, poll)
}

func (sink *SQLResourceStreamSink) ApplyFlightAlerts(ctx context.Context, instance connector.Instance, poll FlightAlertPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	return sink.flights.ApplyFlightAlerts(ctx, instance, poll)
}

func waylineRemoteResources(items []WaylineSummary) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, schemaError()
		}
		seen[item.ID] = struct{}{}
		var updatedAt *time.Time
		if item.UpdatedAt > 0 {
			value := time.UnixMilli(item.UpdatedAt).UTC()
			updatedAt = &value
		}
		resources = append(resources, connector.RemoteResource{
			RemoteID: item.ID, RemoteVersion: fmt.Sprintf("%d:%d", item.UpdatedAt, item.SizeBytes), RemoteUpdatedAt: updatedAt,
			Summary: map[string]any{
				"name": item.Name, "deviceModelKey": item.DeviceModelKey, "templateTypes": item.TemplateTypes,
				"updatedAt": item.UpdatedAt, "sizeBytes": item.SizeBytes, "payloadCount": len(item.PayloadInformation),
			},
		})
	}
	return resources, nil
}

func flightTaskRemoteResources(items []FlightTaskSummary) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, duplicate := seen[item.UUID]; duplicate {
			return nil, schemaError()
		}
		seen[item.UUID] = struct{}{}
		summary := map[string]any{
			"name": item.Name, "taskType": item.TaskType, "status": item.Status,
			"beginAt": item.BeginAt, "endAt": item.EndAt, "runAt": item.RunAt, "completedAt": item.CompletedAt,
			"mediaUploadStatus": item.MediaUploadStatus, "resumableStatus": item.ResumableStatus,
			"breakPointResume": item.BreakPointResume, "currentWaypoint": item.CurrentWaypoint,
			"totalWaypoints": item.TotalWaypoints, "exceptionCount": len(item.Exceptions),
		}
		serialized, err := json.Marshal(summary)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(serialized)
		resources = append(resources, connector.RemoteResource{
			RemoteID: item.UUID, RemoteVersion: hex.EncodeToString(digest[:16]), RemoteUpdatedAt: latestFlightTaskTime(item), Summary: summary,
		})
	}
	return resources, nil
}

func latestFlightTaskTime(item FlightTaskSummary) *time.Time {
	for _, value := range []string{item.CompletedAt, item.RunAt, item.BeginAt, item.EndAt} {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
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
