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

func TestSQLFlightAlertProjectorKeepsLifecycleLinksAndSecretsIdempotent(t *testing.T) {
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
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-alert-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-alert-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-alert-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	createDevice := func(typeKey, kind, serial string) int {
		t.Helper()
		var deviceID int
		if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
			select $1,$2,id,$3,$4,'unknown' from device_types where type_key=$5 and status='active'
			order by version desc limit 1 returning id`, projectID, adapterID, kind+" alert device", kind, typeKey).Scan(&deviceID); err != nil {
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
	dockSN := fmt.Sprintf("DOCK_ALERT_SECRET_%d", suffix)
	droneSN := fmt.Sprintf("DRONE_ALERT_SECRET_%d", suffix)
	dockID := createDevice("dji.dock2", "dock", dockSN)
	droneID := createDevice("dji.matrice3td", "aircraft", droneSN)

	clock := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	projectUUID := "11111111-1111-4111-8111-111111111111"
	instance := connector.Instance{ID: adapterID, ProjectID: projectID, DiscoveryScope: []byte(fmt.Sprintf(`{"projectUuid":%q,"projectName":"test"}`, projectUUID))}
	projector := NewSQLFlightCatalogProjector(database, telemetry.NewIngestor(database), func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(telemetry.NewIngestor(database), repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	waylineID := fmt.Sprintf("WAYLINE_ALERT_SECRET_%d", suffix)
	flightID := fmt.Sprintf("FLIGHT_ALERT_SECRET_%d", suffix)
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "wayline", CompleteSnapshot: true, ReceivedAt: clock, Waylines: []WaylineSummary{{
		ID: waylineID, Name: "告警航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"}, UpdatedAt: clock.UnixMilli(), SizeBytes: 100,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "flight-task", CompleteSnapshot: true, ReceivedAt: clock, FlightTasks: []FlightTaskSummary{{
		UUID: flightID, Name: "告警任务", TaskType: "immediate", Status: "success", SN: dockSN, WaylineUUID: waylineID,
		BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), RunAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), CompletedAt: clock.Format(time.RFC3339Nano),
	}}}); err != nil {
		t.Fatal(err)
	}
	var taskRunID int
	if err := database.QueryRowContext(ctx, `select canonical_target_id::int from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-task' and remote_id=$3`, projectID, adapterID, flightID).Scan(&taskRunID); err != nil {
		t.Fatal(err)
	}

	alertUUID := fmt.Sprintf("AI_ALERT_SECRET_%d", suffix)
	thumbnailSecret := fmt.Sprintf("https://objects.vendor.example/alerts/%s.jpg?signature=THUMBNAIL_SECRET_%d", alertUUID, suffix)
	latitude, longitude, altitude := 30.000003, 120.000003, 42.5
	alert := AIAlertRecord{
		AlertUUID: alertUUID, FlightID: flightID, ProjectID: projectUUID, DroneSN: droneSN, GatewaySN: dockSN,
		Status: 2, Reason: "目标检测", AlgorithmSource: 1,
		Location: &AIAlertLocation{Latitude: &latitude, Longitude: &longitude, Altitude: &altitude}, FileID: 1001, MediaIndex: 7,
		TaskName: "告警任务", TriggerActions: []AIAlertTriggerAction{{Action: 0}},
		Targets: []AIAlertTarget{{TargetType: 0, Confidence: 0.95, Label: "person"}}, Timestamp: clock.Add(-time.Minute).UnixMilli(),
		ThumbnailURL: thumbnailSecret, Labels: []string{"person"}, IntervalSeconds: 30,
	}
	poll := FlightAlertPoll{
		Aggregates: []FlightAlertSummary{{FlightID: flightID, Count: 1, TaskName: "告警任务", TaskType: 1, StartTime: clock.Add(-time.Hour).Unix(), Status: 1}},
		Alerts:     []AIAlertRecord{alert}, CompleteSnapshot: true, ReceivedAt: clock,
	}
	if err := projector.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}

	var eventID, eventStatus, issueStatus, locationQuality, aggregationVersion string
	var eventCount, issueCount, groupCount, assetCount, issueEventCount, triggerCount, eventOccurrence, eventVersion int
	var issueID, linkedRunID, linkedDeviceID, linkedAssetIssueID int
	if err := database.QueryRowContext(ctx, `select event.id,event.status,event.occurrence_count,event.state_version,event.detection_group_id,
		group_row.location_quality,group_row.aggregation_version
		from connector_remote_resources resource join perception_events event
		 on event.project_id=resource.project_id and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
		join detection_groups group_row on group_row.project_id=event.project_id and group_row.id=event.detection_group_id
		where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='ai-alert' and resource.remote_id=$3`,
		projectID, adapterID, alertUUID).Scan(&eventID, &eventStatus, &eventOccurrence, &eventVersion, &groupCount, &locationQuality, &aggregationVersion); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select issue.id,issue.status,issue.task_run_id from issues issue join issue_links link
		on link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='perception_event' and link.target_id=$2
		where issue.project_id=$1`, projectID, eventID).Scan(&issueID, &issueStatus, &linkedRunID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from perception_events where project_id=$1`, projectID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from issues where project_id=$1 and source_type='dji-flighthub-ai'`, projectID).Scan(&issueCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*),max(device_id),max(issue_id) from assets where project_id=$1 and metadata_json->>'sourceKind'='ai-alert-media'`, projectID).Scan(&assetCount, &linkedDeviceID, &linkedAssetIssueID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from issue_events where project_id=$1 and issue_id=$2`, projectID, issueID).Scan(&issueEventCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1 and event_type='perception_event.created'`, projectID).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || issueCount != 1 || assetCount != 1 || eventStatus != "open" || issueStatus != "open" || eventOccurrence != 1 || eventVersion != 0 ||
		linkedRunID != taskRunID || linkedDeviceID != droneID || linkedAssetIssueID != issueID || locationQuality != "low" || aggregationVersion != "dji-flighthub-ai-alert/unverified-v1" ||
		issueEventCount != 1 || triggerCount != 1 {
		t.Fatalf("projection event=%d/%s/%d/%d issue=%d/%s asset=%d links=%d/%d run=%d/%d location=%s/%s events=%d triggers=%d",
			eventCount, eventStatus, eventOccurrence, eventVersion, issueCount, issueStatus, assetCount, linkedDeviceID, linkedAssetIssueID,
			linkedRunID, taskRunID, locationQuality, aggregationVersion, issueEventCount, triggerCount)
	}
	var deviceLinks, assetLinks, spatialLinks int
	if err := database.QueryRowContext(ctx, `select count(*) filter(where link_type='device'),count(*) filter(where link_type='asset'),count(*) filter(where link_type='spatial_group')
		from issue_links where project_id=$1 and issue_id=$2`, projectID, issueID).Scan(&deviceLinks, &assetLinks, &spatialLinks); err != nil {
		t.Fatal(err)
	}
	if deviceLinks != 2 || assetLinks != 1 || spatialLinks != 1 || dockID == droneID {
		t.Fatalf("issue links device=%d asset=%d spatial=%d", deviceLinks, assetLinks, spatialLinks)
	}

	invalidLongitude := 999.0
	alert.Status = 3
	alert.Reason = "状态已更新 " + thumbnailSecret
	alert.Location.Longitude = &invalidLongitude
	alert.Targets[0].Confidence = 0.4
	poll.Alerts[0] = alert
	poll.ReceivedAt = clock.Add(time.Minute)
	if err := projector.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select event.occurrence_count,event.state_version,count(issue_event.id),
		(select count(*) from project_events where project_id=$1 and event_type='perception_event.created')
		from perception_events event join issue_links link on link.project_id=event.project_id and link.link_type='perception_event' and link.target_id=event.id::text
		join issue_events issue_event on issue_event.project_id=link.project_id and issue_event.issue_id=link.issue_id
		where event.project_id=$1 and event.id=$2 group by event.occurrence_count,event.state_version`, projectID, eventID).Scan(&eventOccurrence, &eventVersion, &issueEventCount, &triggerCount); err != nil {
		t.Fatal(err)
	}
	if eventOccurrence != 1 || eventVersion != 0 || issueEventCount != 1 || triggerCount != 1 {
		t.Fatalf("alert update retriggered event occurrence=%d version=%d issueEvents=%d triggers=%d", eventOccurrence, eventVersion, issueEventCount, triggerCount)
	}

	poll.Alerts = []AIAlertRecord{}
	poll.ReceivedAt = clock.Add(2 * time.Minute)
	if err := projector.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := projector.ApplyFlightAlerts(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select event.status,event.state_version,issue.status,count(issue_event.id)
		from perception_events event join issue_links link on link.project_id=event.project_id and link.link_type='perception_event' and link.target_id=event.id::text
		join issues issue on issue.project_id=link.project_id and issue.id=link.issue_id
		join issue_events issue_event on issue_event.project_id=issue.project_id and issue_event.issue_id=issue.id
		where event.project_id=$1 and event.id=$2 group by event.status,event.state_version,issue.status`, projectID, eventID).Scan(&eventStatus, &eventVersion, &issueStatus, &issueEventCount); err != nil {
		t.Fatal(err)
	}
	if eventStatus != "resolved" || issueStatus != "closed" || eventVersion != 1 || issueEventCount != 2 {
		t.Fatalf("recovery status event=%s/%d issue=%s events=%d", eventStatus, eventVersion, issueStatus, issueEventCount)
	}

	var persisted string
	if err := database.QueryRowContext(ctx, `select concat_ws(' ',
		coalesce((select string_agg(summary_json::text,' ') from connector_remote_resources where project_id=$1 and resource_kind in('flight-alert','ai-alert')),''),
		coalesce((select string_agg(title||' '||coalesce(description,'')||' '||labels_json::text,' ') from issues where project_id=$1),''),
		coalesce((select string_agg(title||' '||deduplication_key,' ') from perception_events where project_id=$1),''),
		coalesce((select string_agg(payload_json::text,' ') from project_events where project_id=$1),''),
		coalesce((select string_agg(metadata_json::text,' ') from assets where project_id=$1),''))`, projectID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{alertUUID, flightID, waylineID, projectUUID, dockSN, droneSN, thumbnailSecret} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("canonical alert projection leaked remote identity or URL %q", secret)
		}
	}

	crossScope := poll
	crossScope.Alerts = []AIAlertRecord{alert}
	crossScope.Alerts[0].ProjectID = "OTHER_PROJECT_REDACTED"
	if err := projector.ApplyFlightAlerts(ctx, instance, crossScope); err == nil || !strings.Contains(err.Error(), "project scope") {
		t.Fatalf("cross-project AI alert was accepted: %v", err)
	}
}
