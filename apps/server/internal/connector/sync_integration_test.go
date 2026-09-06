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
