package flighthub

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const flightProjectorTestSecret = "0123456789abcdef0123456789abcdef"

func TestDesiredTaskRunStateMapsFlightHubLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	firstSeen := now.Add(-time.Minute)
	tests := []struct {
		name       string
		item       FlightTaskSummary
		wantStatus string
		wantReason string
		wantError  bool
	}{
		{name: "accepted", item: FlightTaskSummary{Status: "waiting"}, wantStatus: "dispatching", wantReason: "flighthub_accepted"},
		{name: "running", item: FlightTaskSummary{Status: "executing", RunAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}, wantStatus: "running", wantReason: "flighthub_executing"},
		{name: "paused", item: FlightTaskSummary{Status: "paused"}, wantStatus: "paused", wantReason: "flighthub_paused"},
		{name: "success", item: FlightTaskSummary{Status: "success", CompletedAt: now.Format(time.RFC3339Nano)}, wantStatus: "succeeded", wantReason: "flighthub_succeeded"},
		{name: "failure", item: FlightTaskSummary{Status: "partially_done"}, wantStatus: "failed", wantReason: "flighthub_failed", wantError: true},
		{name: "upstream timeout", item: FlightTaskSummary{Status: "timeout"}, wantStatus: "failed", wantReason: "flighthub_timeout", wantError: true},
		{name: "canceled", item: FlightTaskSummary{Status: "terminated"}, wantStatus: "canceled", wantReason: "flighthub_canceled"},
		{name: "unknown timeout", item: FlightTaskSummary{Status: "vendor_future_state", EndAt: now.Add(-time.Hour).Format(time.RFC3339Nano)}, wantStatus: "blocked", wantReason: "flighthub_result_unknown_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := desiredTaskRunState(test.item, now, 30*time.Minute, firstSeen)
			if state.Status != test.wantStatus || state.Reason != test.wantReason || (state.Error != nil) != test.wantError {
				t.Fatalf("state=%#v", state)
			}
		})
	}
}

func TestFlightTaskTransitionGuardsRegressionAndUnknownRecovery(t *testing.T) {
	terminal := desiredFlightRunState{Status: "succeeded"}
	if transitionAllowed("succeeded", "flighthub_succeeded", desiredFlightRunState{Status: "running"}) {
		t.Fatal("terminal state regressed to running")
	}
	if transitionAllowed("paused", "flighthub_paused", desiredFlightRunState{Status: "dispatching"}) {
		t.Fatal("paused state regressed to accepted")
	}
	if transitionAllowed("blocked", "flighthub_result_unknown_timeout", desiredFlightRunState{Status: "running"}) {
		t.Fatal("unknown timeout recovered without a terminal result")
	}
	if !transitionAllowed("blocked", "flighthub_result_unknown_timeout", terminal) {
		t.Fatal("unknown timeout did not recover from a confirmed terminal result")
	}
}

