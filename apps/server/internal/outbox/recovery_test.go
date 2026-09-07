package outbox

import (
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func recoveryDatabase(t *testing.T) (*sql.DB, int, int) {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	var team, project int
	if err := db.QueryRowContext(ctx, "INSERT INTO teams(name) VALUES ('Recovery test') RETURNING id").Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "INSERT INTO projects(team_id, name) VALUES ($1, 'Recovery project') RETURNING id", team).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE recovery_effects (event_id text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	return db, team, project
}

func TestPostgresRestartRecoversWithoutDuplicateTransactionEffects(t *testing.T) {
	db, team, project := recoveryDatabase(t)
	ctx := context.Background()
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel before commit", true: "restart before acknowledgement"}[committed], func(t *testing.T) {
			key := t.Name()
			if _, err := db.ExecContext(ctx, "INSERT INTO outbox_events(project_id, team_id, event_id, event_type) VALUES ($1,$2,$3,'recovery.test')", project, team, key); err != nil {
				t.Fatal(err)
			}
			oldStore := NewStore(db)
			events, err := oldStore.Claim(ctx, "old-worker", []string{"recovery.test"}, 1, time.Minute)
			if err != nil || len(events) != 1 {
				t.Fatalf("initial claim: %v %v", events, err)
			}
			handler := func(ctx context.Context, tx *sql.Tx, event Event) error {
				_, err := tx.ExecContext(ctx, "INSERT INTO recovery_effects(event_id) VALUES ($1)", event.EventID)
				return err
			}
			if committed {
				if err := oldStore.Process(ctx, "recovery-consumer", events[0], handler); err != nil {
					t.Fatal(err)
				}
			} else {
				operation, cancel := context.WithCancel(ctx)
				err := oldStore.Process(operation, "recovery-consumer", events[0], func(ctx context.Context, tx *sql.Tx, event Event) error {
					if err := handler(ctx, tx, event); err != nil {
						return err
					}
					cancel()
					return ctx.Err()
				})
				cancel()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled transaction: %v", err)
				}
			}
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM recovery_effects WHERE event_id=$1", key).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if committed {
				want = 1
			}
			if count != want {
				t.Fatalf("effects before recovery: %d want %d", count, want)
			}
			// Advance only this test event's persisted lease instead of sleeping.
			if _, err := db.ExecContext(ctx, "UPDATE outbox_events SET locked_until=now()-interval '1 second' WHERE event_id=$1", key); err != nil {
				t.Fatal(err)
			}
			restarted := NewConsumer(NewStore(db), "new-worker", "recovery-consumer", testConsumer(nil).logger)
			restarted.Register("recovery.test", handler)
			if n, err := restarted.ConsumeOnce(ctx); err != nil || n != 1 {
				t.Fatalf("recovery: %d %v", n, err)
			}
			if err := oldStore.Complete(ctx, "old-worker", events[0].ID); err == nil {
				t.Fatal("stale worker acknowledged recovered event")
			}
			var status string
			if err := db.QueryRowContext(ctx, "SELECT status FROM outbox_events WHERE event_id=$1", key).Scan(&status); err != nil || status != "completed" {
				t.Fatalf("recovered status %q: %v", status, err)
			}
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM recovery_effects WHERE event_id=$1", key).Scan(&count); err != nil || count != 1 {
				t.Fatalf("duplicate or lost effect: %d %v", count, err)
			}
			if n, err := restarted.ConsumeOnce(ctx); err != nil || n != 0 {
				t.Fatalf("completed event reclaimed: %d %v", n, err)
			}
		})
	}
}

