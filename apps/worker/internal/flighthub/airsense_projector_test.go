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

func TestSQLAirSenseProjectorKeepsLifecycleTaskAndMapProjectionIdempotent(t *testing.T) {
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
	var teamID, projectID, deviceID, taskID, taskRunID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-airsense-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-airsense-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-airsense-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	deviceSN := fmt.Sprintf("DOCK_AIRSENSE_SECRET_%d", suffix)
	icao := fmt.Sprintf("ICAO_AIRSENSE_SECRET_%d", suffix)
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
		select $1,$2,id,'AirSense dock','dock','online' from device_types where type_key='dji.dock2' and status='active'
		order by version desc limit 1 returning id`, projectID, adapterID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into device_external_identities(
		project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at
	) values($1,$2,$3,$4,$5,'dji.dock2',jsonb_build_object('attributes',jsonb_build_object('serialNumber',$6::text)),'managed',now())`,
		projectID, teamID, adapterID, deviceID, secureRemoteKey(deviceSN), deviceSN); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into tasks(project_id,team_id,name,trigger_type,script)
		values($1,$2,'AirSense active task','manual','{}') returning id`, projectID, teamID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into task_runs(project_id,team_id,task_id,selected_device_id,trigger_source,status)
		values($1,$2,$3,$4,'connector','running') returning id`, projectID, teamID, taskID, deviceID).Scan(&taskRunID); err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	projector := NewSQLFlightCatalogProjector(database, telemetry.NewIngestor(database), func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(telemetry.NewIngestor(database), repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	device := connector.ManagedConnectorDevice{DeviceID: deviceID, TeamID: teamID, Serial: deviceSN}
	warning := airSenseWarning(clock, deviceSN, icao)
	apply := func(warnings []DeviceAirSenseWarnings, complete bool, receivedAt time.Time) {
		t.Helper()
		if err := sink.ApplyGeospatialCatalog(ctx, instance, GeospatialCatalogPoll{
			MapElements: []MapElementSnapshot{}, FlightAreas: []FlightArea{}, Devices: []connector.ManagedConnectorDevice{device},
			AirSenseWarnings: warnings, MapElementsComplete: false, FlightAreasComplete: false,
			AirSenseComplete: complete, ReceivedAt: receivedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply([]DeviceAirSenseWarnings{warning}, true, clock)
	apply([]DeviceAirSenseWarnings{warning}, true, clock)

	var eventID, eventStatus, eventSeverity, eventTitle, groupStatus, groupLabel, issueStatus, issuePriority, remoteID, summary string
	var eventOccurrence, eventVersion, issueOccurrence, issueVersion, linkedRunID, eventCount, issueCount, groupCount, triggerCount int
	var longitude, latitude float64
	load := func() {
		t.Helper()
		if err := database.QueryRowContext(ctx, `select resource.remote_id,resource.summary_json::text,event.id,event.status,event.severity,event.title,
			event.occurrence_count,event.state_version,group_row.status,group_row.label,
			st_x(st_centroid(group_row.geographic_geometry)),st_y(st_centroid(group_row.geographic_geometry)),
			issue.status,issue.priority,issue.occurrence_count,issue.state_version,issue.task_run_id
			from connector_remote_resources resource
			join perception_events event on event.project_id=resource.project_id and resource.canonical_target_type='perception_event' and resource.canonical_target_id=event.id::text
			join detection_groups group_row on group_row.project_id=event.project_id and group_row.id=event.detection_group_id
			join issue_links link on link.project_id=event.project_id and link.link_type='perception_event' and link.target_id=event.id::text
			join issues issue on issue.project_id=link.project_id and issue.id=link.issue_id
			where resource.project_id=$1 and resource.connector_instance_id=$2 and resource.resource_kind='air-sense-warning'`, projectID, adapterID).
			Scan(&remoteID, &summary, &eventID, &eventStatus, &eventSeverity, &eventTitle, &eventOccurrence, &eventVersion,
				&groupStatus, &groupLabel, &longitude, &latitude, &issueStatus, &issuePriority, &issueOccurrence, &issueVersion, &linkedRunID); err != nil {
			t.Fatal(err)
		}
	}
	load()
	if err := database.QueryRowContext(ctx, `select
		(select count(*) from perception_events where project_id=$1),
		(select count(*) from issues where project_id=$1 and source_type='dji-flighthub-airsense'),
		(select count(*) from detection_groups where project_id=$1 and aggregation_version='dji-flighthub-airsense/unverified-v1'),
		(select count(*) from project_events where project_id=$1 and event_type='perception_event.created')`, projectID).
		Scan(&eventCount, &issueCount, &groupCount, &triggerCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || issueCount != 1 || groupCount != 1 || triggerCount != 1 || eventStatus != "open" || eventSeverity != "high" ||
		eventTitle != "司空 AirSense 空域告警" || eventOccurrence != 1 || eventVersion != 0 || groupStatus != "active" || groupLabel != "AirSense 空域目标" ||
		issueStatus != "open" || issuePriority != "high" || issueOccurrence != 1 || issueVersion != 0 || linkedRunID != taskRunID ||
		longitude < 120.49 || longitude > 120.51 || latitude < 30.24 || latitude > 30.26 {
		t.Fatalf("initial AirSense event=%d/%s/%s/%d/%d group=%d/%s/%s/%f,%f issue=%d/%s/%s/%d/%d run=%d/%d triggers=%d",
			eventCount, eventStatus, eventSeverity, eventOccurrence, eventVersion, groupCount, groupStatus, groupLabel, longitude, latitude,
			issueCount, issueStatus, issuePriority, issueOccurrence, issueVersion, linkedRunID, taskRunID, triggerCount)
	}
	for _, secret := range []string{deviceSN, icao} {
		if strings.Contains(remoteID, secret) || strings.Contains(summary, secret) {
			t.Fatalf("AirSense projection leaked %q: remote=%s summary=%s", secret, remoteID, summary)
		}
	}

	warning.CapturedAt = clock.Add(time.Minute)
	warning.Timestamp = warning.CapturedAt.UnixMilli()
	warning.ExpiresAt = clock.Add(6 * time.Minute)
	warning.Events[0].WarningLevel = 3
	warning.Events[0].Latitude = 30.75
	warning.Events[0].Longitude = 120.875
	apply([]DeviceAirSenseWarnings{warning}, true, clock.Add(time.Minute))
	load()
	if eventStatus != "open" || eventSeverity != "critical" || issuePriority != "critical" || eventOccurrence != 1 || issueOccurrence != 1 ||
		eventVersion != 0 || issueVersion != 0 || longitude < 120.87 || longitude > 120.88 || latitude < 30.74 || latitude > 30.76 {
		t.Fatalf("updated AirSense status=%s severity=%s/%s occurrences=%d/%d versions=%d/%d location=%f,%f",
			eventStatus, eventSeverity, issuePriority, eventOccurrence, issueOccurrence, eventVersion, issueVersion, longitude, latitude)
	}

	warning.Expired = true
	apply([]DeviceAirSenseWarnings{warning}, true, clock.Add(2*time.Minute))
	load()
	if eventStatus != "resolved" || groupStatus != "superseded" || issueStatus != "closed" || eventVersion != 1 || issueVersion != 1 {
		t.Fatalf("expired AirSense event=%s/%d group=%s issue=%s/%d", eventStatus, eventVersion, groupStatus, issueStatus, issueVersion)
	}

	warning.Expired = false
	warning.CapturedAt = clock.Add(3 * time.Minute)
	warning.Timestamp = warning.CapturedAt.UnixMilli()
	warning.ExpiresAt = clock.Add(8 * time.Minute)
	apply([]DeviceAirSenseWarnings{warning}, true, clock.Add(3*time.Minute))
	load()
	if eventStatus != "open" || groupStatus != "active" || issueStatus != "open" || eventOccurrence != 2 || issueOccurrence != 2 {
		t.Fatalf("reopened AirSense event=%s/%d group=%s issue=%s/%d", eventStatus, eventOccurrence, groupStatus, issueStatus, issueOccurrence)
	}

	apply([]DeviceAirSenseWarnings{}, true, clock.Add(4*time.Minute))
	load()
	if eventStatus != "resolved" || groupStatus != "superseded" || issueStatus != "closed" || eventVersion != 3 || issueVersion != 3 {
		t.Fatalf("missing AirSense event=%s/%d group=%s issue=%s/%d", eventStatus, eventVersion, groupStatus, issueStatus, issueVersion)
	}
}
