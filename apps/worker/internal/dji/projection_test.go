package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	testCommandDispatchAndReplies(t, ctx, database, teamID, projectID, adapterID)
}

type capturedCommandPublisher struct {
	adapterID int64
	topic     string
	payload   json.RawMessage
	err       error
}

func (publisher *capturedCommandPublisher) Publish(_ context.Context, adapterID int64, topic string, payload []byte) error {
	publisher.adapterID = adapterID
	publisher.topic = topic
	publisher.payload = append(json.RawMessage(nil), payload...)
	return publisher.err
}

func testCommandDispatchAndReplies(t *testing.T, ctx context.Context, database *sql.DB, teamID, projectID int, adapterID int64) {
	t.Helper()
	publisher := &capturedCommandPublisher{}
	clock := time.UnixMilli(1787821300000).UTC()
	dispatcher, err := NewCommandDispatcher(publisher, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	var aircraftID int
	if err := database.QueryRowContext(ctx, `select device_id from device_external_identities
		where adapter_id=$1 and external_device_id='M3TD-DEMO-001'`, adapterID).Scan(&aircraftID); err != nil {
		t.Fatal(err)
	}
	commandID := "89f050f8-77f2-4a73-a8b7-8391e3797801"
	if _, err := database.ExecContext(ctx, `
		insert into device_commands(
		  id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,
		  parameters_json,safety_context_json,status,priority,deadline_at
		) values ($1,$2,$3,$4,'return_home','integration-rth','flight.return_home','{}','{}','dispatchable',100,$5)`,
		commandID, projectID, teamID, aircraftID, time.UnixMilli(1787821400000).UTC()); err != nil {
		t.Fatal(err)
	}
	callOutboxHandler(t, ctx, database, dispatcher.DispatchHandler, outbox.Event{
		ProjectID: projectID, TeamID: teamID, EventID: "dispatch:" + commandID,
		EventType: "device.command.dispatch", Payload: jsonObject(map[string]any{"commandId": commandID}),
	})
	if publisher.adapterID != adapterID || publisher.topic != "thing/product/DOCK2-DEMO-001/services" {
		t.Fatalf("aircraft command did not route through its dock gateway: adapter=%d topic=%s", publisher.adapterID, publisher.topic)
	}
	var published struct {
		TransactionID string `json:"tid"`
		BusinessID    string `json:"bid"`
		Method        string `json:"method"`
	}
	if json.Unmarshal(publisher.payload, &published) != nil || published.TransactionID != commandID || published.BusinessID != commandID || published.Method != "return_home" {
		t.Fatalf("published correlation was not stable: %s", publisher.payload)
	}

	unknown := routedReply(t, projectID, adapterID, "00000000-0000-4000-8000-000000000000", 0)
	callOutboxHandler(t, ctx, database, dispatcher.ReplyHandler, replyOutboxEvent(t, teamID, unknown))
	var status string
	if err := database.QueryRowContext(ctx, "select status from device_commands where id=$1", commandID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("unknown reply mutated command: status=%s err=%v", status, err)
	}

	ack := routedReply(t, projectID, adapterID, commandID, 0)
	callOutboxHandler(t, ctx, database, dispatcher.ReplyHandler, replyOutboxEvent(t, teamID, ack))
	var correlationStatus string
	if err := database.QueryRowContext(ctx, `select command.status,correlation.status
		from device_commands command join device_command_protocol_correlations correlation on correlation.command_id=command.id
		where command.id=$1`, commandID).Scan(&status, &correlationStatus); err != nil {
		t.Fatal(err)
	}
	if status != "acknowledged" || correlationStatus != "acknowledged" {
		t.Fatalf("ACK did not update command and correlation: command=%s correlation=%s", status, correlationStatus)
	}

	nackID := "89f050f8-77f2-4a73-a8b7-8391e3797802"
	if _, err := database.ExecContext(ctx, `
		insert into device_commands(
		  id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,
		  parameters_json,safety_context_json,status,priority,deadline_at
		) values ($1,$2,$3,$4,'return_home','integration-rth-nack','flight.return_home','{}','{}','dispatchable',100,$5)`,
		nackID, projectID, teamID, aircraftID, time.UnixMilli(1787821400000).UTC()); err != nil {
		t.Fatal(err)
	}
	callOutboxHandler(t, ctx, database, dispatcher.DispatchHandler, outbox.Event{
		ProjectID: projectID, TeamID: teamID, EventID: "dispatch:" + nackID,
		EventType: "device.command.dispatch", Payload: jsonObject(map[string]any{"commandId": nackID}),
	})
	nack := routedReply(t, projectID, adapterID, nackID, 326108)
	callOutboxHandler(t, ctx, database, dispatcher.ReplyHandler, replyOutboxEvent(t, teamID, nack))
	if err := database.QueryRowContext(ctx, "select status from device_commands where id=$1", nackID).Scan(&status); err != nil || status != "nacked" {
		t.Fatalf("NACK did not update command: status=%s err=%v", status, err)
	}

	timeoutID := "89f050f8-77f2-4a73-a8b7-8391e3797803"
	if _, err := database.ExecContext(ctx, `
		insert into device_commands(
		  id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,
		  parameters_json,safety_context_json,status,priority,deadline_at
		) values ($1,$2,$3,$4,'flight.return_home','integration-rth-timeout','flight.return_home','{}','{}','dispatchable',90,$5)`,
		timeoutID, projectID, teamID, aircraftID, clock.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	callOutboxHandler(t, ctx, database, dispatcher.DispatchHandler, outbox.Event{
		ProjectID: projectID, TeamID: teamID, EventID: "dispatch:" + timeoutID,
		EventType: "device.command.dispatch", Payload: jsonObject(map[string]any{"commandId": timeoutID}),
	})
	clock = clock.Add(2 * time.Second)
	if expired, err := dispatcher.ExpireUnknown(ctx, database); err != nil || expired != 1 {
		t.Fatalf("sent return-home timeout was not reconciled as unknown: expired=%d err=%v", expired, err)
	}
	late := routedReply(t, projectID, adapterID, timeoutID, 0)
	callOutboxHandler(t, ctx, database, dispatcher.ReplyHandler, replyOutboxEvent(t, teamID, late))
	if err := database.QueryRowContext(ctx, `select command.status,correlation.status
		from device_commands command join device_command_protocol_correlations correlation on correlation.command_id=command.id
		where command.id=$1`, timeoutID).Scan(&status, &correlationStatus); err != nil {
		t.Fatal(err)
	}
	if status != "unknown" || correlationStatus != "unknown" {
		t.Fatalf("late ACK converted physically unknown return-home into success: command=%s correlation=%s", status, correlationStatus)
	}

	disconnectedID := "89f050f8-77f2-4a73-a8b7-8391e3797804"
	if _, err := database.ExecContext(ctx, `
		insert into device_commands(
		  id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,
		  parameters_json,safety_context_json,status,priority,deadline_at
		) values ($1,$2,$3,$4,'flight.return_home','integration-rth-disconnected','flight.return_home','{}','{}','dispatchable',90,$5)`,
		disconnectedID, projectID, teamID, aircraftID, clock.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	publisher.err = errors.New("connection unavailable")
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	dispatchErr := dispatcher.DispatchHandler(ctx, tx, outbox.Event{
		ProjectID: projectID, TeamID: teamID, EventID: "dispatch:" + disconnectedID,
		EventType: "device.command.dispatch", Payload: jsonObject(map[string]any{"commandId": disconnectedID}),
	})
	_ = tx.Rollback()
	if dispatchErr == nil {
		t.Fatal("disconnected publisher reported a sent return-home command")
	}
	var correlationCount int
	if err := database.QueryRowContext(ctx, `select command.status,
		(select count(*) from device_command_protocol_correlations where command_id=command.id)
		from device_commands command where command.id=$1`, disconnectedID).Scan(&status, &correlationCount); err != nil {
		t.Fatal(err)
	}
	if status != "dispatchable" || correlationCount != 0 {
		t.Fatalf("disconnected publish left a false sent correlation: status=%s correlations=%d", status, correlationCount)
	}
}

func routedReply(t *testing.T, projectID int, adapterID int64, commandID string, result int) RoutedMessage {
	t.Helper()
	raw := jsonObject(map[string]any{
		"tid": commandID, "bid": commandID, "timestamp": 1787821300100,
		"gateway": "DOCK2-DEMO-001", "method": "return_home", "data": map[string]any{"result": result},
	})
	message, err := RouteMQTTMessage(RouteContext{
		ProjectID: projectID, AdapterID: adapterID, AllowedGatewaySNs: map[string]bool{"DOCK2-DEMO-001": true},
	}, MQTTMessage{Topic: "thing/product/DOCK2-DEMO-001/services_reply", Payload: raw, QoS: 1, ReceivedAt: time.UnixMilli(1787821300200).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func replyOutboxEvent(t *testing.T, teamID int, message RoutedMessage) outbox.Event {
	t.Helper()
	payload, err := json.Marshal(message.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	return outbox.Event{ProjectID: message.Envelope.ProjectID, TeamID: teamID, EventID: message.Envelope.EventID, EventType: message.Envelope.EventType, Payload: payload}
}

func callOutboxHandler(t *testing.T, ctx context.Context, database *sql.DB, handler outbox.Handler, event outbox.Event) {
	t.Helper()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := handler(ctx, tx, event); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
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
