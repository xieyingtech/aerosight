package connector

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lifecycleRunner func(context.Context, Instance, DiscoveryMode) (SyncApplyResult, error)

func (run lifecycleRunner) Run(ctx context.Context, instance Instance, mode DiscoveryMode) (SyncApplyResult, error) {
	return run(ctx, instance, mode)
}

type lifecycleLeases struct {
	*schedulerLeaseFixture
	batch        bool
	renewing     chan struct{}
	renewStopped chan struct{}
}

func (leases lifecycleLeases) ClaimDue(ctx context.Context, owner, key, version string, due time.Time, limit int, duration time.Duration) ([]InstanceLease, error) {
	batch, err := leases.schedulerLeaseFixture.ClaimDue(ctx, owner, key, version, due, limit, duration)
	if leases.batch && len(batch) == 1 {
		second := batch[0]
		second.ID++
		batch = append(batch, second)
	}
	return batch, err
}
func (leases lifecycleLeases) Renew(ctx context.Context, lease InstanceLease, owner string, duration time.Duration) (bool, error) {
	if leases.renewing == nil {
		return leases.schedulerLeaseFixture.Renew(ctx, lease, owner, duration)
	}
	close(leases.renewing)
	<-ctx.Done()
	close(leases.renewStopped)
	return false, ctx.Err()
}

func TestSchedulerCancellationSkipsClaimsAndRemainingBatch(t *testing.T) {
	for _, beforeClaim := range []bool{true, false} {
		t.Run(map[bool]string{true: "before claim", false: "during first batch item"}[beforeClaim], func(t *testing.T) {
			leases := newSchedulerLease()
			leases.due = true
			outcomes := &schedulerOutcomeFixture{leases: leases}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runs := 0
			runner := lifecycleRunner(func(context.Context, Instance, DiscoveryMode) (SyncApplyResult, error) {
				runs++
				cancel()
				return SyncApplyResult{}, nil
			})
			scheduler := schedulerFixture(t, "worker", leases, runner, outcomes)
			scheduler.leases = lifecycleLeases{schedulerLeaseFixture: leases, batch: true}
			if beforeClaim {
				cancel()
			}
			if _, err := scheduler.ReconcileOnce(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
			want := 1
			if beforeClaim {
				want = 0
			}
			if runs != want || leases.claimCount != want || outcomes.succeeded != 0 || len(outcomes.failed) != 0 || leases.releaseCount != 0 {
				t.Fatalf("cancelled batch mutated outcomes: runs=%d claims=%d success=%d failures=%v releases=%d", runs, leases.claimCount, outcomes.succeeded, outcomes.failed, leases.releaseCount)
			}
			if err := scheduler.OutboxHandler(ctx, nil, syncEvent()); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if leases.claimCount != want {
				t.Fatal("cancelled callback claimed work")
			}
		})
	}
}

func TestSchedulerShutdownWaitsForRenewalCancellation(t *testing.T) {
	leases := newSchedulerLease()
	leases.due = true
	outcomes := &schedulerOutcomeFixture{leases: leases}
	runner := &schedulerRunnerFixture{release: make(chan struct{})}
	scheduler := schedulerFixture(t, "worker", leases, runner, outcomes)
	renewing, renewStopped := make(chan struct{}), make(chan struct{})
	scheduler.leases = lifecycleLeases{schedulerLeaseFixture: leases, renewing: renewing, renewStopped: renewStopped}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	select {
	case <-renewing:
	case <-time.After(time.Second):
		t.Fatal("renewal did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
	select {
	case <-renewStopped:
	default:
		t.Fatal("scheduler returned before renewal stopped")
	}
	if outcomes.succeeded != 0 || len(outcomes.failed) != 0 || leases.owner != "worker" {
		t.Fatal("cancelled sync finalized or released lease")
	}
}
