package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/adapter"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/heartbeat"
	"aerosight/worker/internal/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type telemetryIngestorFixture struct {
	batches [][]telemetry.Telemetry
}

func (fixture *telemetryIngestorFixture) IngestBatch(_ context.Context, batch []telemetry.Telemetry) (int, error) {
	fixture.batches = append(fixture.batches, append([]telemetry.Telemetry(nil), batch...))
	return len(batch), nil
}

type remoteResourceWriterFixture struct {
	batches     []connector.RemoteResourceBatch
	writableErr error
}

func (fixture *remoteResourceWriterFixture) AssertWritable(context.Context, connector.Instance) error {
	return fixture.writableErr
}

type freshnessProjectorFixture struct {
	signals []heartbeat.Signal
}

func TestResourceSinkRejectsDisabledConnectorBeforeTelemetryWrite(t *testing.T) {
	ingestor := &telemetryIngestorFixture{}
	resources := &remoteResourceWriterFixture{writableErr: connector.ErrConnectorDisabled}
	sink, _ := NewSQLResourceStreamSink(ingestor, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	snapshot := DeviceStateSnapshot{SN: "AIRCRAFT_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{"mode_code": json.RawMessage(`14`)}}
	err := sink.ApplyDeviceState(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, DeviceStatePoll{
		Device:   connector.ManagedConnectorDevice{DeviceID: 11, TeamID: 2, Serial: snapshot.SN},
		Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: time.Now().UTC(),
	})
	if !errors.Is(err, connector.ErrConnectorDisabled) || len(ingestor.batches) != 0 {
		t.Fatalf("disabled connector wrote telemetry: batches=%d err=%v", len(ingestor.batches), err)
	}
}

type healthProjectorFixture struct {
	polls []HealthPoll
}

type flightCatalogProjectorFixture struct {
	waylines      int
	tasks         int
	alerts        int
	airSensePolls []AirSensePoll
}

func (fixture *flightCatalogProjectorFixture) ApplyWaylines(_ context.Context, _ connector.Instance, items []WaylineSummary) error {
	fixture.waylines += len(items)
	return nil
}

func (fixture *flightCatalogProjectorFixture) ApplyFlightTasks(_ context.Context, _ connector.Instance, items []FlightTaskSummary) error {
	fixture.tasks += len(items)
	return nil
}

func (fixture *flightCatalogProjectorFixture) ListArtifactTargets(context.Context, connector.Instance, int) ([]FlightArtifactTarget, error) {
	return []FlightArtifactTarget{}, nil
}

func (fixture *flightCatalogProjectorFixture) ApplyFlightArtifacts(context.Context, connector.Instance, FlightArtifactPoll) error {
	return nil
}

func (fixture *flightCatalogProjectorFixture) ApplyFlightExports(context.Context, connector.Instance, FlightExportPoll) error {
	return nil
}

func (fixture *flightCatalogProjectorFixture) ApplyFlightAlerts(_ context.Context, _ connector.Instance, poll FlightAlertPoll) error {
	fixture.alerts += len(poll.Alerts)
	return nil
}

func (fixture *flightCatalogProjectorFixture) ApplyAirSense(_ context.Context, _ connector.Instance, poll AirSensePoll) error {
	fixture.airSensePolls = append(fixture.airSensePolls, poll)
	return nil
}

func (fixture *healthProjectorFixture) Apply(_ context.Context, _ connector.Instance, poll HealthPoll) error {
	fixture.polls = append(fixture.polls, poll)
	return nil
}

func (fixture *freshnessProjectorFixture) Record(_ context.Context, signal heartbeat.Signal) error {
	fixture.signals = append(fixture.signals, signal)
	return nil
}

type fixedHeartbeatClock struct{ now time.Time }

func (clock *fixedHeartbeatClock) Now() time.Time { return clock.now }

func (fixture *remoteResourceWriterFixture) ApplyRemoteResources(_ context.Context, _ connector.Instance, batch connector.RemoteResourceBatch) (connector.RemoteResourceApplyResult, error) {
	fixture.batches = append(fixture.batches, batch)
	return connector.RemoteResourceApplyResult{Upserted: len(batch.Resources)}, nil
}

func TestResourceSinkBuildsTransactionalStateAndPoseBatch(t *testing.T) {
	ingestor := &telemetryIngestorFixture{}
	resources := &remoteResourceWriterFixture{}
	freshness := &freshnessProjectorFixture{}
	sink, err := NewSQLResourceStreamSink(ingestor, resources, freshness, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 9, 1, 10, 0, 1, 0, time.UTC)
	snapshot := DeviceStateSnapshot{SN: "AIRCRAFT_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{
		"longitude": json.RawMessage(`120`), "latitude": json.RawMessage(`30`), "height": json.RawMessage(`48.2`),
		"attitude_head": json.RawMessage(`180`), "battery": json.RawMessage(`{"capacity_percent":76}`),
		"device_data_update_time": json.RawMessage(`1788256800000`),
	}}
	poll := DeviceStatePoll{
		Device:   connector.ManagedConnectorDevice{DeviceID: 11, TeamID: 2, Serial: snapshot.SN, Online: true},
		Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: receivedAt,
	}
	if err := sink.ApplyDeviceState(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(ingestor.batches) != 1 || len(ingestor.batches[0]) != 2 {
		t.Fatalf("state and pose must share one ingestion transaction: %#v", ingestor.batches)
	}
	poseItem, stateItem := ingestor.batches[0][0], ingestor.batches[0][1]
	if poseItem.Type != "telemetry.pose" || stateItem.Type != "dji.flighthub.state" || !poseItem.CapturedAt.Before(receivedAt) || poseItem.ReceivedAt != receivedAt || poseItem.EventID == stateItem.EventID {
		t.Fatalf("telemetry batch lost time/source/idempotency metadata: %#v", ingestor.batches[0])
	}
	var pose adapter.Pose
	if json.Unmarshal(poseItem.Payload, &pose) != nil || pose.CRS != "dji-flighthub:unverified" || pose.TransformVersion != StateMapperVersion || pose.Orientation == nil {
		t.Fatalf("pose payload=%s", poseItem.Payload)
	}
	var quality map[string]any
	if json.Unmarshal(stateItem.Quality, &quality) != nil || quality["capturedAtSource"] != "device_data_update_time:milliseconds" || quality["source"] != "dji-flighthub-openapi" {
		t.Fatalf("quality=%s", stateItem.Quality)
	}
	if len(freshness.signals) != 1 || !freshness.signals[0].ObservedAt.Equal(poseItem.CapturedAt) || freshness.signals[0].ReceivedAt != receivedAt {
		t.Fatalf("freshness signal lost upstream/local time separation: %#v", freshness.signals)
	}
}

func TestResourceSinkKeepsInvalidCoordinatesOutOfPoseButRetainsState(t *testing.T) {
	ingestor := &telemetryIngestorFixture{}
	sink, _ := NewSQLResourceStreamSink(ingestor, &remoteResourceWriterFixture{}, &freshnessProjectorFixture{}, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	snapshot := DeviceStateSnapshot{SN: "AIRCRAFT_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{
		"longitude": json.RawMessage(`999`), "latitude": json.RawMessage(`-999`), "mode_code": json.RawMessage(`14`),
	}}
	poll := DeviceStatePoll{Device: connector.ManagedConnectorDevice{DeviceID: 11, TeamID: 2, Serial: snapshot.SN}, Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: time.Now().UTC()}
	if err := sink.ApplyDeviceState(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(ingestor.batches) != 1 || len(ingestor.batches[0]) != 1 || ingestor.batches[0][0].Type != "dji.flighthub.state" {
		t.Fatalf("invalid coordinate created a pose: %#v", ingestor.batches)
	}
}

func TestResourceSinkFreshnessUsesUpstreamCaptureTimeInsteadOfPollTime(t *testing.T) {
	ingestor := &telemetryIngestorFixture{}
	freshness := &freshnessProjectorFixture{}
	sink, _ := NewSQLResourceStreamSink(ingestor, &remoteResourceWriterFixture{}, freshness, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	capturedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	receivedAt := capturedAt.Add(5 * time.Minute)
	snapshot := DeviceStateSnapshot{SN: "AIRCRAFT_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{
		"device_data_update_time": json.RawMessage(strconv.FormatInt(capturedAt.UnixMilli(), 10)),
		"mode_code":               json.RawMessage(`14`),
	}}
	poll := DeviceStatePoll{
		Device:   connector.ManagedConnectorDevice{DeviceID: 11, TeamID: 2, Serial: snapshot.SN, Online: true},
		Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: receivedAt, FreshnessInterval: 15 * time.Second,
	}
	if err := sink.ApplyDeviceState(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(freshness.signals) != 1 {
		t.Fatalf("freshness signals=%#v", freshness.signals)
	}
	signal := freshness.signals[0]
	projection := heartbeat.Evaluate(receivedAt, &signal.ObservedAt, nil, time.Duration(signal.HeartbeatIntervalSecond)*time.Second, nil, false)
	if projection.Status != "offline" || !signal.ObservedAt.Equal(capturedAt) || signal.ReceivedAt != receivedAt {
		t.Fatalf("old upstream state was disguised as current: signal=%#v projection=%#v", signal, projection)
	}
}

func TestResourceSinkProjectsHealthWithoutPersistingSerialsOrMessages(t *testing.T) {
	resources := &remoteResourceWriterFixture{}
	health := &healthProjectorFixture{}
	sink, _ := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, health, &flightCatalogProjectorFixture{})
	alert := HMSAlert{ID: "HMS_REDACTED", Code: "CODE_REDACTED", Message: "vendor message must not persist", Level: "warning", Module: "hms", Status: 1}
	poll := HealthPoll{
		Devices: []connector.ManagedConnectorDevice{{DeviceID: 11, TeamID: 2, Serial: "AIRCRAFT_REDACTED"}},
		HMS: []DeviceHMS{{SN: "AIRCRAFT_REDACTED", Alerts: struct {
			List []HMSAlert `json:"list"`
		}{List: []HMSAlert{alert}}}},
		AutoRecord: AutoRecordingConfig{ID: 1, ProjectID: "PROJECT_REDACTED", UpdatedAt: "v1", Items: []AutoRecordingItem{{SN: "AIRCRAFT_REDACTED", CameraIndex: "0-0-0", RecordingStrategy: 1}}},
		ReceivedAt: time.Now().UTC(),
	}
	if err := sink.ApplyHealth(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(resources.batches) != 3 || resources.batches[0].Kind != "hms" || resources.batches[1].Kind != "topology" || resources.batches[2].Kind != "auto-record" || !resources.batches[0].CompleteSnapshot || len(health.polls) != 1 {
		t.Fatalf("health projections=%#v", resources.batches)
	}
	serialized, _ := json.Marshal(resources.batches)
	if strings.Contains(string(serialized), "AIRCRAFT_REDACTED") || strings.Contains(string(serialized), alert.Message) {
		t.Fatalf("health projection persisted serial or raw message: %s", serialized)
	}
}

func TestResourceSinkProjectsCatalogsWithStableIdentityAndSanitizedSummaries(t *testing.T) {
	resources := &remoteResourceWriterFixture{}
	flights := &flightCatalogProjectorFixture{}
	sink, _ := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, flights)
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	waylinePoll := CatalogPoll{
		Kind: "wayline", CompleteSnapshot: true, ReceivedAt: now,
		Waylines: []WaylineSummary{{
			ID: "WAYLINE_REDACTED", Name: "脱敏航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"},
			PayloadInformation: []WaylinePayload{{Domain: "1", Type: "98", LensType: "wide"}}, UpdatedAt: now.UnixMilli(), SizeBytes: 1024,
		}},
	}
	taskPoll := CatalogPoll{
		Kind: "flight-task", CompleteSnapshot: true, ReceivedAt: now,
		FlightTasks: []FlightTaskSummary{{
			UUID: "TASK_REDACTED", Name: "脱敏任务", TaskType: "immediate", Status: "success",
			SN: "DOCK_REDACTED", WaylineUUID: "WAYLINE_REDACTED", BeginAt: "2026-09-01T09:00:00Z", CompletedAt: "2026-09-01T10:00:00Z",
		}},
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := sink.ApplyCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, waylinePoll); err != nil {
			t.Fatal(err)
		}
		if err := sink.ApplyCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, taskPoll); err != nil {
			t.Fatal(err)
		}
	}
	if len(resources.batches) != 4 {
		t.Fatalf("catalog batches=%#v", resources.batches)
	}
	if flights.waylines != 2 || flights.tasks != 2 {
		t.Fatalf("canonical projector calls waylines=%d tasks=%d", flights.waylines, flights.tasks)
	}
	firstWayline, secondWayline := resources.batches[0].Resources[0], resources.batches[2].Resources[0]
	firstTask, secondTask := resources.batches[1].Resources[0], resources.batches[3].Resources[0]
	if firstWayline.RemoteID != secondWayline.RemoteID || firstWayline.RemoteVersion != secondWayline.RemoteVersion || firstTask.RemoteID != secondTask.RemoteID || firstTask.RemoteVersion != secondTask.RemoteVersion {
		t.Fatalf("repeated catalog projection was not idempotent: waylines=%#v/%#v tasks=%#v/%#v", firstWayline, secondWayline, firstTask, secondTask)
	}
	for _, resource := range []connector.RemoteResource{firstWayline, firstTask} {
		summary, _ := json.Marshal(resource.Summary)
		if strings.Contains(string(summary), "WAYLINE_REDACTED") || strings.Contains(string(summary), "TASK_REDACTED") || strings.Contains(string(summary), "DOCK_REDACTED") || strings.Contains(string(summary), "http") {
			t.Fatalf("catalog summary persisted remote identity or URL: %s", summary)
		}
	}
}

