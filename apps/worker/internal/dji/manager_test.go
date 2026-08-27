package dji

import (
	"context"
	"encoding/json"
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

func (fixture *managedSessionFixture) Events() <-chan SessionEvent { return fixture.events }
func (fixture *managedSessionFixture) Done() <-chan struct{}       { return fixture.done }

func TestAdapterLeaseAllowsOnlyOneActiveWorker(t *testing.T) {
	repository := &leaseRepositoryFixture{lease: AdapterLease{
		AdapterID: 1, ProjectID: 2, BrokerURL: "mqtt://broker.example.test:1883", SecretRef: "secret://adapter/1",
		ConfigJSON: json.RawMessage(`{"topics":["dji/project-2/GW001/#"]}`),
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
