package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/testdb"
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestSessionCleanupPostgresExpiryAndLockedShutdown(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions(token, data, expiry) VALUES
		('expired', 'old', current_timestamp - interval '1 hour'),
		('active', 'keep', current_timestamp + interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	queries := sqlcgen.New(db)
	if err := queries.DeleteExpiredHTTPSessions(ctx); err != nil {
		t.Fatal(err)
	}
	var token, data string
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("session count %d: %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT token, data FROM sessions").Scan(&token, &data); err != nil || token != "active" || data != "keep" {
		t.Fatalf("active session changed: %q %q %v", token, data, err)
	}
	lock, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err := lock.ExecContext(ctx, "LOCK TABLE sessions IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	stop := startSessionCleanup(time.Millisecond, time.Hour, func(ctx context.Context) error {
		err := queries.DeleteExpiredHTTPSessions(ctx)
		finished <- err
		return err
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer stop()
	// Observe the real DELETE blocked in PostgreSQL before cancelling it.
	for {
		var blocked bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND query LIKE '-- name: DeleteExpiredHTTPSessions%')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cleanup failed to stop while table remains locked")
	}
	if err := <-finished; err == nil {
		t.Fatal("blocked cleanup unexpectedly succeeded")
	}
}

func TestSessionCleanupCancellationAndDeadline(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "shutdown"}[shutdown], func(t *testing.T) {
			started := make(chan struct{}, 1)
			finished := make(chan error, 1)
			timeout := 20 * time.Millisecond
			if shutdown {
				timeout = time.Hour
			}
			stop := startSessionCleanup(time.Millisecond, timeout, func(ctx context.Context) error {
				select {
				case started <- struct{}{}:
				default:
				}
				<-ctx.Done()
				select {
				case finished <- ctx.Err():
				default:
				}
				return ctx.Err()
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			defer stop()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("cleanup did not start")
			}
			if shutdown {
				stop()
			}
			select {
			case err := <-finished:
				want := context.DeadlineExceeded
				if shutdown {
					want = context.Canceled
				}
				if err != want {
					t.Fatalf("cleanup: %v want %v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup did not cancel")
			}
			var callers sync.WaitGroup
			for range 8 {
				callers.Go(stop)
			}
			callers.Wait()
		})
	}
}
