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
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSQLDeviceHealthProjectorKeepsHMSAndTopologyLifecycleIdempotent(t *testing.T) {
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
	var teamID, projectID, dockID, aircraftID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-health-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-health-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-health-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	createDevice := func(typeKey, name, kind string) int {
		t.Helper()
		var id int
		if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
			select $1,$2,id,$3,$4,'unknown' from device_types where type_key=$5 and status='active'
			order by version desc limit 1 returning id`, projectID, adapterID, name, kind, typeKey).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	dockID = createDevice("dji.dock2", "health dock", "dock")
	aircraftID = createDevice("dji.matrice3td", "health aircraft", "aircraft")
	if _, err := database.ExecContext(ctx, `insert into device_relationships(
		project_id,team_id,from_device_id,to_device_id,relation_type,source_type,valid_from
	) values($1,$2,$3,$4,'mounted','manual',$5)`, projectID, teamID, dockID, aircraftID, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	devices := []connector.ManagedConnectorDevice{
		{DeviceID: dockID, TeamID: teamID, Serial: "DOCK_REDACTED"},
		{DeviceID: aircraftID, TeamID: teamID, Serial: "AIRCRAFT_REDACTED"},
	}
	alert := HMSAlert{
		ID: "HMS_REDACTED", StatusKey: "STATUS_REDACTED", Code: "CODE_REDACTED", Message: "raw vendor message must not persist",
		Level: "warning", Module: "device_management", DomainType: "drone", Status: 1,
		BeginTime: base.UnixMilli(), DeviceDataUpdateTime: base.UnixMilli(),
	}
	deviceHMS := DeviceHMS{SN: "AIRCRAFT_REDACTED"}
	deviceHMS.Alerts.List = []HMSAlert{alert}
	topology := HistoricalTopology{Index: "TOPOLOGY_REDACTED", Host: &HistoricalDevice{SN: "AIRCRAFT_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}}, Parents: []HistoricalDevice{{SN: "DOCK_REDACTED", Model: DeviceModel{Key: "3-2-0", Class: "airport"}}}}
	poll := HealthPoll{Devices: devices, HMS: []DeviceHMS{deviceHMS}, Topologies: []HistoricalTopology{topology}, ReceivedAt: base.Add(time.Second)}
	projector := NewSQLDeviceHealthProjector(database)
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	if err := projector.Apply(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	if err := projector.Apply(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	var issueCount, occurrenceCount, activeTopologyCount int
	var status, title, description, labels string
	if err := database.QueryRowContext(ctx, `select count(*),max(occurrence_count),max(status),max(title),max(description),max(labels_json::text)
		from issues where project_id=$1 and source_type='dji-flighthub-hms'`, projectID).Scan(&issueCount, &occurrenceCount, &status, &title, &description, &labels); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from device_relationships where project_id=$1
		and relation_type=$2 and source_type='driver' and valid_until is null`, projectID, historicalTopologyRelation).Scan(&activeTopologyCount); err != nil {
		t.Fatal(err)
	}
	if issueCount != 1 || occurrenceCount != 1 || status != "open" || activeTopologyCount != 1 {
		t.Fatalf("idempotent health projection issue=%d occurrences=%d status=%s topology=%d", issueCount, occurrenceCount, status, activeTopologyCount)
	}
	serialized := strings.Join([]string{title, description, labels}, " ")
	if strings.Contains(serialized, "AIRCRAFT_REDACTED") || strings.Contains(serialized, alert.Message) || strings.Contains(serialized, alert.ID) {
		t.Fatalf("health projection persisted sensitive vendor values: %s", serialized)
	}

	recoveredAt := base.Add(time.Minute)
	alert.Status = 2
	alert.EndTime = recoveredAt.UnixMilli()
	alert.DeviceDataUpdateTime = recoveredAt.UnixMilli()
	deviceHMS.Alerts.List = []HMSAlert{alert}
	poll.HMS = []DeviceHMS{deviceHMS}
	poll.Topologies = []HistoricalTopology{}
	poll.ReceivedAt = recoveredAt.Add(time.Second)
	if err := projector.Apply(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	var closedAt, topologyValidUntil time.Time
	if err := database.QueryRowContext(ctx, `select status,closed_at from issues where project_id=$1 and source_type='dji-flighthub-hms'`, projectID).Scan(&status, &closedAt); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select valid_until from device_relationships where project_id=$1 and relation_type=$2`, projectID, historicalTopologyRelation).Scan(&topologyValidUntil); err != nil {
		t.Fatal(err)
	}
	var manualActive bool
	if err := database.QueryRowContext(ctx, `select exists(select 1 from device_relationships where project_id=$1 and relation_type='mounted' and valid_until is null)`, projectID).Scan(&manualActive); err != nil {
		t.Fatal(err)
	}
	if status != "closed" || !closedAt.Equal(recoveredAt) || !topologyValidUntil.Equal(poll.ReceivedAt) || !manualActive {
		t.Fatalf("health lifecycle status=%s closed=%s topologyUntil=%s manualActive=%v", status, closedAt, topologyValidUntil, manualActive)
	}
}