func TestResourceSinkProjectsLiveCatalogsIdempotentlyWithoutSecrets(t *testing.T) {
	resources := &remoteResourceWriterFixture{}
	sink, _ := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	recording := RecordingTask{
		"task_id":  json.RawMessage(`"RECORDING_REDACTED"`),
		"status":   json.RawMessage(`"running"`),
		"password": json.RawMessage(`"RECORDING_PASSWORD_MUST_NOT_PERSIST"`),
	}
	share := LiveShare{
		"share_id": json.RawMessage(`"SHARE_REDACTED"`),
		"status":   json.RawMessage(`1`),
		"token":    json.RawMessage(`"SHARE_TOKEN_MUST_NOT_PERSIST"`),
	}
	converter := StreamConverter{
		ID: "CONVERTER_REDACTED", Name: "脱敏转发", State: "running", UpdatedAt: "2026-09-02T05:00:00Z",
		SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", Video: "normal-0", VideoType: "wide",
		Schema: "rtsp", AutoPushStream: true, DeviceOnline: true,
		BypassOption: &StreamConverterBypassOption{
			RTSPURL: "rtsp://media.vendor.example/SECRET_PATH", Username: "SECRET_USERNAME", Password: "SECRET_PASSWORD",
		},
	}
	poll := LiveCatalogPoll{
		Devices: []connector.ManagedConnectorDevice{{DeviceID: 11, TeamID: 2, Serial: "AIRCRAFT_REDACTED"}},
		Recordings: []ScopedRecordingTask{{Scope: "project", Device: connector.ManagedConnectorDevice{
			DeviceID: 11, TeamID: 2, Serial: "AIRCRAFT_REDACTED",
		}, Task: recording}},
		Shares: []LiveShare{share}, Converters: []StreamConverter{converter},
		RecordingComplete: true, ShareComplete: true, ConverterComplete: true,
		ReceivedAt: time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC),
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := sink.ApplyLiveCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
			t.Fatal(err)
		}
	}
	if len(resources.batches) != 6 {
		t.Fatalf("live batches=%#v", resources.batches)
	}
	for index, repeated := range []int{3, 4, 5} {
		first := resources.batches[index].Resources[0]
		second := resources.batches[repeated].Resources[0]
		if first.RemoteID != second.RemoteID || first.RemoteVersion != second.RemoteVersion {
			t.Fatalf("live projection was not idempotent: %#v / %#v", first, second)
		}
	}
	serialized, _ := json.Marshal(resources.batches)
	for _, secret := range []string{
		"RECORDING_REDACTED", "RECORDING_PASSWORD_MUST_NOT_PERSIST", "SHARE_REDACTED",
		"SHARE_TOKEN_MUST_NOT_PERSIST", "AIRCRAFT_REDACTED", "SECRET_PATH", "SECRET_USERNAME", "SECRET_PASSWORD",
	} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("live projection persisted secret %q: %s", secret, serialized)
		}
	}
	empty := LiveCatalogPoll{
		RecordingComplete: true, ShareComplete: true, ConverterComplete: true,
		ReceivedAt: poll.ReceivedAt.Add(time.Minute),
	}
	if err := sink.ApplyLiveCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, empty); err != nil {
		t.Fatal(err)
	}
	if len(resources.batches) != 9 {
		t.Fatalf("empty live batches=%#v", resources.batches)
	}
	for _, batch := range resources.batches[6:] {
		if !batch.CompleteSnapshot || len(batch.Resources) != 0 {
			t.Fatalf("empty live catalog was not a complete healthy snapshot: %#v", batch)
		}
	}
}

