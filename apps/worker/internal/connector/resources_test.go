package connector

import (
	"context"
	"database/sql"
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
