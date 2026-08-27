package dji

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"aerosight/worker/internal/device"
	"aerosight/worker/internal/driver"
)

func dock3Registry(t *testing.T) *driver.DeviceTypeRegistry {
	t.Helper()
	drivers := driver.NewRegistry()
	if err := RegisterDriver(drivers, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		t.Fatal(err)
	}
	types := driver.NewDeviceTypeRegistry(drivers)
	if err := RegisterDock3DeviceTypes(types); err != nil {
		t.Fatal(err)
	}
	return types
}

func TestDock3M4TDFixtureUsesIndependentTypesCapabilitiesAndStreams(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/dji/dock3-m4td-topology.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	nodes, err := ExpandDock3Topology("DOCK3-DEMO-001", fixture.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 6 {
		t.Fatalf("expected six unified devices, got %+v", nodes)
	}
	wantTypes := map[string]string{
		"DOCK3-DEMO-001":              "dji.dock3",
		"DOCK3-DEMO-001:camera:0":     "dji.dock3.camera",
		"DOCK3-DEMO-001:environment":  "dji.dock3.environment-sensor",
		"M4TD-DEMO-001":               "dji.matrice4td",
		"M4TD-DEMO-001:camera:0":      "dji.matrice4td.camera",
		"M4TD-DEMO-001:vision-assist": "dji.matrice4.vision-assist",
	}
	types := dock3Registry(t)
	for _, node := range nodes {
		if wantTypes[node.ExternalID] != node.TypeKey {
			t.Fatalf("Dock 3 node reused a wrong type: %+v", node)
		}
		resolved := types.Resolve(node.TypeKey, 1)
		if !resolved.Available || resolved.Runtime.Manifest.DriverKey != DriverKey {
			t.Fatalf("Dock 3 type did not bind DJI Driver: %+v", resolved)
		}
		observedAt := time.Date(2026, 8, 27, 9, 10, 0, 0, time.UTC)
		projection, err := device.ApplyStatusObservation(device.StatusProjection{}, device.StatusObservation{
			ObservedAt: observedAt, ReceivedAt: observedAt.Add(time.Second), RawReference: "dji/fixture/dock3-status",
		}, 30*time.Second)
		if err != nil || projection.Status != device.StatusOnline {
			t.Fatalf("Dock 3 node did not use unified status: node=%+v projection=%+v err=%v", node, projection, err)
		}
	}
	if resolved := types.Resolve("dji.dock2", 1); resolved.Available || !resolved.ReadOnly {
		t.Fatalf("Dock 3 registry silently reused Dock 2 type: %+v", resolved)
	}
	dockCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.dock3", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertCapabilityKind(t, dockCapabilities, "dock.debug.control", driver.CapabilityCommand)
	for _, capability := range dockCapabilities.Capabilities {
		if capability.Code == "dock.debug.control" && capability.Parameters["productFamily"] != "dock3" {
			t.Fatalf("Dock 3 command profile reused another generation: %+v", capability)
		}
	}
	aircraftCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.matrice4td", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertCapabilityKind(t, aircraftCapabilities, "flight.return_home", driver.CapabilityCommand)
	assertStream(t, aircraftCapabilities, "telemetry.primary", driver.StreamTelemetry)
	cameraCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.matrice4td.camera", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertStream(t, cameraCapabilities, "video.primary", driver.StreamVideo)
	assertStream(t, cameraCapabilities, "sensor.primary", driver.StreamSensor)
	sensorCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.dock3.environment-sensor", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertStream(t, sensorCapabilities, "sensor.primary", driver.StreamSensor)
}

func TestDock3OfficialEnumsNeverResolveAsDock2Products(t *testing.T) {
	tests := []struct {
		key     ProductKey
		typeKey string
	}{
		{ProductKey{Domain: 3, Type: 3, Subtype: 0}, "dji.dock3"},
		{ProductKey{Domain: 0, Type: 100, Subtype: 0}, "dji.matrice4d"},
		{ProductKey{Domain: 0, Type: 100, Subtype: 1}, "dji.matrice4td"},
		{ProductKey{Domain: 1, Type: 98, Subtype: 0}, "dji.matrice4d.camera"},
		{ProductKey{Domain: 1, Type: 99, Subtype: 0}, "dji.matrice4td.camera"},
	}
	for _, fixture := range tests {
		product, exists := ResolveDock3Product(fixture.key)
		if !exists || product.TypeKey != fixture.typeKey || product.ValidatedFirmware == "14.03.07.01" {
			t.Fatalf("Dock 3 enum %+v mapped incorrectly: %+v", fixture.key, product)
		}
		if legacy, exists := ResolveDock2Product(fixture.key); exists {
			t.Fatalf("Dock 3 enum %+v was accepted by Dock 2 matrix as %+v", fixture.key, legacy)
		}
	}
}
