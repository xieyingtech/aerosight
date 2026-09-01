package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type resourceStoreFixture struct {
	mu      sync.Mutex
	states  map[string]connector.ResourceSyncUpdate
	devices []connector.ManagedConnectorDevice
}

func (store *resourceStoreFixture) LoadResourceSyncState(_ context.Context, _ connector.Instance, kind string) (connector.ResourceSyncUpdate, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	state, ok := store.states[kind]
	return state, ok, nil
}

func (store *resourceStoreFixture) SaveResourceSyncState(_ context.Context, _ connector.Instance, update connector.ResourceSyncUpdate) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.states == nil {
		store.states = map[string]connector.ResourceSyncUpdate{}
	}
	store.states[update.Kind] = update
	return nil
}

func (store *resourceStoreFixture) ListManagedDevices(context.Context, connector.Instance) ([]connector.ManagedConnectorDevice, error) {
	return append([]connector.ManagedConnectorDevice(nil), store.devices...), nil
}

func (store *resourceStoreFixture) state(kind string) connector.ResourceSyncUpdate {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.states[kind]
}

type resourceClientFixture struct {
	mu            sync.Mutex
	stateCalls    []string
	stateErr      error
	hmsErr        error
	autoRecordErr error
}

func (client *resourceClientFixture) GetDeviceState(_ context.Context, _, _, serial string) (DeviceStateSnapshot, error) {
	client.mu.Lock()
	client.stateCalls = append(client.stateCalls, serial)
	client.mu.Unlock()
	if client.stateErr != nil {
		return DeviceStateSnapshot{}, client.stateErr
	}
	model := DeviceModel{Key: "0-91-1", Class: "drone"}
	if serial == "DOCK_REDACTED" {
		model = DeviceModel{Key: "3-2-0", Class: "airport"}
	}
	return DeviceStateSnapshot{SN: serial, Model: model, State: map[string]json.RawMessage{
		"longitude": json.RawMessage(`120`), "latitude": json.RawMessage(`30`), "mode_code": json.RawMessage(`0`),
	}}, nil
}

func (client *resourceClientFixture) ListDeviceHMS(context.Context, string, string, []string) ([]DeviceHMS, error) {
	return []DeviceHMS{}, client.hmsErr
}

func (client *resourceClientFixture) ListHistoricalTopologies(context.Context, string, string) ([]HistoricalTopology, error) {
	return []HistoricalTopology{}, nil
}

func (client *resourceClientFixture) GetAutoRecordingConfig(context.Context, string, string) (AutoRecordingConfig, error) {
	if client.autoRecordErr != nil {
		return AutoRecordingConfig{}, client.autoRecordErr
	}
	return AutoRecordingConfig{ID: 1, ProjectID: "PROJECT_REDACTED", Items: []AutoRecordingItem{}}, nil
}

func (client *resourceClientFixture) calls() []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]string(nil), client.stateCalls...)
}

type resourceSinkFixture struct {
	stateApplied chan DeviceStatePoll
	mu           sync.Mutex
	health       int
	states       int
}

func (sink *resourceSinkFixture) ApplyDeviceState(_ context.Context, _ connector.Instance, poll DeviceStatePoll) error {
	sink.mu.Lock()
	sink.states++
	sink.mu.Unlock()
	if sink.stateApplied != nil {
		sink.stateApplied <- poll
	}
	return nil
}

func (sink *resourceSinkFixture) stateCount() int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.states
}

func (sink *resourceSinkFixture) ApplyHealth(context.Context, connector.Instance, HealthPoll) error {
	sink.mu.Lock()
	sink.health++
	sink.mu.Unlock()
	return nil
}

type blockingInventoryRunner struct {
	started chan struct{}
	release chan struct{}
}

type countingInventoryRunner struct{ calls int }

func (runner *countingInventoryRunner) Run(context.Context, connector.Instance, connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	runner.calls++
	return connector.SyncApplyResult{RunID: int64(runner.calls), Discovered: 2}, nil
}

func (runner blockingInventoryRunner) Run(context.Context, connector.Instance, connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	close(runner.started)
	<-runner.release
	return connector.SyncApplyResult{Discovered: 2}, nil
}

