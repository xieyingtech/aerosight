package flighthub

import (
	"context"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type runtimeProbeStoreFixture struct {
	snapshots []connector.CapabilitySnapshot
	devices   []connector.ManagedConnectorDevice
}

func (store *runtimeProbeStoreFixture) SaveCapabilitySnapshot(_ context.Context, _ connector.Instance, snapshot connector.CapabilitySnapshot) error {
	store.snapshots = append(store.snapshots, snapshot)
	return nil
}

func (store *runtimeProbeStoreFixture) ListCapabilitySnapshots(context.Context, connector.Instance, string, string) ([]connector.CapabilitySnapshot, error) {
	return append([]connector.CapabilitySnapshot(nil), store.snapshots...), nil
}

func (store *runtimeProbeStoreFixture) ListManagedDevices(context.Context, connector.Instance) ([]connector.ManagedConnectorDevice, error) {
	return append([]connector.ManagedConnectorDevice(nil), store.devices...), nil
}

type runtimeProbeClientFixture struct {
	calls int
	input CapabilityProbeInput
}

func (client *runtimeProbeClientFixture) ProbeCapabilities(_ context.Context, input CapabilityProbeInput) ([]CapabilityProbeResult, error) {
	client.calls++
	client.input = input
	results := make([]CapabilityProbeResult, 0, len(Capabilities()))
	for _, capability := range Capabilities() {
		results = append(results, CapabilityProbeResult{
			CapabilityCode: capability.Code, Status: ProbeUnverified, Reason: "fixture",
			Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeUnverified, Acceptance: ProbeUnverified},
		})
	}
	return results, nil
}

func TestCapabilityProbeRunnerPersistsReadEvidenceAndHonorsTTL(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	inner := &countingInventoryRunner{}
	client := &runtimeProbeClientFixture{}
	store := &runtimeProbeStoreFixture{devices: []connector.ManagedConnectorDevice{{DeviceID: 1, Serial: "DEVICE_REDACTED", ModelKey: "dock-model"}}}
	runner, err := NewCapabilityProbeRunner(inner, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, 15*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), resourceStreamInstance(), connector.DiscoveryPoll); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.input.DeviceSerial != "DEVICE_REDACTED" || len(store.snapshots) != len(Capabilities()) {
		t.Fatalf("probe did not persist a complete safe snapshot: calls=%d input=%#v snapshots=%d", client.calls, client.input, len(store.snapshots))
	}
	for _, snapshot := range store.snapshots {
		if snapshot.EvidenceLevel != "live-read" || snapshot.ExpiresAt == nil || !snapshot.ExpiresAt.Equal(now.Add(15*time.Minute)) {
			t.Fatalf("invalid probe evidence TTL: %#v", snapshot)
		}
	}
	if _, err := runner.Run(context.Background(), resourceStreamInstance(), connector.DiscoveryPoll); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || inner.calls != 2 {
		t.Fatalf("current evidence did not suppress upstream probe: probes=%d inner=%d", client.calls, inner.calls)
	}
}