func TestSQLResourceSinkIdempotencyOrderingAndInvalidCoordinates(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var teamID, projectID, deviceID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-state-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-state-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-state-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
		select $1,$2,id,'state test aircraft','drone','unknown' from device_types
		 where type_key='dji.matrice3td' and status='active' order by version desc limit 1 returning id`, projectID, adapterID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}

	clock := &fixedHeartbeatClock{now: time.Date(2026, 9, 1, 10, 0, 1, 0, time.UTC)}
	freshness := heartbeat.NewProjector(database, clock)
	sink, err := NewSQLResourceStreamSink(telemetry.NewIngestor(database), connector.NewSQLResourceRepository(database), freshness, NewSQLDeviceHealthProjector(database), &flightCatalogProjectorFixture{})
	if err != nil {
		t.Fatal(err)
	}
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	device := connector.ManagedConnectorDevice{DeviceID: deviceID, TeamID: teamID, Serial: "AIRCRAFT_REDACTED", Online: true}
	makePoll := func(captured time.Time, longitude, latitude float64) DeviceStatePoll {
		snapshot := DeviceStateSnapshot{SN: device.Serial, Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{
			"longitude":               json.RawMessage(strconv.FormatFloat(longitude, 'f', -1, 64)),
			"latitude":                json.RawMessage(strconv.FormatFloat(latitude, 'f', -1, 64)),
			"height":                  json.RawMessage(`20`),
			"device_data_update_time": json.RawMessage(strconv.FormatInt(captured.UnixMilli(), 10)),
		}}
		return DeviceStatePoll{Device: device, Snapshot: snapshot, Mapped: MapDeviceState(snapshot), ReceivedAt: captured.Add(time.Second)}
	}
	newer := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	newerPoll := makePoll(newer, 120, 30)
	if err := sink.ApplyDeviceState(ctx, instance, newerPoll); err != nil {
		t.Fatal(err)
	}
	if err := sink.ApplyDeviceState(ctx, instance, newerPoll); err != nil {
		t.Fatal(err)
	}
	older := makePoll(newer.Add(-time.Minute), 119, 29)
	if err := sink.ApplyDeviceState(ctx, instance, older); err != nil {
		t.Fatal(err)
	}
	invalidCaptured := newer.Add(time.Minute)
	invalid := makePoll(invalidCaptured, 999, -999)
	if err := sink.ApplyDeviceState(ctx, instance, invalid); err != nil {
		t.Fatal(err)
	}

	var telemetryCount, observationCount, poseCount int
	if err := database.QueryRowContext(ctx, `select count(*) from device_telemetry where project_id=$1 and device_id=$2`, projectID, deviceID).Scan(&telemetryCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from observations where project_id=$1 and device_id=$2`, projectID, deviceID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from poses where project_id=$1 and device_id=$2`, projectID, deviceID).Scan(&poseCount); err != nil {
		t.Fatal(err)
	}
	if telemetryCount != 5 || observationCount != 2 || poseCount != 2 {
		t.Fatalf("idempotent projection counts telemetry=%d observations=%d poses=%d", telemetryCount, observationCount, poseCount)
	}
	var latestCaptured, lastSeen, latestPose time.Time
	if err := database.QueryRowContext(ctx, `select captured_at from device_latest_telemetry where project_id=$1 and device_id=$2`, projectID, deviceID).Scan(&latestCaptured); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select last_seen_at from devices where project_id=$1 and id=$2`, projectID, deviceID).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	var originalPresent, standardPresent bool
	var transformVersion, spatialQuality string
	if err := database.QueryRowContext(ctx, `select captured_at,original_position is not null,standard_position is not null,transform_version,spatial_quality
		from poses where project_id=$1 and device_id=$2 order by captured_at desc limit 1`, projectID, deviceID).Scan(&latestPose, &originalPresent, &standardPresent, &transformVersion, &spatialQuality); err != nil {
		t.Fatal(err)
	}
	if !latestCaptured.Equal(invalidCaptured) || !lastSeen.Equal(invalidCaptured) || !latestPose.Equal(newer) || !originalPresent || standardPresent || transformVersion != StateMapperVersion || spatialQuality != "unusable" {
		t.Fatalf("projection ordering/latest pose invalid latest=%s lastSeen=%s pose=%s original=%v standard=%v transform=%s quality=%s", latestCaptured, lastSeen, latestPose, originalPresent, standardPresent, transformVersion, spatialQuality)
	}
}
