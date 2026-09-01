package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRemoteResourceValidation(t *testing.T) {
	t.Parallel()
	if err := validateRemoteBatch(RemoteResourceBatch{Kind: "unknown"}); err == nil {
		t.Fatal("unknown resource kind was accepted")
	}
	if err := validateRemoteBatch(RemoteResourceBatch{Kind: "model", Resources: []RemoteResource{{RemoteID: " duplicate"}}}); err == nil {
		t.Fatal("unnormalized remote id was accepted")
	}
	if err := validateRemoteBatch(RemoteResourceBatch{Kind: "model", Resources: []RemoteResource{{RemoteID: "same"}, {RemoteID: "same"}}}); err == nil {
		t.Fatal("duplicate remote id was accepted")
	}
	if err := validateRemoteBatch(RemoteResourceBatch{Kind: "model", Resources: []RemoteResource{{RemoteID: "model-1", Canonical: &CanonicalResourceLink{TargetType: "asset"}}}}); err == nil {
		t.Fatal("partial canonical link was accepted")
	}
}

func TestSQLResourceRepositoryTenantIsolationAndIdempotency(t *testing.T) {
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
	var definitionID int64
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	type scope struct {
		teamID, projectID int
		connectorID       int64
	}
	createScope := func(label string) scope {
		t.Helper()
		var item scope
		if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("resource-%s-%d", label, suffix)).Scan(&item.teamID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, item.teamID)
		})
		if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, item.teamID, fmt.Sprintf("resource-%s-%d", label, suffix)).Scan(&item.projectID); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `insert into device_adapters(
			project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
		) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`,
			item.projectID, item.teamID, fmt.Sprintf("resource-%s-%d", label, suffix), definitionID).Scan(&item.connectorID); err != nil {
			t.Fatal(err)
		}
		return item
	}
	left, right := createScope("left"), createScope("right")
	repository := NewSQLResourceRepository(database)
	resource := RemoteResource{RemoteID: "same-remote-id", RemoteVersion: "1", Summary: map[string]any{"name": "sanitized"}}
	for _, item := range []scope{left, right} {
		result, err := repository.ApplyRemoteResources(ctx, Instance{ID: item.connectorID, ProjectID: item.projectID}, RemoteResourceBatch{
			Kind: "wayline", Resources: []RemoteResource{resource}, CompleteSnapshot: true,
		})
		if err != nil || result.Upserted != 1 || result.Missing != 0 {
			t.Fatalf("apply result=%#v err=%v", result, err)
		}
	}
	result, err := repository.ApplyRemoteResources(ctx, Instance{ID: left.connectorID, ProjectID: left.projectID}, RemoteResourceBatch{
		Kind: "wayline", Resources: []RemoteResource{{RemoteID: resource.RemoteID, RemoteVersion: "2", Summary: map[string]any{"name": "updated"}}}, CompleteSnapshot: true,
	})
	if err != nil || result.Upserted != 1 || result.Missing != 0 {
		t.Fatalf("idempotent apply result=%#v err=%v", result, err)
	}
	if err := repository.LinkRemoteResource(ctx, Instance{ID: left.connectorID, ProjectID: right.projectID}, "wayline", resource.RemoteID, CanonicalResourceLink{TargetType: "task", TargetID: "7"}); !errors.Is(err, ErrRemoteResourceUnavailable) {
		t.Fatalf("cross-project link err=%v", err)
	}
	if err := repository.LinkRemoteResource(ctx, Instance{ID: left.connectorID, ProjectID: left.projectID}, "wayline", resource.RemoteID, CanonicalResourceLink{TargetType: "task", TargetID: "7"}); err != nil {
		t.Fatal(err)
	}
	missing, err := repository.ApplyRemoteResources(ctx, Instance{ID: left.connectorID, ProjectID: left.projectID}, RemoteResourceBatch{Kind: "wayline", CompleteSnapshot: true})
	if err != nil || missing.Missing != 1 {
		t.Fatalf("missing result=%#v err=%v", missing, err)
	}
	var leftCount, rightCount int
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and remote_id=$2`, left.projectID, resource.RemoteID).Scan(&leftCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and remote_id=$2`, right.projectID, resource.RemoteID).Scan(&rightCount); err != nil {
		t.Fatal(err)
	}
	if leftCount != 1 || rightCount != 1 {
		t.Fatalf("remote resources crossed project boundary: left=%d right=%d", leftCount, rightCount)
	}
}

func TestDisabledConnectorRejectsOutboxLeaseSyncAndResourceWritesButKeepsHistory(t *testing.T) {
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
	var connectorID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("disabled-connector-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("disabled-connector-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`, projectID, teamID, fmt.Sprintf("disabled-connector-%d", suffix), definitionID).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	instance := Instance{ID: connectorID, ProjectID: projectID, ConnectorKey: "dji.flighthub2", Version: "1.0.0"}
	repository := NewSQLResourceRepository(database)
	if _, err := repository.ApplyRemoteResources(ctx, instance, RemoteResourceBatch{Kind: "model", Resources: []RemoteResource{{RemoteID: "historical-resource"}}, CompleteSnapshot: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'connector.sync.requested',$4)`, projectID, teamID, fmt.Sprintf("disabled-sync-%d", suffix),
		fmt.Sprintf(`{"connectorInstanceId":"%d","connectorKey":"dji.flighthub2"}`, connectorID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `update device_adapters set status='disabled',lease_owner=null,lease_expires_at=null where id=$1 and project_id=$2`, connectorID, projectID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AssertWritable(ctx, instance); !errors.Is(err, ErrConnectorDisabled) {
		t.Fatalf("disabled connector write gate err=%v", err)
	}
	if _, err := repository.ApplyRemoteResources(ctx, instance, RemoteResourceBatch{Kind: "model", Resources: []RemoteResource{{RemoteID: "new-resource"}}}); !errors.Is(err, ErrConnectorDisabled) {
		t.Fatalf("disabled connector resource write err=%v", err)
	}
	if _, claimed, err := NewSQLLeaseRepository(database).ClaimInstance(ctx, "worker-test", projectID, connectorID, "dji.flighthub2", "1.0.0", time.Minute); err != nil || claimed {
		t.Fatalf("disabled connector claimed by outbox worker: claimed=%v err=%v", claimed, err)
	}
	if backlog, err := NewSQLLeaseRepository(database).Backlog(ctx, "dji.flighthub2", "1.0.0"); err != nil || backlog != 0 {
		t.Fatalf("disabled connector remained in outbox backlog: backlog=%d err=%v", backlog, err)
	}
	if _, err := NewSQLSyncStore(database).ApplyBatch(ctx, instance, DiscoveryPoll, json.RawMessage(`{}`), DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "must-not-bind", ExternalType: "dji.dock2"}}, Cursor: json.RawMessage(`{"page":1}`),
	}); err == nil {
		t.Fatal("disabled connector applied a discovery batch")
	}
	var historical, newWrites int
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and remote_id='historical-resource'`, projectID, connectorID).Scan(&historical); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and remote_id='new-resource'`, projectID, connectorID).Scan(&newWrites); err != nil {
		t.Fatal(err)
	}
	if historical != 1 || newWrites != 0 {
		t.Fatalf("disabled lifecycle history=%d newWrites=%d", historical, newWrites)
	}
}
