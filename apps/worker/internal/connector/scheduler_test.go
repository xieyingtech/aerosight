package connector

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aerosight/worker/internal/outbox"
)

type schedulerLeaseFixture struct {
	mu           sync.Mutex
	lease        InstanceLease
	owner        string
	due          bool
	renew        bool
	claimDueAt   time.Time
	claimCount   int
	releaseCount int
	backlog      int
}

func (fixture *schedulerLeaseFixture) Backlog(context.Context, string, string) (int, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.backlog, nil
}

func (fixture *schedulerLeaseFixture) claim(owner string) (InstanceLease, bool) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.owner != "" {
		return InstanceLease{}, false
	}
	fixture.owner = owner
	fixture.claimCount++
	fixture.lease.Epoch++
	fixture.lease.LeaseEpoch = fixture.lease.Epoch
	fixture.lease.LeaseOwner = owner
	return fixture.lease, true
}

func (fixture *schedulerLeaseFixture) ClaimDue(
	_ context.Context, owner, _, _ string, dueBefore time.Time, _ int, _ time.Duration,
) ([]InstanceLease, error) {
	fixture.mu.Lock()
	fixture.claimDueAt = dueBefore
	due := fixture.due
	fixture.mu.Unlock()
	if !due {
		return nil, nil
	}
	lease, claimed := fixture.claim(owner)
	if !claimed {
		return nil, nil
	}
	return []InstanceLease{lease}, nil
}

func (fixture *schedulerLeaseFixture) ClaimInstance(
	_ context.Context, owner string, projectID int, instanceID int64, connectorKey, version string, _ time.Duration,
) (InstanceLease, bool, error) {
	if fixture.lease.ProjectID != projectID || fixture.lease.ID != instanceID ||
		fixture.lease.ConnectorKey != connectorKey || fixture.lease.Version != version {
		return InstanceLease{}, false, nil
	}
	lease, claimed := fixture.claim(owner)
	return lease, claimed, nil
}

func (fixture *schedulerLeaseFixture) Renew(_ context.Context, lease InstanceLease, owner string, _ time.Duration) (bool, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.renew && fixture.owner == owner && fixture.lease.Epoch == lease.Epoch, nil
}

func (fixture *schedulerLeaseFixture) Release(_ context.Context, lease InstanceLease, owner string) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.owner == owner && fixture.lease.Epoch == lease.Epoch {
		fixture.owner = ""
		fixture.releaseCount++
	}
	return nil
}

func (fixture *schedulerLeaseFixture) expire() {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.owner = ""
}

type schedulerRunnerFixture struct {
	mu      sync.Mutex
	runs    int
	started chan struct{}
	release chan struct{}
	err     error
}

func (fixture *schedulerRunnerFixture) Run(ctx context.Context, instance Instance, mode DiscoveryMode) (SyncApplyResult, error) {
	if instance.LeaseOwner == "" || instance.LeaseEpoch <= 0 || mode != DiscoveryPoll {
		return SyncApplyResult{}, errors.New("runner received an unleased instance")
	}
	fixture.mu.Lock()
	fixture.runs++
	fixture.mu.Unlock()
	if fixture.started != nil {
		select {
		case fixture.started <- struct{}{}:
		default:
		}
	}
	if fixture.release != nil {
		select {
		case <-fixture.release:
		case <-ctx.Done():
			return SyncApplyResult{}, ctx.Err()
		}
	}
	if fixture.err != nil {
		return SyncApplyResult{}, fixture.err
	}
	return SyncApplyResult{RunID: int64(fixture.runs), Discovered: 2}, nil
}

type schedulerOutcomeFixture struct {
	leases    *schedulerLeaseFixture
	mu        sync.Mutex
	succeeded int
	failed    []string
}

type safeConnectorError string

func (err safeConnectorError) Error() string             { return "redacted connector failure" }
func (err safeConnectorError) ConnectorSafeCode() string { return string(err) }

func TestConnectorMetricOutcomeUsesOnlyBoundedSafeLabels(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, "succeeded"},
		{safeConnectorError("rate_limited"), "rate_limited"},
		{safeConnectorError("credential_invalid"), "credential_invalid"},
		{safeConnectorError("schema_incompatible"), "schema_incompatible"},
		{safeConnectorError("directory_limit_reached"), "directory_incomplete"},
		{ErrConnectorLeaseLost, "lease_lost"},
		{errors.New("Bearer secret-token with project 17"), "failed"},
	}
	for _, test := range tests {
		if got := connectorMetricOutcome(test.err); got != test.want {
			t.Fatalf("metric outcome=%q want=%q for %v", got, test.want, test.err)
		}
	}
}

func (fixture *schedulerOutcomeFixture) Succeeded(ctx context.Context, lease InstanceLease, owner string, _ SyncApplyResult) error {
	fixture.mu.Lock()
	fixture.succeeded++
	fixture.mu.Unlock()
	return fixture.leases.Release(ctx, lease, owner)
}

func (fixture *schedulerOutcomeFixture) Failed(ctx context.Context, lease InstanceLease, owner string, _ DiscoveryMode, code string) error {
	fixture.mu.Lock()
	fixture.failed = append(fixture.failed, code)
	fixture.mu.Unlock()
	return fixture.leases.Release(ctx, lease, owner)
}

