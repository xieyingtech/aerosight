package outbox

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"
)

type fakeRepository struct {
	event       Event
	claims      int
	consumed    bool
	completed   int
	dead        bool
	lastFailure error
}

func (repository *fakeRepository) Claim(_ context.Context, _ string, eventTypes []string, _ int, _ time.Duration) ([]Event, error) {
	if repository.dead {
		return nil, nil
	}
	if !slices.Contains(eventTypes, repository.event.EventType) {
		return nil, nil
	}
	repository.claims++
	event := repository.event
	event.Attempts = repository.claims
	return []Event{event}, nil
}

func (repository *fakeRepository) Process(ctx context.Context, _ string, event Event, handler Handler) error {
	if repository.consumed {
		return nil
	}
	if err := handler(ctx, nil, event); err != nil {
		return err
	}
	repository.consumed = true
	return nil
}

func (repository *fakeRepository) Complete(context.Context, string, int64) error {
	repository.completed++
	return nil
}

func (repository *fakeRepository) Fail(_ context.Context, _ string, event Event, cause error, _ time.Duration) (string, error) {
	repository.lastFailure = cause
	if event.Attempts >= event.MaxAttempts {
		repository.dead = true
		return "dead", nil
	}
	return "pending", nil
}

func testConsumer(repository Repository) *Consumer {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewConsumer(repository, "worker-test", "consumer-test", logger)
}

func TestDuplicateDeliveryAfterRestartRunsHandlerOnce(t *testing.T) {
	repository := &fakeRepository{event: Event{ID: 1, EventID: "event-1", EventType: "known", MaxAttempts: 3}}
	consumer := testConsumer(repository)
	handled := 0
	consumer.Register("known", func(context.Context, *sql.Tx, Event) error {
		handled++
		return nil
	})

	if _, err := consumer.ConsumeOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := consumer.ConsumeOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if handled != 1 || repository.completed != 2 {
		t.Fatalf("duplicate handling mismatch: handled=%d completed=%d", handled, repository.completed)
	}
}

func TestPoisonEventBecomesDead(t *testing.T) {
	repository := &fakeRepository{event: Event{ID: 2, EventID: "event-2", EventType: "poison", MaxAttempts: 3}}
	consumer := testConsumer(repository)
	consumer.Register("poison", func(context.Context, *sql.Tx, Event) error {
		return errors.New("poison payload")
	})

	for range 3 {
		if _, err := consumer.ConsumeOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if !repository.dead || repository.lastFailure == nil {
		t.Fatalf("poison event was not dead-lettered: %#v", repository)
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	if retryDelay(1) != time.Second || retryDelay(20) != 5*time.Minute {
		t.Fatalf("unexpected retry delays: first=%s capped=%s", retryDelay(1), retryDelay(20))
	}
}

func TestUnknownEventIsNotClaimed(t *testing.T) {
	repository := &fakeRepository{event: Event{ID: 3, EventID: "future-event", EventType: "future.evidence.sealed", MaxAttempts: 3}}
	consumer := testConsumer(repository)
	consumer.Register("known", func(context.Context, *sql.Tx, Event) error { return nil })

	claimed, err := consumer.ConsumeOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if claimed != 0 || repository.claims != 0 || repository.completed != 0 || repository.lastFailure != nil {
		t.Fatalf("unknown event was mutated: %#v", repository)
	}
}

type unavailableRepository struct {
	*fakeRepository
	err error
}

func (r unavailableRepository) Claim(context.Context, string, []string, int, time.Duration) ([]Event, error) {
	return nil, r.err
}

func TestRunReportsUnavailableRepository(t *testing.T) {
	failure := errors.New("claim unavailable")
	consumer := testConsumer(unavailableRepository{fakeRepository: &fakeRepository{}, err: failure})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := consumer.Run(ctx); !errors.Is(err, failure) {
		t.Fatalf("lost repository failure: %v", err)
	}
}

func TestCancellationLeavesClaimedEventForLeaseRecovery(t *testing.T) {
	for _, beforeClaim := range []bool{true, false} {
		t.Run(map[bool]string{true: "before claim", false: "during handler"}[beforeClaim], func(t *testing.T) {
			repository := &fakeRepository{event: Event{ID: 1, EventID: "event-1", EventType: "known", MaxAttempts: 3}}
			consumer := testConsumer(repository)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			consumer.Register("known", func(context.Context, *sql.Tx, Event) error {
				cancel()
				return context.Canceled
			})
			if beforeClaim {
				cancel()
			}
			if err := consumer.Run(ctx); err != nil {
				t.Fatal(err)
			}
			wantClaims := 1
			if beforeClaim {
				wantClaims = 0
			}
			if repository.claims != wantClaims || repository.completed != 0 || repository.lastFailure != nil || repository.consumed {
				t.Fatalf("cancelled work changed state: %#v", repository)
			}
		})
	}
}
