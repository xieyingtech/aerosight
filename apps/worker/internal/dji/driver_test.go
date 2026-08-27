package dji

import (
	"context"
	"encoding/json"
	"testing"

	"aerosight/worker/internal/driver"
)

func TestDJIDriverManifestRegistersAndBindsDeviceTypes(t *testing.T) {
	registry := driver.NewRegistry()
	if err := RegisterDriver(registry, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		t.Fatal(err)
	}
	runtime, err := registry.Resolve(DriverKey, DriverVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Manifest.Protocols) != 1 || runtime.Manifest.Protocols[0] != "mqtt5" {
		t.Fatalf("DJI runtime does not bind MQTT 5: %+v", runtime.Manifest.Protocols)
	}
	types := driver.NewDeviceTypeRegistry(registry)
	if err := types.Register(driver.DeviceTypeDefinition{
		TypeKey: "dji.fixture", Version: 1, DisplayName: "DJI fixture",
		Category:  "gateway",
		DriverKey: DriverKey, DriverVersionConstraint: "^1.0.0", Status: driver.DeviceTypeActive,
		CapabilityProfile: map[string]json.RawMessage{"state.read": json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	resolved := types.Resolve("dji.fixture", 1)
	if !resolved.Available || resolved.Runtime.Manifest.DriverKey != DriverKey {
		t.Fatalf("DJI DeviceType did not resolve its driver: %+v", resolved)
	}
}