func TestPostgresActiveTransactionCannotBeReclaimed(t *testing.T) {
	db, team, project := recoveryDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "INSERT INTO outbox_events(project_id,team_id,event_id,event_type) VALUES ($1,$2,'locked-event','recovery.test')", project, team); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	events, err := store.Claim(ctx, "old-worker", []string{"recovery.test"}, 1, time.Minute)
	if err != nil || len(events) != 1 {
		t.Fatalf("claim: %v %v", events, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE outbox_events SET locked_until=now()-interval '1 second' WHERE id=$1", events[0].ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		done <- store.Process(ctx, "recovery-consumer", events[0], func(ctx context.Context, tx *sql.Tx, event Event) error {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := tx.ExecContext(ctx, "INSERT INTO recovery_effects(event_id) VALUES ($1)", event.EventID)
			return err
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	claimed, err := store.Claim(ctx, "new-worker", []string{"recovery.test"}, 1, time.Minute)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("active transaction reclaimed: %v %v", claimed, err)
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	claimed, err = store.Claim(ctx, "new-worker", []string{"recovery.test"}, 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("finished transaction not recovered: %v %v", claimed, err)
	}
	called := false
	if err := store.Process(ctx, "recovery-consumer", events[0], func(context.Context, *sql.Tx, Event) error { called = true; return nil }); err == nil || called {
		t.Fatalf("stale worker invoked handler: %v called=%v", err, called)
	}
	if err := store.Process(ctx, "recovery-consumer", claimed[0], func(context.Context, *sql.Tx, Event) error { called = true; return nil }); err != nil || called {
		t.Fatalf("new worker repeated committed handler: %v called=%v", err, called)
	}
	if err := store.Complete(ctx, "new-worker", claimed[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresExhaustedLeaseResolvesCommittedAndUncommittedWork(t *testing.T) {
	db, team, project := recoveryDatabase(t)
	ctx := context.Background()
	store := NewStore(db)
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "dead without committed consumption", true: "acknowledge committed consumption"}[committed], func(t *testing.T) {
			key := t.Name()
			if _, err := db.ExecContext(ctx, "INSERT INTO outbox_events(project_id,team_id,event_id,event_type,max_attempts) VALUES ($1,$2,$3,'recovery.test',1)", project, team, key); err != nil {
				t.Fatal(err)
			}
			events, err := store.Claim(ctx, "old-worker", []string{"recovery.test"}, 1, time.Minute)
			if err != nil || len(events) != 1 {
				t.Fatalf("claim: %v %v", events, err)
			}
			if committed {
				if err := store.Process(ctx, "recovery-consumer", events[0], func(ctx context.Context, tx *sql.Tx, event Event) error {
					_, err := tx.ExecContext(ctx, "INSERT INTO recovery_effects(event_id) VALUES ($1)", event.EventID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, "UPDATE outbox_events SET locked_until=now()-interval '1 second' WHERE id=$1", events[0].ID); err != nil {
				t.Fatal(err)
			}
			// Recovery is scoped to the consumer's registered event types.
			if err := store.RecoverExhausted(ctx, "recovery-consumer", []string{"other.test"}, 20); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.QueryRowContext(ctx, "SELECT status FROM outbox_events WHERE id=$1", events[0].ID).Scan(&status); err != nil || status != "processing" {
				t.Fatalf("unregistered event mutated: %s %v", status, err)
			}
			restarted := NewConsumer(store, "new-worker", "recovery-consumer", testConsumer(nil).logger)
			called := false
			restarted.Register("recovery.test", func(context.Context, *sql.Tx, Event) error { called = true; return nil })
			if n, err := restarted.ConsumeOnce(ctx); err != nil || n != 0 || called {
				t.Fatalf("exhausted work repeated: %d %v called=%v", n, err, called)
			}
			var attempts int
			var unlocked bool
			var finished bool
			if err := db.QueryRowContext(ctx, "SELECT status,attempts,locked_by IS NULL AND locked_until IS NULL,completed_at IS NOT NULL FROM outbox_events WHERE id=$1", events[0].ID).Scan(&status, &attempts, &unlocked, &finished); err != nil {
				t.Fatal(err)
			}
			want := "dead"
			if committed {
				want = "completed"
			}
			if status != want || attempts != 1 || !unlocked || finished != committed {
				t.Fatalf("recovery state: %s attempts=%d unlocked=%v finished=%v", status, attempts, unlocked, finished)
			}
		})
	}
}
