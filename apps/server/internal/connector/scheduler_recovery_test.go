package connector

import (
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPostgresSchedulerCancellationAndRestartRecovery(t *testing.T) {
	db := testdb.New(t)
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	var team, project int
	var adapter int64
	if err := db.QueryRowContext(ctx, "INSERT INTO teams(name) VALUES ('Scheduler recovery') RETURNING id").Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "INSERT INTO projects(team_id,name) VALUES ($1,'Scheduler recovery') RETURNING id", team).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,external_scope_key)
		SELECT $1,$2,'Recovery adapter','dji-flighthub2',id,'2','connecting','{"projectUuid":"00000000-0000-4000-8000-000000000001"}','00000000-0000-4000-8000-000000000001'
		FROM connector_definitions WHERE connector_key='dji.flighthub2' AND version='1.0.0' RETURNING id`, project, team).Scan(&adapter); err != nil {
		t.Fatal(err)
	}
	leases, outcomes := NewSQLLeaseRepository(db), NewSQLSyncOutcomeStore(db)
	newScheduler := func(owner string, runner SyncRunner) *Scheduler {
		s, err := NewScheduler(leases, runner, outcomes, SchedulerConfig{Owner: owner, ConnectorKey: "dji.flighthub2", Version: "1.0.0", PollInterval: time.Minute, ReconcileEvery: time.Second, LeaseDuration: time.Minute, RenewEvery: time.Second, BatchSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	started := make(chan Instance, 1)
	first := newScheduler("before-stop", lifecycleRunner(func(ctx context.Context, instance Instance, _ DiscoveryMode) (SyncApplyResult, error) {
		started <- instance
		<-ctx.Done()
		return SyncApplyResult{}, ctx.Err()
	}))
	operation, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := first.ReconcileOnce(operation); done <- err }()
	var old Instance
	select {
	case old = <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	var owner, status string
	var checked bool
	if err := db.QueryRowContext(ctx, "SELECT lease_owner,status,last_checked_at IS NOT NULL FROM device_adapters WHERE id=$1", adapter).Scan(&owner, &status, &checked); err != nil {
		t.Fatal(err)
	}
	if owner != "before-stop" || status != "connecting" || checked {
		t.Fatalf("cancel finalized adapter: %s %s %v", owner, status, checked)
	}
	var runs int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM connector_sync_runs WHERE connector_instance_id=$1", adapter).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("cancel wrote outcome: %d %v", runs, err)
	}
	if _, ok, err := leases.ClaimInstance(ctx, "early-restart", project, adapter, "dji.flighthub2", "1.0.0", time.Minute); err != nil || ok {
		t.Fatalf("live lease stolen: %v %v", ok, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE device_adapters SET lease_expires_at=now()-interval '1 second' WHERE id=$1", adapter); err != nil {
		t.Fatal(err)
	}
	recovered, ok, err := leases.ClaimInstance(ctx, "after-restart", project, adapter, "dji.flighthub2", "1.0.0", time.Minute)
	if err != nil || !ok || recovered.Epoch <= old.LeaseEpoch {
		t.Fatalf("recovery lease: %+v %v %v", recovered, ok, err)
	}
	stale := InstanceLease{Instance: old, TeamID: team, Epoch: old.LeaseEpoch}
	if ok, err := leases.Renew(ctx, stale, "before-stop", time.Minute); err != nil || ok {
		t.Fatalf("stale renew: %v %v", ok, err)
	}
	if err := leases.Release(ctx, stale, "before-stop"); err != nil {
		t.Fatal(err)
	}
	if err := outcomes.Succeeded(ctx, stale, "before-stop", SyncApplyResult{}); !errors.Is(err, ErrConnectorLeaseLost) {
		t.Fatalf("stale success: %v", err)
	}
	if err := outcomes.Failed(ctx, stale, "before-stop", DiscoveryPoll, "sync_failed"); !errors.Is(err, ErrConnectorLeaseLost) {
		t.Fatalf("stale failure: %v", err)
	}
	runner := lifecycleRunner(func(ctx context.Context, instance Instance, mode DiscoveryMode) (SyncApplyResult, error) {
		store := NewSQLSyncStore(db)
		cursor, err := store.CurrentCursor(ctx, instance)
		if err != nil {
			return SyncApplyResult{}, err
		}
		return store.ApplyBatch(ctx, instance, mode, cursor, DiscoveryBatch{
			Devices: []ExternalDevice{{ExternalID: "00000000-0000-4000-8000-000000000001/RECOVERY-DOCK", ExternalType: "dji.dock2", Attributes: map[string]any{"serialNumber": "RECOVERY-DOCK"}}},
			Cursor:  json.RawMessage(`{"recovered":true}`), CompleteSnapshot: true, SourceVersion: "recovery-test",
		})
	})
	restarted := newScheduler("after-restart", runner)
	if _, err := restarted.executeLease(ctx, recovered); err != nil {
		t.Fatal(err)
	}
	var held sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT lease_owner,status,last_checked_at IS NOT NULL FROM device_adapters WHERE id=$1", adapter).Scan(&held, &status, &checked); err != nil {
		t.Fatal(err)
	}
	if held.Valid || status != "connected" || !checked {
		t.Fatalf("recovery not completed: %v %s %v", held, status, checked)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM connector_sync_runs WHERE connector_instance_id=$1 AND status='succeeded'", adapter).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("recovered runs: %d %v", runs, err)
	}
	if n, err := restarted.ReconcileOnce(ctx); err != nil || n != 0 {
		t.Fatalf("completed sync immediately repeated: %d %v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM device_external_identities WHERE adapter_id=$1", adapter).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("recovered identities: %d %v", runs, err)
	}
}