func TestSQLFlightCatalogProjectorConvertsWaylinesAndReconcilesRuns(t *testing.T) {
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
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-flight-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-flight-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-flight-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
		select $1,$2,id,'flight test dock','dock','unknown' from device_types
		 where type_key='dji.dock2' and status='active' order by version desc limit 1 returning id`, projectID, adapterID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	secretSN := fmt.Sprintf("DOCK_SECRET_%d", suffix)
	if _, err := database.ExecContext(ctx, `insert into device_external_identities(
		project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at
	) values($1,$2,$3,$4,$5,'dji.dock2',jsonb_build_object('attributes',jsonb_build_object('serialNumber',$6::text)),'managed',now())`,
		projectID, teamID, adapterID, deviceID, secureRemoteKey(secretSN), secretSN); err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	projector := NewSQLFlightCatalogProjector(database, telemetry.NewIngestor(database), func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	waylineID := fmt.Sprintf("WAYLINE_SECRET_%d", suffix)
	unknownWaylineID := fmt.Sprintf("WAYLINE_UNKNOWN_SECRET_%d", suffix)
	waylines := []WaylineSummary{
		{ID: waylineID, Name: "可转换航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"}, UpdatedAt: clock.Add(-time.Hour).UnixMilli(), SizeBytes: 1024},
		{ID: unknownWaylineID, Name: "未知模板航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"vendor_future_template"}, UpdatedAt: clock.Add(-time.Hour).UnixMilli(), SizeBytes: 2048},
	}
	applyWaylines := func() {
		t.Helper()
		if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "wayline", Waylines: waylines, CompleteSnapshot: true, ReceivedAt: clock}); err != nil {
			t.Fatal(err)
		}
	}
	applyWaylines()
	applyWaylines()
	var taskID int
	var versionCount int
	if err := database.QueryRowContext(ctx, `select canonical_target_id::int from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='wayline' and remote_id=$3`, projectID, adapterID, waylineID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from task_versions where project_id=$1 and task_id=$2`, projectID, taskID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 1 {
		t.Fatalf("same wayline version created %d task versions", versionCount)
	}
	var unknownLinked bool
	if err := database.QueryRowContext(ctx, `select canonical_target_id is not null from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='wayline' and remote_id=$3`, projectID, adapterID, unknownWaylineID).Scan(&unknownLinked); err != nil {
		t.Fatal(err)
	}
	if unknownLinked {
		t.Fatal("unknown wayline template created a canonical task")
	}
	waylines[0].UpdatedAt = clock.UnixMilli()
	waylines[0].SizeBytes++
	applyWaylines()
	if err := database.QueryRowContext(ctx, `select count(*) from task_versions where project_id=$1 and task_id=$2`, projectID, taskID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 {
		t.Fatalf("changed wayline version created %d task versions", versionCount)
	}

	mainUUID := fmt.Sprintf("TASK_MAIN_SECRET_%d", suffix)
	failureUUID := fmt.Sprintf("TASK_FAILURE_SECRET_%d", suffix)
	canceledUUID := fmt.Sprintf("TASK_CANCELED_SECRET_%d", suffix)
	unknownUUID := fmt.Sprintf("TASK_UNKNOWN_SECRET_%d", suffix)
	mainTask := FlightTaskSummary{
		UUID: mainUUID, Name: "主任务", SN: secretSN, WaylineUUID: waylineID, Status: "waiting",
		BeginAt: clock.Add(-time.Minute).Format(time.RFC3339Nano), EndAt: clock.Add(time.Hour).Format(time.RFC3339Nano),
	}
	allTasks := []FlightTaskSummary{mainTask}
	applyTasks := func() {
		t.Helper()
		if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "flight-task", FlightTasks: allTasks, CompleteSnapshot: true, ReceivedAt: clock}); err != nil {
			t.Fatal(err)
		}
	}
	applyTasks()
	for _, status := range []string{"executing", "paused", "executing", "success"} {
		allTasks[0].Status = status
		if status == "executing" {
			allTasks[0].RunAt = clock.Add(-30 * time.Second).Format(time.RFC3339Nano)
		}
		if status == "success" {
			allTasks[0].CompletedAt = clock.Format(time.RFC3339Nano)
		}
		applyTasks()
	}
	applyTasks()
	allTasks = append(allTasks,
		FlightTaskSummary{UUID: failureUUID, Name: "失败任务", SN: secretSN, WaylineUUID: waylineID, Status: "starting_failure", BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano)},
		FlightTaskSummary{UUID: canceledUUID, Name: "取消任务", SN: secretSN, WaylineUUID: waylineID, Status: "terminated", BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano)},
		FlightTaskSummary{UUID: unknownUUID, Name: "未知任务", SN: secretSN, WaylineUUID: unknownWaylineID, Status: "vendor_future_state", EndAt: clock.Add(-time.Hour).Format(time.RFC3339Nano)},
	)
	applyTasks()
	applyTasks()

	assertRun := func(remoteID, wantStatus, wantReason string, wantVersion, wantEvents int) int {
		t.Helper()
		var runID int
		var status, reason, triggerKey string
		var stateVersion, eventCount int
		if err := database.QueryRowContext(ctx, `select run.id,run.status,run.state_reason,run.state_version,run.trigger_key
			from connector_remote_resources resource join task_runs run
			  on resource.project_id=run.project_id and resource.canonical_target_type='task_run' and resource.canonical_target_id=run.id::text
			where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='flight-task' and resource.remote_id=$3`,
			projectID, adapterID, remoteID).Scan(&runID, &status, &reason, &stateVersion, &triggerKey); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1 and event_type='task_run.transitioned' and payload_json->>'taskRunId'=$2`, projectID, fmt.Sprint(runID)).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || reason != wantReason || stateVersion != wantVersion || eventCount != wantEvents {
			t.Fatalf("run %d status=%s reason=%s version=%d events=%d", runID, status, reason, stateVersion, eventCount)
		}
		if strings.Contains(triggerKey, remoteID) {
			t.Fatalf("trigger key leaked remote identity: %s", triggerKey)
		}
		return runID
	}
	mainRunID := assertRun(mainUUID, "succeeded", "flighthub_succeeded", 4, 5)
	assertRun(failureUUID, "failed", "flighthub_failed", 0, 1)
	assertRun(canceledUUID, "canceled", "flighthub_canceled", 0, 1)
	unknownRunID := assertRun(unknownUUID, "blocked", "flighthub_result_unknown_timeout", 0, 1)

	allTasks[3].Status = "success"
	allTasks[3].CompletedAt = clock.Format(time.RFC3339Nano)
	applyTasks()
	assertRun(unknownUUID, "succeeded", "flighthub_succeeded", 1, 2)

	var fallbackTaskID int
	if err := database.QueryRowContext(ctx, `select task_id from task_runs where project_id=$1 and id=$2`, projectID, unknownRunID).Scan(&fallbackTaskID); err != nil {
		t.Fatal(err)
	}
	if fallbackTaskID == taskID {
		t.Fatal("flight task with an unconvertible wayline did not receive an isolated fallback task")
	}
	var startedAt, finishedAt time.Time
	if err := database.QueryRowContext(ctx, `select started_at,finished_at from task_runs where project_id=$1 and id=$2`, projectID, mainRunID).Scan(&startedAt, &finishedAt); err != nil {
		t.Fatal(err)
	}
	wantStartedAt, _ := time.Parse(time.RFC3339Nano, allTasks[0].RunAt)
	wantFinishedAt, _ := time.Parse(time.RFC3339Nano, allTasks[0].CompletedAt)
	if !startedAt.Equal(wantStartedAt) || !finishedAt.Equal(wantFinishedAt) {
		t.Fatalf("remote timeline was not reconciled: started=%s finished=%s", startedAt, finishedAt)
	}
	var outboxCount int
	if err := database.QueryRowContext(ctx, `select count(*) from outbox_events where project_id=$1 and event_type='task_run.transitioned'`, projectID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("read projection emitted %d mission execution events", outboxCount)
	}
	var projections string
	if err := database.QueryRowContext(ctx, `select concat_ws(' ',
		coalesce((select string_agg(definition_json::text,' ') from task_versions where project_id=$1),''),
		coalesce((select string_agg(input_snapshot_json::text,' ') from task_runs where project_id=$1),''),
		coalesce((select string_agg(payload_json::text,' ') from project_events where project_id=$1),''))`, projectID).Scan(&projections); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{secretSN, waylineID, unknownWaylineID, mainUUID, failureUUID, canceledUUID, unknownUUID} {
		if strings.Contains(projections, secret) {
			t.Fatalf("canonical snapshots or events leaked remote identity %q", secret)
		}
	}
	var selectedDeviceID sql.NullInt64
	if err := database.QueryRowContext(ctx, `select selected_device_id from task_runs where project_id=$1 and id=$2`, projectID, mainRunID).Scan(&selectedDeviceID); err != nil {
		t.Fatal(err)
	}
	if !selectedDeviceID.Valid || int(selectedDeviceID.Int64) != deviceID {
		t.Fatalf("managed dock was not associated with the remote run: %#v", selectedDeviceID)
	}
}

func TestSQLFlightArtifactProjectorStoresIdempotentTaskTrajectoryAndSanitizedTimeline(t *testing.T) {
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
	var teamID, projectID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-artifact-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-artifact-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-artifact-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	createDevice := func(typeKey, kind, serial string) int {
		t.Helper()
		var deviceID int
		if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
			select $1,$2,id,$3,$4,'unknown' from device_types where type_key=$5 and status='active'
			order by version desc limit 1 returning id`, projectID, adapterID, kind+" artifact device", kind, typeKey).Scan(&deviceID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, `insert into device_external_identities(
			project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at
		) values($1,$2,$3,$4,$5,$6,jsonb_build_object('attributes',jsonb_build_object('serialNumber',$7::text)),'managed',now())`,
			projectID, teamID, adapterID, deviceID, secureRemoteKey(serial), typeKey, serial); err != nil {
			t.Fatal(err)
		}
		return deviceID
	}
	dockSN := fmt.Sprintf("DOCK_ARTIFACT_SECRET_%d", suffix)
	droneSN := fmt.Sprintf("DRONE_ARTIFACT_SECRET_%d", suffix)
	_ = createDevice("dji.dock2", "dock", dockSN)
	droneID := createDevice("dji.matrice3td", "aircraft", droneSN)

	clock := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ingestor := telemetry.NewIngestor(database)
	projector := NewSQLFlightCatalogProjector(database, ingestor, func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(ingestor, repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	waylineID := fmt.Sprintf("WAYLINE_ARTIFACT_SECRET_%d", suffix)
	taskUUID := fmt.Sprintf("TASK_ARTIFACT_SECRET_%d", suffix)
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "wayline", CompleteSnapshot: true, ReceivedAt: clock, Waylines: []WaylineSummary{{
		ID: waylineID, Name: "轨迹航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"}, UpdatedAt: clock.UnixMilli(), SizeBytes: 100,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "flight-task", CompleteSnapshot: true, ReceivedAt: clock, FlightTasks: []FlightTaskSummary{{
		UUID: taskUUID, Name: "轨迹任务", TaskType: "immediate", Status: "success", SN: dockSN, WaylineUUID: waylineID,
		BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), RunAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), CompletedAt: clock.Format(time.RFC3339Nano),
	}}}); err != nil {
		t.Fatal(err)
	}
	targets, err := projector.ListArtifactTargets(ctx, instance, 25)
	if err != nil || len(targets) != 1 || !targets[0].NeedTrack || !targets[0].NeedOperation || !targets[0].NeedMedia || !targets[0].MediaUploadFinal {
		t.Fatalf("artifact targets=%#v err=%v", targets, err)
	}
	target := targets[0]
	earlier := clock.Add(-2 * time.Minute)
	later := clock.Add(-time.Minute)
	trackID := fmt.Sprintf("TRACK_ARTIFACT_SECRET_%d", suffix)
	track := FlightTaskTrack{Track: FlightTrack{
		ID: trackID, DroneSN: droneSN, FlightDistance: 250, FlightDuration: 60,
		Points: []FlightTrackPoint{
			{Timestamp: later.UnixMilli(), Latitude: 30.2, Longitude: 120.2, Height: 22},
			{Timestamp: earlier.UnixMilli(), Latitude: 30.1, Longitude: 120.1, Height: 21},
			{Timestamp: earlier.UnixMilli(), Latitude: 30.1, Longitude: 120.1, Height: 21},
			{Timestamp: clock.UnixMilli(), Latitude: 30.3, Longitude: 999, Height: 23},
		},
	}}
	userSecret := fmt.Sprintf("USER_ARTIFACT_SECRET_%d", suffix)
	bidSecret := fmt.Sprintf("BID_ARTIFACT_SECRET_%d", suffix)
	timeline := FlightTaskOperationTimeline{
		ControlChanges: []FlightControlChange{{Time: earlier.UnixMilli(), UserName: userSecret, UserID: userSecret, ControlType: "wayline"}},
		PayloadChanges: []FlightControlChange{},
		OperationLogs: []FlightOperationLog{
			{Method: "pause_task", Time: later.UnixMilli(), Bid: bidSecret, UserName: userSecret, UserID: userSecret},
			{Method: "pause_task", Time: later.UnixMilli(), Bid: bidSecret, UserName: userSecret, UserID: userSecret},
		},
		RelatedUsers: []FlightOperationUser{{UserName: userSecret, UserID: userSecret, OperType: "operator"}},
	}
	emptyMedia := []FlightTaskMedia{}
	poll := FlightArtifactPoll{Target: target, Track: &track, Operations: &timeline, Media: &emptyMedia, ReceivedAt: clock}
	if err := projector.ApplyFlightArtifacts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyFlightArtifacts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	remaining, err := projector.ListArtifactTargets(ctx, instance, 25)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("completed artifact remained pending: %#v err=%v", remaining, err)
	}

	var observationCount, poseCount, operationCount, outboxCount int
	var minCaptured, maxCaptured time.Time
	var minHeight, maxHeight float64
	var allOriginal, allStandardNull bool
	if err := database.QueryRowContext(ctx, `select count(*),min(captured_at),max(captured_at),
		min(st_z(original_geometry)),max(st_z(original_geometry)),bool_and(original_geometry is not null),bool_and(standard_geometry is null)
		from observations where project_id=$1 and task_run_id=$2`, projectID, target.TaskRunID).Scan(&observationCount, &minCaptured, &maxCaptured, &minHeight, &maxHeight, &allOriginal, &allStandardNull); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from poses pose join observations observation on observation.id=pose.observation_id
		where observation.project_id=$1 and observation.task_run_id=$2 and pose.device_id=$3 and pose.standard_position is null and pose.original_position is not null`,
		projectID, target.TaskRunID, droneID).Scan(&poseCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != 2 || poseCount != 2 || !minCaptured.Equal(earlier) || !maxCaptured.Equal(later) || minHeight != 21 || maxHeight != 22 || !allOriginal || !allStandardNull {
		t.Fatalf("trajectory observations=%d poses=%d min=%s max=%s height=%f/%f original=%v standardNull=%v", observationCount, poseCount, minCaptured, maxCaptured, minHeight, maxHeight, allOriginal, allStandardNull)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1 and event_type='task_run.vendor_operation'`, projectID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 2 {
		t.Fatalf("duplicate operation timeline created %d events", operationCount)
	}
	var validPoints, invalidPoints int
	if err := database.QueryRowContext(ctx, `select (payload_json->>'validPointCount')::int,(payload_json->>'invalidPointCount')::int
		from project_events where project_id=$1 and event_id=$2`, projectID, flightArtifactMarkerID(projectID, target.TaskRunID, "track")).Scan(&validPoints, &invalidPoints); err != nil {
		t.Fatal(err)
	}
	if validPoints != 2 || invalidPoints != 1 {
		t.Fatalf("track marker valid=%d invalid=%d", validPoints, invalidPoints)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from outbox_events where project_id=$1 and event_type like 'task_run.%'`, projectID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("artifact read projection emitted %d mission events", outboxCount)
	}
	var persisted string
	if err := database.QueryRowContext(ctx, `select concat_ws(' ',
		coalesce((select string_agg(event_id||' '||payload_json::text,' ') from project_events where project_id=$1),''),
		coalesce((select string_agg(properties_json::text||' '||quality_json::text,' ') from observations where project_id=$1),''))`, projectID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{taskUUID, waylineID, trackID, dockSN, droneSN, userSecret, bidSecret} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("trajectory or timeline leaked remote identity %q", secret)
		}
	}
}
