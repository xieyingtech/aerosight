package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/connector"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/telemetry"
	"aerosight/server/internal/testdb"
)

func TestTaskManagedAlertNewUpdateMissingAndLegacyRelease(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(query string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := db.QueryRow(query, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	team := id("insert into teams(name) values('managed alerts') returning id")
	project := id("insert into projects(team_id,name) values($1,'managed alerts') returning id", team)
	adapter := id(`insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
 select $1,$2,'managed alerts','dji-flighthub2',id,'2','connected' from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0' returning id`, project, team)
	drone := id(`insert into devices(project_id,adapter_id,device_type_id,name,type,status)
 select $1,$2,id,'drone','aircraft','unknown' from device_types where type_key='dji.matrice3td' and status='active' order by version desc limit 1 returning id`, project, adapter)
	const serial = "managed-drone"
	exec(`insert into device_external_identities(project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at)
 values($1,$2,$3,$4,$5,'dji.matrice3td',jsonb_build_object('attributes',jsonb_build_object('serialNumber',$6::text)),'managed',now())`, project, team, adapter, drone, secureRemoteKey(serial), serial)
	instance := connector.Instance{ID: adapter, ProjectID: int(project), DiscoveryScope: []byte(`{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"managed alerts"}`)}
	now := time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
	projector := NewSQLFlightCatalogProjector(db, telemetry.NewIngestor(db), func() time.Time { return now }, time.Minute, flightProjectorTestSecret)
	base := AIAlertRecord{AlertUUID: "legacy-alert", FlightID: "legacy-flight", ProjectID: "11111111-1111-4111-8111-111111111111", DroneSN: serial, Timestamp: now.UnixMilli(), Status: 1, Reason: "person", Targets: []AIAlertTarget{{Label: "person", Confidence: 42}}, AlgorithmSource: 1}
	poll := func(alerts ...AIAlertRecord) {
		t.Helper()
		aggregates := []FlightAlertSummary{}
		seen := map[string]bool{}
		for _, alert := range alerts {
			if !seen[alert.FlightID] {
				aggregates = append(aggregates, FlightAlertSummary{FlightID: alert.FlightID, Count: 1, StartTime: now.Unix()})
				seen[alert.FlightID] = true
			}
		}
		if err := projector.ApplyFlightAlerts(ctx, instance, FlightAlertPoll{Aggregates: aggregates, Alerts: append([]AIAlertRecord{}, alerts...), ReceivedAt: now, CompleteSnapshot: true}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		var n int
		if err := db.QueryRow("select count(*) from issues where project_id=$1", project).Scan(&n); err != nil || n != want {
			t.Fatalf("issues %d want %d: %v", n, want, err)
		}
	}
	mutate := func(fn func(*sql.Tx) error) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err = fn(tx); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	// Absence of an explicit policy preserves the legacy lifecycle.
	poll(base)
	count(1)
	poll()
	var originalIssue int64
	var status string
	if err := db.QueryRow("select id,status from issues where project_id=$1", project).Scan(&originalIssue, &status); err != nil || status != "closed" {
		t.Fatalf("legacy did not close: %s %v", status, err)
	}
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'analysis','manual','typed-task-v2') returning id", project, team)
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
	run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", project, team, task, version)
	mutate(func(tx *sql.Tx) error {
		return inspection.ClaimAlertFlight(ctx, tx, int(project), adapter, base.FlightID, run)
	})
	mutate(func(tx *sql.Tx) error { return inspection.SetAlertPolicy(ctx, tx, int(project), adapter, true) })
	early := base
	early.AlertUUID = "early-alert"
	early.FlightID = "not-yet-bound-flight"
	poll(base, early)
	poll(base, early)
	count(1)
	if err := db.QueryRow("select status from issues where id=$1", originalIssue).Scan(&status); err != nil || status != "closed" {
		t.Fatal("managed update reopened old case")
	}
	var ownership string
	if err := db.QueryRow("select ownership from inspection_flight_ownership where project_id=$1 and remote_flight_id=$2", project, early.FlightID).Scan(&ownership); err != nil || ownership != "pending" {
		t.Fatalf("early alert not held: %s %v", ownership, err)
	}
	var raw []byte
	var hasConfidence bool
	if err := db.QueryRow(`select source.evidence_json,resource.summary_json ? 'confidence' from inspection_alert_sources source join connector_remote_resources resource on resource.id=source.remote_resource_id and resource.project_id=source.project_id where resource.project_id=$1 and resource.remote_id=$2`, project, early.AlertUUID).Scan(&raw, &hasConfidence); err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	if hasConfidence || evidence["targets"].([]any)[0].(map[string]any)["targetValue"] != float64(42) {
		t.Fatalf("target_value semantics lost: %s", raw)
	}
	// Changing the connector default cannot release known held/task-owned flights.
	mutate(func(tx *sql.Tx) error { return inspection.SetAlertPolicy(ctx, tx, int(project), adapter, false) })
	poll(base, early)
	count(1)
	exec("update issues set status='open',closed_at=null where id=$1", originalIssue)
	exec("update perception_events set status='open',resolved_at=null where project_id=$1", project)
	poll()
	count(1)
	if err := db.QueryRow("select status from issues where id=$1", originalIssue).Scan(&status); err != nil || status != "open" {
		t.Fatal("missing managed alert closed case")
	}
	var missing int
	if err := db.QueryRow("select count(*) from connector_remote_resources where project_id=$1 and resource_kind='ai-alert' and status='missing'", project).Scan(&missing); err != nil || missing != 2 {
		t.Fatalf("missing evidence not marked: %d %v", missing, err)
	}
	// Explicitly confirmed non-Task flight resumes only its own legacy lifecycle.
	mutate(func(tx *sql.Tx) error {
		return inspection.ReleaseLegacyFlight(ctx, tx, int(project), adapter, early.FlightID)
	})
	poll(base, early)
	count(2)
	poll()
	count(2)
	if err := db.QueryRow("select status from issues where id=$1", originalIssue).Scan(&status); err != nil || status != "open" {
		t.Fatal("legacy release affected managed case")
	}
	corrupt := base
	corrupt.FlightID = "wrong-flight"
	corruptPoll := FlightAlertPoll{Aggregates: []FlightAlertSummary{{FlightID: corrupt.FlightID, Count: 1, StartTime: now.Unix()}}, Alerts: []AIAlertRecord{corrupt}, ReceivedAt: now, CompleteSnapshot: true}
	if err := projector.ApplyFlightAlerts(ctx, instance, corruptPoll); err == nil || !strings.Contains(err.Error(), "FLIGHT_ID_CHANGED") {
		t.Fatalf("same alert rebound to a different flight: %v", err)
	}
	var linked sql.NullInt64
	if err := db.QueryRow("select task_run_id from issues where id=$1", originalIssue).Scan(&linked); err != nil || linked.Valid {
		t.Fatalf("legacy case implicitly bound to business Run: %+v %v", linked, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = inspection.ReleaseLegacyFlight(ctx, tx, int(project), adapter, base.FlightID); err == nil {
		t.Fatal("Task-owned flight released")
	}
}