func schedulerFixture(t *testing.T, owner string, leases *schedulerLeaseFixture, runner SyncRunner, outcomes SyncOutcomeStore) *Scheduler {
	t.Helper()
	scheduler, err := NewScheduler(leases, runner, outcomes, SchedulerConfig{
		Owner: owner, ConnectorKey: "dji.flighthub2", Version: "1.0.0",
		PollInterval: time.Minute, JitterWindow: 10 * time.Second, ReconcileEvery: time.Second,
		LeaseDuration: 100 * time.Millisecond, RenewEvery: 25 * time.Millisecond, BatchSize: 2,
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		Jitter: func(time.Duration) time.Duration { return 5 * time.Second },
	})
	if err != nil {
		t.Fatal(err)
	}
	return scheduler
}

func newSchedulerLease() *schedulerLeaseFixture {
	return &schedulerLeaseFixture{renew: true, lease: InstanceLease{
		Instance: Instance{ID: 7, ProjectID: 3, ConnectorKey: "dji.flighthub2", Version: "1.0.0"},
		TeamID:   9,
	}}
}

func syncEvent() outbox.Event {
	payload, _ := json.Marshal(map[string]string{
		"connectorInstanceId": "7", "connectorKey": "dji.flighthub2", "discoveryMode": "poll", "trigger": "manual",
	})
	return outbox.Event{ProjectID: 3, TeamID: 9, EventType: "connector.sync.requested", Payload: payload}
}

func TestConcurrentSyncRequestsAllowOnlyOneWorkerPerInstance(t *testing.T) {
	leases := newSchedulerLease()
	runner := &schedulerRunnerFixture{started: make(chan struct{}, 1), release: make(chan struct{})}
	outcomes := &schedulerOutcomeFixture{leases: leases}
	first := schedulerFixture(t, "worker-a", leases, runner, outcomes)
	second := schedulerFixture(t, "worker-b", leases, runner, outcomes)
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.OutboxHandler(context.Background(), nil, syncEvent()) }()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("first worker did not start")
	}
	if err := second.OutboxHandler(context.Background(), nil, syncEvent()); !errors.Is(err, ErrConnectorLeaseUnavailable) {
		t.Fatalf("second worker entered the same connector instance: %v", err)
	}
	close(runner.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.runs != 1 || outcomes.succeeded != 1 {
		t.Fatalf("connector was not single-active: runs=%d successes=%d", runner.runs, outcomes.succeeded)
	}
}

func TestExpiredLeaseCanBeRecoveredAfterWorkerRestart(t *testing.T) {
	leases := newSchedulerLease()
	if _, claimed := leases.claim("dead-worker"); !claimed {
		t.Fatal("failed to arrange stale worker lease")
	}
	leases.expire()
	runner := &schedulerRunnerFixture{}
	outcomes := &schedulerOutcomeFixture{leases: leases}
	restarted := schedulerFixture(t, "restarted-worker", leases, runner, outcomes)
	if err := restarted.OutboxHandler(context.Background(), nil, syncEvent()); err != nil {
		t.Fatal(err)
	}
	if runner.runs != 1 || leases.claimCount != 2 || outcomes.succeeded != 1 {
		t.Fatalf("expired lease was not recovered: runs=%d claims=%d successes=%d", runner.runs, leases.claimCount, outcomes.succeeded)
	}
}

func TestLeaseRenewalLossCancelsSyncAndRecordsFailure(t *testing.T) {
	leases := newSchedulerLease()
	leases.renew = false
	runner := &schedulerRunnerFixture{release: make(chan struct{})}
	outcomes := &schedulerOutcomeFixture{leases: leases}
	scheduler := schedulerFixture(t, "worker-a", leases, runner, outcomes)
	err := scheduler.OutboxHandler(context.Background(), nil, syncEvent())
	if !errors.Is(err, ErrConnectorLeaseLost) || outcomes.succeeded != 0 || len(outcomes.failed) != 1 {
		t.Fatalf("lost lease outcome mismatch: error=%v successes=%d failures=%v", err, outcomes.succeeded, outcomes.failed)
	}
}

func TestPeriodicReconcileUsesBoundedJitterAndSharedLease(t *testing.T) {
	leases := newSchedulerLease()
	leases.due = true
	runner := &schedulerRunnerFixture{}
	outcomes := &schedulerOutcomeFixture{leases: leases}
	scheduler := schedulerFixture(t, "periodic-worker", leases, runner, outcomes)
	count, err := scheduler.ReconcileOnce(context.Background())
	if err != nil || count != 1 || runner.runs != 1 {
		t.Fatalf("periodic reconciliation failed: count=%d runs=%d error=%v", count, runner.runs, err)
	}
	want := time.Unix(1_700_000_000, 0).Add(-time.Minute - 5*time.Second)
	if !leases.claimDueAt.Equal(want) {
		t.Fatalf("periodic jitter cutoff=%s want=%s", leases.claimDueAt, want)
	}
}

func TestOutboxSyncRequestRejectsCrossScopeAndMalformedPayload(t *testing.T) {
	leases := newSchedulerLease()
	runner := &schedulerRunnerFixture{}
	outcomes := &schedulerOutcomeFixture{leases: leases}
	scheduler := schedulerFixture(t, "worker-a", leases, runner, outcomes)
	event := syncEvent()
	event.TeamID = 10
	if err := scheduler.OutboxHandler(context.Background(), nil, event); !errors.Is(err, ErrConnectorLeaseUnavailable) || runner.runs != 0 {
		t.Fatalf("cross-team event reached runner: %v", err)
	}
	event = syncEvent()
	event.Payload = json.RawMessage(`{"connectorInstanceId":"7","connectorKey":"dji.flighthub2","discoveryMode":"subscribe","trigger":"manual"}`)
	if err := scheduler.OutboxHandler(context.Background(), nil, event); err == nil || runner.runs != 0 {
		t.Fatalf("malformed mode reached runner: %v", err)
	}
}
