package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSQLSyncStoreMarksCrossSourceSerialConflictWithoutChangingBindings(t *testing.T) {
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
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("connector-sync-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID)
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("connector-sync-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var directDefinitionID, flightHubDefinitionID int64
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.cloud-api' and version='1.0.0'`).Scan(&directDefinitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&flightHubDefinitionID); err != nil {
		t.Fatal(err)
	}
	var directID, flightHubID int64
	if err := database.QueryRowContext(ctx, `
		insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
		values($1,$2,$3,'dji',$4,'1','connected') returning id`,
		projectID, teamID, fmt.Sprintf("direct-%d", suffix), directDefinitionID).Scan(&directID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		insert into device_adapters(
		  project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,external_scope_key
		) values($1,$2,$3,'dji-flighthub2',$4,'2','connecting',$5,$6) returning id`,
		projectID, teamID, fmt.Sprintf("flighthub-%d", suffix), flightHubDefinitionID,
		json.RawMessage(`{"projectUuid":"00000000-0000-4000-8000-000000000001","projectName":"测试"}`),
		fmt.Sprintf("00000000-0000-4000-8000-%012d", suffix%1_000_000_000_000)).Scan(&flightHubID); err != nil {
		t.Fatal(err)
	}
	const serial = "CROSS-SOURCE-SN-001"
	var directIdentityID int64
	if err := database.QueryRowContext(ctx, `
		insert into device_external_identities(
		  project_id,team_id,adapter_id,external_device_id,external_device_type,identity_json,discovery_status
		) values($1,$2,$3,$4,'dji.dock2','{}','managed') returning id`,
		projectID, teamID, directID, serial).Scan(&directIdentityID); err != nil {
		t.Fatal(err)
	}
	store := NewSQLSyncStore(database)
	instance := Instance{ID: flightHubID, ProjectID: projectID, ConnectorKey: "dji.flighthub2", Version: "1.0.0"}
	result, err := store.ApplyBatch(ctx, instance, DiscoveryPoll, json.RawMessage(`{}`), DiscoveryBatch{
		Devices: []ExternalDevice{{
			ExternalID:   "00000000-0000-4000-8000-000000000001/" + serial,
			ExternalType: "dji.dock2",
			Attributes:   map[string]any{"serialNumber": serial, "projectUuid": "00000000-0000-4000-8000-000000000001"},
		}},
		Cursor: json.RawMessage(`{"snapshotSha256":"integration"}`), CompleteSnapshot: true, SourceVersion: "integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Discovered != 1 {
		t.Fatalf("unexpected sync result %#v", result)
	}
	rows, err := database.QueryContext(ctx, `
		select discovery_status,suggested_device_type_id is not null
		  from device_external_identities where project_id=$1 and id=$2 or (project_id=$1 and adapter_id=$3)
		 order by id`, projectID, directIdentityID, flightHubID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var status string
		var suggested bool
		if err := rows.Scan(&status, &suggested); err != nil {
			t.Fatal(err)
		}
		if status != "conflicted" {
			t.Fatalf("cross-source identity status=%s", status)
		}
		if count == 1 && !suggested {
			t.Fatal("FlightHub identity did not resolve the existing DJI DeviceType")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected both source identities, got %d", count)
	}
	var bindings int
	if err := database.QueryRowContext(ctx, `select count(*) from device_connector_bindings where project_id=$1`, projectID).Scan(&bindings); err != nil {
		t.Fatal(err)
	}
	if bindings != 0 {
		t.Fatalf("conflict policy automatically changed downstream routing: bindings=%d", bindings)
	}
}

func TestSQLSyncStoreAppliesSafeOnboardingPolicies(t *testing.T) {
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
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("onboarding-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID)
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("onboarding-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var definitionID int64
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	createAdapter := func(policy string) int64 {
		t.Helper()
		var adapterID int64
		if err := database.QueryRowContext(ctx, `
			insert into device_adapters(
			  project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,onboarding_policy
			) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5) returning id`,
			projectID, teamID, fmt.Sprintf("%s-%d", policy, suffix), definitionID, policy).Scan(&adapterID); err != nil {
			t.Fatal(err)
		}
		return adapterID
	}
	store := NewSQLSyncStore(database)
	automaticID := createAdapter("automatic")
	automatic := Instance{ID: automaticID, ProjectID: projectID, ConnectorKey: "dji.flighthub2", Version: "1.0.0"}
	result, err := store.ApplyBatch(ctx, automatic, DiscoveryPoll, json.RawMessage(`{}`), DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "dock-exact", ExternalType: "dji.dock2", Attributes: map[string]any{"name": "Dock exact"}}},
		Cursor:  json.RawMessage(`{"page":1}`), SourceVersion: "integration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Managed != 1 || result.Discovered != 1 {
		t.Fatalf("automatic exact match was not safely managed: %#v", result)
	}
	var deviceID int
	var status, role string
	if err := database.QueryRowContext(ctx, `
		select identity.device_id,identity.discovery_status,binding.route_role
		  from device_external_identities identity
		  join device_connector_bindings binding on binding.external_identity_id=identity.id
		 where identity.adapter_id=$1 and identity.external_device_id='dock-exact'`, automaticID).Scan(&deviceID, &status, &role); err != nil {
		t.Fatal(err)
	}
	if deviceID <= 0 || status != "managed" || role != "gateway" {
		t.Fatalf("unexpected automatic binding: device=%d status=%s role=%s", deviceID, status, role)
	}
	replayed, err := store.ApplyBatch(ctx, automatic, DiscoveryPoll, json.RawMessage(`{"page":1}`), DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "dock-exact", ExternalType: "dji.dock2"}},
		Cursor:  json.RawMessage(`{"page":1}`), SourceVersion: "integration",
	})
	if err != nil || !replayed.ReplayedCursor {
		t.Fatalf("cursor replay was not idempotent: %#v err=%v", replayed, err)
	}
	var deviceCount int
	if err := database.QueryRowContext(ctx, `select count(*) from devices where project_id=$1`, projectID).Scan(&deviceCount); err != nil || deviceCount != 1 {
		t.Fatalf("replay duplicated device: count=%d err=%v", deviceCount, err)
	}
	if _, err := database.ExecContext(ctx, `
		insert into device_external_identities(project_id,team_id,adapter_id,external_device_id,external_device_type,identity_json,discovery_status)
		values($1,$2,$3,'ignored-dock','dji.dock2','{}','ignored')`, projectID, teamID, automaticID); err != nil {
		t.Fatal(err)
	}
	result, err = store.ApplyBatch(ctx, automatic, DiscoveryPoll, json.RawMessage(`{"page":1}`), DiscoveryBatch{
		Devices: []ExternalDevice{
			{ExternalID: "ignored-dock", ExternalType: "dji.dock2"},
			{ExternalID: "unknown-model", ExternalType: "vendor.unknown"},
		},
		Cursor: json.RawMessage(`{"page":2}`), SourceVersion: "integration",
	})
	if err != nil || result.Managed != 0 {
		t.Fatalf("ignored or unknown identity was auto managed: %#v err=%v", result, err)
	}
	rows, err := database.QueryContext(ctx, `select external_device_id,discovery_status,device_id is null
		from device_external_identities where adapter_id=$1 and external_device_id in('ignored-dock','unknown-model') order by external_device_id`, automaticID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]string{"ignored-dock": "ignored", "unknown-model": "discovered"}
	for rows.Next() {
		var externalID, gotStatus string
		var deviceMissing bool
		if err := rows.Scan(&externalID, &gotStatus, &deviceMissing); err != nil {
			t.Fatal(err)
		}
		if gotStatus != want[externalID] || !deviceMissing {
			t.Fatalf("unsafe identity transition: id=%s status=%s missing=%v", externalID, gotStatus, deviceMissing)
		}
		delete(want, externalID)
	}
	if len(want) != 0 {
		t.Fatalf("missing onboarding rows: %v", want)
	}

	reviewID := createAdapter("review")
	review := Instance{ID: reviewID, ProjectID: projectID, ConnectorKey: "dji.flighthub2", Version: "1.0.0"}
	reviewResult, err := store.ApplyBatch(ctx, review, DiscoveryPoll, json.RawMessage(`{}`), DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "review-dock", ExternalType: "dji.dock2"}},
		Cursor:  json.RawMessage(`{"page":1}`), SourceVersion: "integration",
	})
	if err != nil || reviewResult.Managed != 0 {
		t.Fatalf("review policy auto managed a device: %#v err=%v", reviewResult, err)
	}
	observeID := createAdapter("observe-only")
	observe := Instance{ID: observeID, ProjectID: projectID, ConnectorKey: "dji.flighthub2", Version: "1.0.0"}
	observeResult, err := store.ApplyBatch(ctx, observe, DiscoveryPoll, json.RawMessage(`{}`), DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "observe-dock", ExternalType: "dji.dock2"}},
		Cursor:  json.RawMessage(`{"page":1}`), SourceVersion: "integration",
	})
	if err != nil || observeResult.Managed != 0 {
		t.Fatalf("observe-only policy auto managed a device: %#v err=%v", observeResult, err)
	}
}
