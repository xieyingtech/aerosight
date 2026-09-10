package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type memorySyncStore struct {
	cursor     json.RawMessage
	identities map[string]string
	applied    int
}

func (store *memorySyncStore) CurrentCursor(context.Context, Instance) (json.RawMessage, error) {
	return store.cursor, nil
}

func (store *memorySyncStore) ApplyBatch(
	_ context.Context, _ Instance, _ DiscoveryMode, expected json.RawMessage, batch DiscoveryBatch,
) (SyncApplyResult, error) {
	if !sameJSON(store.cursor, expected) {
		return SyncApplyResult{}, ErrSyncCursorAdvanced
	}
	if sameJSON(store.cursor, batch.Cursor) {
		return SyncApplyResult{RunID: int64(store.applied + 1), ReplayedCursor: true}, nil
	}
	store.applied++
	seen := map[string]struct{}{}
	for _, device := range batch.Devices {
		store.identities[device.ExternalID] = "discovered"
		seen[device.ExternalID] = struct{}{}
	}
	missing := 0
	if batch.CompleteSnapshot {
		for externalID, status := range store.identities {
			if _, present := seen[externalID]; !present && (status == "discovered" || status == "managed") {
				store.identities[externalID] = "missing"
				missing++
			}
		}
	}
	store.cursor = batch.Cursor
	return SyncApplyResult{RunID: int64(store.applied), Discovered: len(batch.Devices), Missing: missing}, nil
}

func syncRuntime(batch *DiscoveryBatch) Runtime {
	runtime := memoryIoTRuntime()
	runtime.DiscoveryHandlers[DiscoveryPoll] = func(context.Context, DiscoveryRequest) (DiscoveryBatch, error) {
		return *batch, nil
	}
	runtime.ScopeFilter = func(_ Instance, device ExternalDevice) bool {
		return strings.HasPrefix(device.ExternalID, "site-a/")
	}
	return runtime
}

func TestSynchronizerDeduplicatesAndMarksMissingOnlyForCompleteSnapshots(t *testing.T) {
	batch := DiscoveryBatch{
		Devices: []ExternalDevice{
			{ExternalID: "site-a/sensor-1", ExternalType: "temperature"},
			{ExternalID: "site-a/sensor-1", ExternalType: "temperature"},
		},
		Cursor: json.RawMessage(`{"sequence":1}`), CompleteSnapshot: true, SourceVersion: "fixture-v1",
	}
	registry := NewRegistry()
	if err := registry.Register(syncRuntime(&batch)); err != nil {
		t.Fatal(err)
	}
	store := &memorySyncStore{
		cursor: json.RawMessage(`{}`), identities: map[string]string{"site-a/old": "managed", "site-a/ignored": "ignored"},
	}
	synchronizer, _ := NewSynchronizer(registry, store)
	instance := Instance{ID: 7, ProjectID: 3, ConnectorKey: "acme.iot-hub", Version: "1.0.0"}
	result, err := synchronizer.Run(context.Background(), instance, DiscoveryPoll)
	if err != nil {
		t.Fatal(err)
	}
	if result.Discovered != 1 || result.Missing != 1 || store.identities["site-a/old"] != "missing" {
		t.Fatalf("unexpected complete sync result: %#v, %#v", result, store.identities)
	}
	if store.identities["site-a/ignored"] != "ignored" {
		t.Fatal("complete scan changed an explicitly ignored identity")
	}

	replayed, err := synchronizer.Run(context.Background(), instance, DiscoveryPoll)
	if err != nil || !replayed.ReplayedCursor || store.applied != 1 {
		t.Fatalf("cursor replay was not idempotent: %#v, %v", replayed, err)
	}

	batch.Cursor = json.RawMessage(`{"sequence":2}`)
	batch.CompleteSnapshot = false
	batch.Devices = []ExternalDevice{{ExternalID: "site-a/sensor-2", ExternalType: "temperature"}}
	store.identities["site-a/still-online"] = "managed"
	if _, err := synchronizer.Run(context.Background(), instance, DiscoveryPoll); err != nil {
		t.Fatal(err)
	}
	if store.identities["site-a/still-online"] != "managed" {
		t.Fatal("incremental discovery incorrectly marked an absent identity missing")
	}
}

func TestSynchronizerRejectsOutOfScopeAndConcurrentCursor(t *testing.T) {
	batch := DiscoveryBatch{
		Devices: []ExternalDevice{{ExternalID: "site-b/sensor-1", ExternalType: "temperature"}},
		Cursor:  json.RawMessage(`{"sequence":1}`),
	}
	registry := NewRegistry()
	if err := registry.Register(syncRuntime(&batch)); err != nil {
		t.Fatal(err)
	}
	store := &memorySyncStore{cursor: json.RawMessage(`{}`), identities: map[string]string{}}
	synchronizer, _ := NewSynchronizer(registry, store)
	instance := Instance{ID: 7, ProjectID: 3, ConnectorKey: "acme.iot-hub", Version: "1.0.0"}
	if _, err := synchronizer.Run(context.Background(), instance, DiscoveryPoll); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("out-of-scope device was accepted: %v", err)
	}
	if store.applied != 0 {
		t.Fatal("out-of-scope batch reached persistence")
	}

	if _, err := store.ApplyBatch(context.Background(), instance, DiscoveryPoll,
		json.RawMessage(`{"sequence":99}`), DiscoveryBatch{}); !errors.Is(err, ErrSyncCursorAdvanced) {
		t.Fatalf("stale cursor was accepted: %v", err)
	}
}
