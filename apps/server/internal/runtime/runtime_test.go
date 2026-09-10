package runtime

import (
	"aerosight/server/internal/config"
	"aerosight/server/internal/httpapi"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFailureReportedBeforePeersDrain(t *testing.T) {
	for _, unexpectedSuccess := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "unexpected stop"}[unexpectedSuccess], func(t *testing.T) {
			failure := errors.New("consumer unavailable")
			started := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			cancelled := make(chan struct{})
			r := &Runtime{tasks: []func(context.Context) error{
				func(context.Context) error {
					<-started
					if unexpectedSuccess {
						return nil
					}
					return failure
				},
				func(ctx context.Context) error {
					close(started)
					<-ctx.Done()
					close(cancelled)
					<-release
					return nil
				},
			}}
			notified := make(chan error, 1)
			done := make(chan error, 1)
			go func() { done <- r.RunWithFailure(context.Background(), func(err error) { notified <- err }) }()
			select {
			case err := <-notified:
				if err == nil || (!unexpectedSuccess && !errors.Is(err, failure)) {
					t.Fatalf("failure notification: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("failure hidden behind draining peer")
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("peer not cancelled")
			}
			select {
			case err := <-done:
				t.Fatalf("returned before draining peer: %v", err)
			default:
			}
			release <- struct{}{}
			select {
			case err := <-done:
				if err == nil || (!unexpectedSuccess && !errors.Is(err, failure)) {
					t.Fatalf("lost first failure: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("runtime did not finish")
			}
			if len(notified) != 0 {
				t.Fatal("failure notified more than once")
			}
		})
	}
}

func TestComponentFailureRevokesHTTPReadinessWhileDraining(t *testing.T) {
	db := testdb.New(t)
	api, err := httpapi.New(db, config.HTTP{
		PublicOrigin: "http://localhost", Development: true,
		CSRFKey: bytes.Repeat([]byte{1}, 32), LoginLimit: 10,
		SessionLifetime: time.Hour, SessionIdle: time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	api.AttachRuntime(http.NotFoundHandler())
	api.SetReady(true)
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	check := func(path string, want int) {
		t.Helper()
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s: %d want %d", path, response.StatusCode, want)
		}
	}
	check("/readyz", 200)
	check("/healthz", 200)
	release := make(chan struct{})
	defer close(release)
	notified := make(chan struct{})
	done := make(chan error, 1)
	// The test database deliberately has no outbox table: Ping succeeds while
	// the real required consumer cannot claim work.
	consumer := outbox.NewConsumer(outbox.NewStore(db), "test-worker", "test-consumer", slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := &Runtime{tasks: []func(context.Context) error{
		consumer.Run,
		func(ctx context.Context) error { <-ctx.Done(); <-release; return nil },
	}}
	go func() {
		done <- r.RunWithFailure(context.Background(), func(error) {
			api.SetReady(false)
			close(notified)
		})
	}()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("failure not reported")
	}
	check("/readyz", 503)
	check("/healthz", 200)
	select {
	case <-done:
		t.Fatal("peer finished before it was released")
	default:
	}
	release <- struct{}{}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("failure lost")
		}
	case <-time.After(time.Second):
		t.Fatal("runtime failed to finish")
	}
}

func TestNormalCancellationDrainsWithoutFailureNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	r := &Runtime{tasks: []func(context.Context) error{func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}}
	notified := make(chan error, 1)
	done := make(chan error, 1)
	go func() { done <- r.RunWithFailure(ctx, func(err error) { notified <- err }) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("normal cancellation did not finish")
	}
	if len(notified) != 0 {
		t.Fatal("normal cancellation reported as failure")
	}
}

func TestCancellationPreservesCleanupFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	failure := errors.New("MQTT cleanup budget exceeded")
	r := &Runtime{tasks: []func(context.Context) error{func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return failure
	}}}
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("cleanup failure lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime failed to stop")
	}
}
