package dji

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type leaseRepositoryFixture struct {
	mu       sync.Mutex
	lease    AdapterLease
	owner    string
	statuses []string
}

func TestManagerShutdownWaitsForMQTTBeforeReleasingLease(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "session closes", true: "session exceeds cleanup budget"}[timeout], func(t *testing.T) {
			repository := &leaseRepositoryFixture{lease: AdapterLease{
				AdapterID: 1, ProjectID: 2, BrokerURL: "mqtt://broker.example.test:1883",
				ConfigJSON: json.RawMessage(`{"topics":["dji/project-2/GW001/#"],"gatewaySerials":["GW001"]}`),
			}}
			sessionDone := make(chan struct{})
			closeSession := sync.OnceFunc(func() { close(sessionDone) })
			defer closeSession()
			started := make(chan struct{})
			cancelled := make(chan struct{})
			connector := func(ctx context.Context, _ MQTTConfig, _ MQTTMessageHandler) (ManagedSession, error) {
				close(started)
				go func() { <-ctx.Done(); close(cancelled) }()
				return &managedSessionFixture{events: make(chan SessionEvent), done: sessionDone}, nil
			}
			manager := NewAdapterManager(repository, secretFixture{credentials: MQTTCredentials{Username: "worker", Password: "password"}}, connector, nil, "worker-a", nil)
			manager.shutdownTimeout = 100 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- manager.Run(ctx) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("session did not start")
			}
			cancel()
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("session not cancelled")
			}
			repository.mu.Lock()
			owner := repository.owner
			repository.mu.Unlock()
			if owner != "worker-a" {
				t.Fatal("lease released before MQTT stopped")
			}
			if !timeout {
				closeSession()
			}
			select {
			case err := <-done:
				if timeout && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("cleanup timeout: %v", err)
				}
				if !timeout && err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup exceeded budget")
			}
			repository.mu.Lock()
			owner = repository.owner
			repository.mu.Unlock()
			if timeout && owner != "worker-a" {
				t.Fatal("unclosed session lost its lease")
			}
			if !timeout && owner != "" {
				t.Fatal("closed session retained its lease")
			}
		})
	}
}

func (fixture *leaseRepositoryFixture) Claim(_ context.Context, owner string, _ int, _ time.Duration) ([]AdapterLease, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.owner != "" {
		return nil, nil
	}
	fixture.owner = owner
	fixture.lease.Epoch++
	return []AdapterLease{fixture.lease}, nil
}

func (fixture *leaseRepositoryFixture) Renew(_ context.Context, lease AdapterLease, owner string, _ time.Duration) (bool, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.owner == owner && fixture.lease.AdapterID == lease.AdapterID && fixture.lease.Epoch == lease.Epoch, nil
}

func (fixture *leaseRepositoryFixture) Release(_ context.Context, lease AdapterLease, owner string) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.owner == owner && fixture.lease.Epoch == lease.Epoch {
		fixture.owner = ""
	}
	return nil
}

func (fixture *leaseRepositoryFixture) UpdateStatus(_ context.Context, _ AdapterLease, owner, status, _ string) error {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.owner == owner {
		fixture.statuses = append(fixture.statuses, status)
	}
	return nil
}

type managedSessionFixture struct {
	events chan SessionEvent
	done   chan struct{}
}

type blockedReleaseRepository struct{ *leaseRepositoryFixture }

func (repository blockedReleaseRepository) Release(ctx context.Context, _ AdapterLease, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestManagerShutdownBoundsLeaseRelease(t *testing.T) {
	repository := blockedReleaseRepository{&leaseRepositoryFixture{owner: "worker-a"}}
	manager := NewAdapterManager(repository, secretFixture{}, func(context.Context, MQTTConfig, MQTTMessageHandler) (ManagedSession, error) {
		t.Error("cancelled manager started a new session")
		return nil, errors.New("unexpected connect")
	}, nil, "worker-a", nil)
	manager.shutdownTimeout = 20 * time.Millisecond
	closed := make(chan struct{})
	close(closed)
	manager.active[1] = activeAdapter{session: &managedSessionFixture{done: closed}, cancel: func() {}, allDone: closed}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("release deadline: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease release exceeded cleanup budget")
	}
}

func (fixture *managedSessionFixture) Events() <-chan SessionEvent { return fixture.events }
func (fixture *managedSessionFixture) Done() <-chan struct{}       { return fixture.done }

func TestAdapterLeaseAllowsOnlyOneActiveWorker(t *testing.T) {
	repository := &leaseRepositoryFixture{lease: AdapterLease{
		AdapterID: 1, ProjectID: 2, BrokerURL: "mqtt://broker.example.test:1883",
		ConfigJSON: json.RawMessage(`{"topics":["dji/project-2/GW001/#"],"gatewaySerials":["GW001"]}`),
	}}
	var mu sync.Mutex
	starts := map[string]int{}
	connectorFor := func(owner string) SessionConnector {
		return func(ctx context.Context, _ MQTTConfig, _ MQTTMessageHandler) (ManagedSession, error) {
			mu.Lock()
			starts[owner]++
			mu.Unlock()
			session := &managedSessionFixture{events: make(chan SessionEvent, 1), done: make(chan struct{})}
			session.events <- SessionEvent{State: "connected", Code: "DJI_MQTT_READY"}
			go func() { <-ctx.Done(); close(session.done) }()
			return session, nil
		}
	}
	resolver := secretFixture{credentials: MQTTCredentials{Username: "worker", Password: "password"}}
	first := NewAdapterManager(repository, resolver, connectorFor("worker-a"), nil, "worker-a", nil)
	second := NewAdapterManager(repository, resolver, connectorFor("worker-b"), nil, "worker-b", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := first.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if starts["worker-a"] != 1 || starts["worker-b"] != 0 {
		t.Fatalf("adapter was not single-active: %+v", starts)
	}
}
