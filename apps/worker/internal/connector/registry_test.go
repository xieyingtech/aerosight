package connector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func memoryIoTRuntime() Runtime {
	return Runtime{
		Manifest: Manifest{
			ConnectorKey: "acme.iot-hub", Version: "1.0.0", DisplayName: "ACME memory IoT hub",
			ConfigSchema:     json.RawMessage(`{"type":"object","properties":{"tenant":{"type":"string"}},"required":["tenant"]}`),
			CredentialSchema: json.RawMessage(`{"type":"object","properties":{"token":{"type":"string"}},"required":["token"]}`),
			DiscoveryModes:   []DiscoveryMode{DiscoveryPoll}, Protocols: []string{"memory"},
			CompatibleDrivers: []string{"acme.sensor"},
			Lease:             LeasePolicy{Duration: 30 * time.Second, RenewBefore: 10 * time.Second},
		},
		DiscoveryHandlers: map[DiscoveryMode]DiscoveryHandler{
			DiscoveryPoll: func(_ context.Context, request DiscoveryRequest) (DiscoveryBatch, error) {
				return DiscoveryBatch{
					Devices: []ExternalDevice{{ExternalID: "sensor-01", ExternalType: "temperature", Attributes: map[string]any{"unit": "celsius"}}},
					Cursor:  json.RawMessage(`{"offset":1}`),
				}, nil
			},
		},
		HealthCheck: func(context.Context, Instance) (Health, error) {
			return Health{Status: "healthy", Details: map[string]any{"transport": "memory"}}, nil
		},
		ScopeFilter: func(Instance, ExternalDevice) bool { return true },
	}
}

func TestRegistryAddsNonDJIConnectorWithoutDeviceSpecificTypes(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(memoryIoTRuntime()); err != nil {
		t.Fatalf("register memory IoT connector: %v", err)
	}
	runtime, err := registry.Resolve("acme.iot-hub", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := runtime.DiscoveryHandlers[DiscoveryPoll](context.Background(), DiscoveryRequest{
		Instance: Instance{ID: 7, ProjectID: 3, ConnectorKey: "acme.iot-hub", Version: "1.0.0"},
		Mode:     DiscoveryPoll,
	})
	if err != nil || len(batch.Devices) != 1 || batch.Devices[0].ExternalID != "sensor-01" {
		t.Fatalf("generic discovery failed: %#v, %v", batch, err)
	}
	if got := registry.Keys(); len(got) != 1 || got[0] != "acme.iot-hub@1.0.0" {
		t.Fatalf("unexpected registry keys: %v", got)
	}
	if err := registry.SetEnabled("acme.iot-hub", "1.0.0", false); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("acme.iot-hub", "1.0.0"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled connector resolved: %v", err)
	}
}

func TestRegistryRejectsIncompleteOrInvalidConnectorManifest(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Runtime)
		want string
	}{
		{name: "config schema", edit: func(runtime *Runtime) { runtime.Manifest.ConfigSchema = json.RawMessage(`{"type":"array"}`) }, want: "object schema"},
		{name: "credential schema", edit: func(runtime *Runtime) { runtime.Manifest.CredentialSchema = nil }, want: "credential schema is required"},
		{name: "mode", edit: func(runtime *Runtime) { runtime.Manifest.DiscoveryModes = []DiscoveryMode{"scan"} }, want: "unsupported"},
		{name: "handler", edit: func(runtime *Runtime) { delete(runtime.DiscoveryHandlers, DiscoveryPoll) }, want: "no handler"},
		{name: "health", edit: func(runtime *Runtime) { runtime.HealthCheck = nil }, want: "health check"},
		{name: "scope", edit: func(runtime *Runtime) { runtime.ScopeFilter = nil }, want: "scope filter"},
		{name: "lease", edit: func(runtime *Runtime) { runtime.Manifest.Lease.RenewBefore = runtime.Manifest.Lease.Duration }, want: "lease renewal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := memoryIoTRuntime()
			test.edit(&runtime)
			if err := NewRegistry().Register(runtime); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestRegistryRejectsDuplicateConnectorRuntime(t *testing.T) {
	registry := NewRegistry()
	runtime := memoryIoTRuntime()
	if err := registry.Register(runtime); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(runtime); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate connector runtime was accepted: %v", err)
	}
}
