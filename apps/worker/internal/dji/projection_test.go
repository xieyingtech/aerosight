package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"aerosight/worker/internal/outbox"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestEnvironmentalSensorPayloadUsesGenericSamples(t *testing.T) {
	payload, exists := environmentalSensorPayload(json.RawMessage(`{
		"environment_temperature":24.5,"humidity":58,"wind_speed":3.2,"flighttask_step_code":7
	}`))
	if !exists {
		t.Fatal("expected environmental sensor sample")
	}
	var decoded struct {
		Samples map[string]struct {
			Value float64 `json:"value"`
			Unit  string  `json:"unit"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Samples) != 3 || decoded.Samples["humidity"].Unit != "%RH" {
		t.Fatalf("unexpected generic sensor payload: %s", payload)
	}
	if _, leaked := decoded.Samples["flighttask_step_code"]; leaked {
		t.Fatalf("non-sensor field leaked into sensor payload: %s", payload)
	}
}

func TestProjectorClaimsDJITopologyIntoUnifiedDeviceQuery(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var teamID, projectID int
	if err := database.QueryRowContext(ctx, "insert into teams(name) values ('DJI projection integration') returning id").Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, "delete from teams where id=$1", teamID)
	if err := database.QueryRowContext(ctx, "insert into projects(team_id,name) values ($1,'DJI unified devices') returning id", teamID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var adapterID int64
	if err := database.QueryRowContext(ctx, `
		insert into device_adapters(project_id,team_id,name,adapter_type,vendor,status)
		values ($1,$2,'DJI Cloud integration','dji-cloud','dji','connected') returning id`,
		projectID, teamID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}

	fixture, err := os.ReadFile("../../testdata/dji/dock2-m3td-topology.json")
	if err != nil {
		t.Fatal(err)
	}
	topology, err := RouteMQTTMessage(RouteContext{
		ProjectID: projectID, AdapterID: adapterID, AllowedGatewaySNs: map[string]bool{"DOCK2-DEMO-001": true},
	}, MQTTMessage{Topic: "sys/product/DOCK2-DEMO-001/status", Payload: fixture, QoS: 1, ReceivedAt: time.UnixMilli(1787821200100).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	projector := NewProjector()
	projectEvent(t, ctx, database, projector, teamID, topology)
	projectEvent(t, ctx, database, projector, teamID, topology)
	dock3Fixture, err := os.ReadFile("../../testdata/dji/dock3-m4td-topology.json")
	if err != nil {
		t.Fatal(err)
	}
	dock3Topology, err := RouteMQTTMessage(RouteContext{
		ProjectID: projectID, AdapterID: adapterID, AllowedGatewaySNs: map[string]bool{"DOCK3-DEMO-001": true},
	}, MQTTMessage{Topic: "sys/product/DOCK3-DEMO-001/status", Payload: dock3Fixture, QoS: 1, ReceivedAt: time.UnixMilli(1787821201100).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	projectEvent(t, ctx, database, projector, teamID, dock3Topology)

	rows, err := database.QueryContext(ctx, `
		select device_type.category, device_type.type_key, driver.driver_key
		from devices device
		join device_types device_type on device_type.id=device.device_type_id
		join driver_definitions driver on driver.id=device_type.driver_definition_id
		where device.project_id=$1 order by device_type.category,device_type.type_key`, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	categories := map[string]int{}
	count := 0
	for rows.Next() {
		var category, typeKey, driverKey string
		if err := rows.Scan(&category, &typeKey, &driverKey); err != nil {
			t.Fatal(err)
		}
		if driverKey != DriverKey || typeKey == "legacy.device" {
			t.Fatalf("page query returned a non-DJI typed device: %s %s", driverKey, typeKey)
		}
		categories[category]++
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 12 || categories["dock"] != 2 || categories["aircraft"] != 2 || categories["camera"] != 6 || categories["sensor"] != 2 {
		t.Fatalf("unified page query did not expose full topology: count=%d categories=%v", count, categories)
	}
	var relationships, channels int
	if err := database.QueryRowContext(ctx, "select count(*) from device_relationships where project_id=$1", projectID).Scan(&relationships); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "select count(*) from device_stream_channels where project_id=$1", projectID).Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if relationships != 10 || channels < 10 {
		t.Fatalf("topology was not materialized with relationships/streams: relationships=%d channels=%d", relationships, channels)
	}

	osd := []byte(`{"tid":"fixture-dock-osd","timestamp":1787821202000,"gateway":"DOCK2-DEMO-001","data":{"seq":2,"environment_temperature":24.5,"temperature":31.2,"humidity":58,"wind_speed":3.2,"rainfall":0}}`)
	realtime, err := RouteMQTTMessage(RouteContext{
		ProjectID: projectID, AdapterID: adapterID, AllowedGatewaySNs: map[string]bool{"DOCK2-DEMO-001": true},
	}, MQTTMessage{Topic: "thing/product/DOCK2-DEMO-001/osd", Payload: osd, QoS: 1, ReceivedAt: time.UnixMilli(1787821202050).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	projectEvent(t, ctx, database, projector, teamID, realtime)
	var telemetryType, sensorStatus string
	var sensorPayload json.RawMessage
	if err := database.QueryRowContext(ctx, `
		select latest.telemetry_type,latest.payload_json,device.status
		from device_latest_telemetry latest
		join devices device on device.id=latest.device_id
		join device_external_identities identity on identity.device_id=device.id and identity.adapter_id=$1
		where identity.external_device_id='DOCK2-DEMO-001:environment'`, adapterID,
	).Scan(&telemetryType, &sensorPayload, &sensorStatus); err != nil {
		t.Fatal(err)
	}
	if telemetryType != "dji.environment" || sensorStatus != "online" || !json.Valid(sensorPayload) {
		t.Fatalf("sensor telemetry was not written to unified realtime tables: type=%s status=%s payload=%s", telemetryType, sensorStatus, sensorPayload)
	}
}

func projectEvent(t *testing.T, ctx context.Context, database *sql.DB, projector *Projector, teamID int, message RoutedMessage) {
	t.Helper()
	payload, err := json.Marshal(message.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := projector.Handler(ctx, tx, outbox.Event{
		ProjectID: message.Envelope.ProjectID, TeamID: teamID, EventID: message.Envelope.EventID,
		EventType: message.Envelope.EventType, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