func resourceStreamInstance() connector.Instance {
	return connector.Instance{ID: 7, ProjectID: 3, ConnectorKey: ConnectorKey, Version: ConnectorVersion, DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"测试项目"}`)}
}

func TestResourceStreamsRunInsideLeaseWithoutSlowInventoryOrHMSBlockingPosition(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{{DeviceID: 1, Serial: "AIRCRAFT_REDACTED", Online: true}}}
	client := &resourceClientFixture{hmsErr: errors.New("HMS temporarily unavailable")}
	sink := &resourceSinkFixture{stateApplied: make(chan DeviceStatePoll, 1)}
	streamErrors := make(chan string, 2)
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now }, OnError: func(kind string, _ error) { streamErrors <- kind },
	})
	if err != nil {
		t.Fatal(err)
	}
	inventory := blockingInventoryRunner{started: make(chan struct{}), release: make(chan struct{})}
	runner, err := NewConcurrentResourceRunner(inventory, coordinator)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(context.Background(), resourceStreamInstance(), connector.DiscoveryPoll)
		done <- runErr
	}()
	<-inventory.started
	select {
	case poll := <-sink.stateApplied:
		if poll.Mapped.Position == nil || poll.Mapped.Position.Validity != "valid" {
			t.Fatalf("position stream mapped invalid state: %#v", poll)
		}
	case <-time.After(time.Second):
		t.Fatal("slow inventory or HMS blocked the position stream")
	}
	close(inventory.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state := store.state("device-state"); state.Status != "idle" || state.AttemptCount != 0 {
		t.Fatalf("device state stream=%#v", state)
	}
	if health := store.state("health"); health.Status != "backoff" || health.AttemptCount != 1 || health.LastErrorCode != "stream_failed" {
		t.Fatalf("health stream=%#v", health)
	}
	select {
	case kind := <-streamErrors:
		if kind != "health" {
			t.Fatalf("unexpected stream error %q", kind)
		}
	default:
		t.Fatal("health failure did not reach safe diagnostics")
	}
}

func TestDeviceStateStreamUsesAdaptiveIntervalsAndRecoversPersistedBackoff(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{
		{DeviceID: 1, Serial: "AIRCRAFT_REDACTED", Online: true},
		{DeviceID: 2, Serial: "DOCK_REDACTED", Online: false},
	}}
	client := &resourceClientFixture{}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	coordinator.Run(context.Background(), instance)
	if calls := client.calls(); len(calls) != 2 {
		t.Fatalf("initial adaptive poll calls=%v", calls)
	}
	now = now.Add(15 * time.Second)
	coordinator.Run(context.Background(), instance)
	if calls := client.calls(); len(calls) != 3 || calls[2] != "AIRCRAFT_REDACTED" {
		t.Fatalf("online/offline adaptive poll calls=%v", calls)
	}

	client.stateErr = &APIError{SafeCode: "rate_limited", Retryable: true}
	now = now.Add(15 * time.Second)
	appliedBeforeFailure := sink.stateCount()
	coordinator.Run(context.Background(), instance)
	failed := store.state("device-state")
	if failed.Status != "backoff" || failed.AttemptCount != 1 || failed.LastErrorCode != "rate_limited" || failed.NextAttemptAt == nil {
		t.Fatalf("persisted backoff=%#v", failed)
	}
	if sink.stateCount() != appliedBeforeFailure {
		t.Fatal("failed poll refreshed the previous device state")
	}
	callCount := len(client.calls())
	now = now.Add(500 * time.Millisecond)
	coordinator.Run(context.Background(), instance)
	if len(client.calls()) != callCount {
		t.Fatal("persisted backoff was ignored")
	}
	client.stateErr = nil
	now = failed.NextAttemptAt.Add(time.Millisecond)
	coordinator.Run(context.Background(), instance)
	if recovered := store.state("device-state"); recovered.Status != "idle" || recovered.AttemptCount != 0 {
		t.Fatalf("stream did not recover from backoff: %#v", recovered)
	}
}

func TestInventoryUsesIndependentPersistentCadence(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}}
	inventory := &countingInventoryRunner{}
	runner, err := NewScheduledInventoryRunner(inventory, store, 10*time.Minute, 5*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	instance := resourceStreamInstance()
	first, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 1 || first.RunID != 1 {
		t.Fatalf("first inventory=%#v calls=%d err=%v", first, inventory.calls, err)
	}
	now = now.Add(15 * time.Second)
	skipped, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 1 || skipped.RunID != 0 {
		t.Fatalf("inventory cadence did not skip=%#v calls=%d err=%v", skipped, inventory.calls, err)
	}
	now = now.Add(10 * time.Minute)
	second, err := runner.Run(context.Background(), instance, connector.DiscoveryPoll)
	if err != nil || inventory.calls != 2 || second.RunID != 2 {
		t.Fatalf("inventory did not resume=%#v calls=%d err=%v", second, inventory.calls, err)
	}
}
