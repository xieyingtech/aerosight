package driver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func noopProtocol(context.Context, AdapterConfig) error { return nil }
func noopCommand(context.Context, Command) (CommandResult, error) {
	return CommandResult{Payload: json.RawMessage(`{"ok":true}`)}, nil
}
func noopStream(context.Context, StreamRequest) error { return nil }

func validRuntime() Runtime {
	return Runtime{
		Manifest: Manifest{
			DriverKey:   "test.device",
			Version:     "1.2.3",
			DisplayName: "Test device driver",
			Protocols:   []string{"mqtt5"},
			Capabilities: []CapabilityDefinition{
				{Code: "state.read", Kind: CapabilityRead, Risk: RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
				{Code: "mission.execute", Kind: CapabilityCommand, Risk: RiskHigh, InputSchema: json.RawMessage(`{"type":"object"}`)},
				{Code: "stream.sensor.read", Kind: CapabilityStream, Risk: RiskLow, OutputSchema: json.RawMessage(`{"type":"number"}`)},
			},
			Streams: []StreamDefinition{
				{ChannelKey: "temperature", CapabilityCode: "stream.sensor.read", DataType: StreamSensor, Schema: json.RawMessage(`{"type":"number"}`), Unit: "celsius"},
			},
		},
		ProtocolHandlers: map[string]ProtocolHandler{"mqtt5": noopProtocol},
		CommandHandlers:  map[string]CommandHandler{"mission.execute": noopCommand},
		StreamHandlers:   map[string]StreamHandler{"temperature": noopStream},
	}
}

func TestRegistryRegistersCompleteRuntime(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(validRuntime()); err != nil {
		t.Fatalf("register valid runtime: %v", err)
	}
	if got := registry.Keys(); len(got) != 1 || got[0] != "test.device@1.2.3" {
		t.Fatalf("unexpected registry keys: %v", got)
	}
	capability, err := registry.ResolveCapability("test.device", "1.2.3", "mission.execute")
	if err != nil || capability.Risk != RiskHigh {
		t.Fatalf("resolve declared capability: %#v, %v", capability, err)
	}
	if _, err := registry.ResolveCapability("test.device", "1.2.3", "dock.cover.control"); err == nil {
		t.Fatal("unknown capability should be rejected")
	}
}

func TestRegistryRejectsIncompleteHandlers(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Runtime)
		want string
	}{
		{name: "protocol", edit: func(runtime *Runtime) { delete(runtime.ProtocolHandlers, "mqtt5") }, want: "protocol"},
		{name: "command", edit: func(runtime *Runtime) { delete(runtime.CommandHandlers, "mission.execute") }, want: "command capability"},
		{name: "stream", edit: func(runtime *Runtime) { delete(runtime.StreamHandlers, "temperature") }, want: "stream channel"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := validRuntime()
			test.edit(&runtime)
			if err := NewRegistry().Register(runtime); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestRegistryRejectsInvalidManifestAndDuplicate(t *testing.T) {
	runtime := validRuntime()
	runtime.Manifest.Version = "latest"
	if err := NewRegistry().Register(runtime); err == nil || !strings.Contains(err.Error(), "semantic versioning") {
		t.Fatalf("invalid semver was accepted: %v", err)
	}

	runtime = validRuntime()
	runtime.Manifest.Streams[0].CapabilityCode = "state.read"
	if err := NewRegistry().Register(runtime); err == nil || !strings.Contains(err.Error(), "non-stream capability") {
		t.Fatalf("invalid stream capability was accepted: %v", err)
	}

	registry := NewRegistry()
	runtime = validRuntime()
	if err := registry.Register(runtime); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(runtime); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate runtime was accepted: %v", err)
	}
}
