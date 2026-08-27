package driver

import (
	"encoding/json"
	"strings"
	"testing"
)

func registerTestDriver(t *testing.T, registry *Registry, version string) {
	t.Helper()
	runtime := validRuntime()
	runtime.Manifest.Version = version
	if err := registry.Register(runtime); err != nil {
		t.Fatalf("register test driver %s: %v", version, err)
	}
}

func validDeviceType() DeviceTypeDefinition {
	return DeviceTypeDefinition{
		TypeKey:                 "test.sensor",
		Version:                 1,
		DisplayName:             "Test sensor",
		Category:                "sensor",
		DriverKey:               "test.device",
		DriverVersionConstraint: "^1.2.0",
		CapabilityProfile: map[string]json.RawMessage{
			"state.read":         json.RawMessage(`{"enabled":true}`),
			"stream.sensor.read": json.RawMessage(`{"sampleRateHz":1}`),
		},
		Status: DeviceTypeActive,
	}
}

func TestDeviceTypeResolvesHighestCompatibleDriver(t *testing.T) {
	drivers := NewRegistry()
	registerTestDriver(t, drivers, "1.2.3")
	registerTestDriver(t, drivers, "1.9.0")
	registerTestDriver(t, drivers, "2.0.0")
	types := NewDeviceTypeRegistry(drivers)
	if err := types.Register(validDeviceType()); err != nil {
		t.Fatalf("register device type: %v", err)
	}
	resolved := types.Resolve("test.sensor", 1)
	if !resolved.Available || resolved.ReadOnly || resolved.Runtime.Manifest.Version != "1.9.0" {
		t.Fatalf("unexpected type resolution: %#v", resolved)
	}
	if err := drivers.SetEnabled("test.device", "1.9.0", false); err != nil {
		t.Fatal(err)
	}
	resolved = types.Resolve("test.sensor", 1)
	if !resolved.Available || resolved.Runtime.Manifest.Version != "1.2.3" {
		t.Fatalf("type did not fall back within compatible range: %#v", resolved)
	}
	if err := drivers.SetEnabled("test.device", "1.2.3", false); err != nil {
		t.Fatal(err)
	}
	resolved = types.Resolve("test.sensor", 1)
	if resolved.Available || !resolved.ReadOnly || resolved.Reason != "DEVICE_DRIVER_UNAVAILABLE" {
		t.Fatalf("missing compatible driver did not degrade safely: %#v", resolved)
	}
}

func TestDeviceTypeRejectsUnknownCapabilityAndDriver(t *testing.T) {
	drivers := NewRegistry()
	registerTestDriver(t, drivers, "1.2.3")
	types := NewDeviceTypeRegistry(drivers)
	definition := validDeviceType()
	definition.CapabilityProfile["device.untrusted.control"] = json.RawMessage(`{}`)
	if err := types.Register(definition); err == nil || !strings.Contains(err.Error(), "not declared by driver") {
		t.Fatalf("unknown capability was accepted: %v", err)
	}
	definition = validDeviceType()
	definition.DriverVersionConstraint = ">=2.0.0"
	if err := types.Register(definition); err == nil || !strings.Contains(err.Error(), "driver is unavailable") {
		t.Fatalf("incompatible driver was accepted: %v", err)
	}
}

func TestVersionConstraints(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		matches    bool
	}{
		{"^1.2.3", "1.9.9", true},
		{"^1.2.3", "2.0.0", false},
		{"~1.2.3", "1.2.8", true},
		{"~1.2.3", "1.3.0", false},
		{">=1.2.0 <2.0.0", "1.5.0", true},
		{"=1.2.3", "1.2.4", false},
		{"*", "9.0.0", true},
	}
	for _, test := range tests {
		rule, err := ParseVersionConstraint(test.constraint)
		if err != nil {
			t.Fatalf("parse %s: %v", test.constraint, err)
		}
		version, err := ParseVersion(test.version)
		if err != nil {
			t.Fatal(err)
		}
		if got := rule.Matches(version); got != test.matches {
			t.Fatalf("constraint %s version %s: got %v", test.constraint, test.version, got)
		}
	}
}
