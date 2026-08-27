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

func dock2Registry(t *testing.T) *driver.DeviceTypeRegistry {
	t.Helper()
	drivers := driver.NewRegistry()
	if err := RegisterDriver(drivers, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		t.Fatal(err)
	}
	types := driver.NewDeviceTypeRegistry(drivers)
	if err := RegisterDock2DeviceTypes(types); err != nil {
		t.Fatal(err)
	}
	return types
}

func TestDock2M3TDFixtureMapsDriverTopologyStatusCommandsAndStreams(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/dji/dock2-m3td-topology.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	nodes, err := ExpandDock2Topology("DOCK2-DEMO-001", fixture.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 6 {
		t.Fatalf("expected dock, aircraft, three cameras and environment sensor, got %+v", nodes)
	}
	wantTypes := map[string]string{
		"DOCK2-DEMO-001":              "dji.dock2",
		"DOCK2-DEMO-001:camera:0":     "dji.dock2.camera",
		"DOCK2-DEMO-001:environment":  "dji.dock2.environment-sensor",
		"M3TD-DEMO-001":               "dji.matrice3td",
		"M3TD-DEMO-001:camera:0":      "dji.matrice3td.camera",
		"M3TD-DEMO-001:vision-assist": "dji.matrice3.vision-assist",
	}
	types := dock2Registry(t)
	observedAt := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	for _, node := range nodes {
		if wantTypes[node.ExternalID] != node.TypeKey {
			t.Fatalf("unexpected product mapping for %s: %+v", node.ExternalID, node)
		}
		if node.ExternalID != "DOCK2-DEMO-001" && (node.ParentExternalID == "" || node.Relation == "") {
			t.Fatalf("topology child has no device relation: %+v", node)
		}
		resolved := types.Resolve(node.TypeKey, 1)
		if !resolved.Available || resolved.Runtime.Manifest.DriverKey != DriverKey {
			t.Fatalf("device type did not bind DJI Driver: %+v", resolved)
		}
		effective, err := device.CalculateEffectiveCapabilities(resolved, device.CapabilityReport{}, device.CapabilityReport{})
		if err != nil {
			t.Fatal(err)
		}
		if len(effective.Capabilities) == 0 {
			t.Fatalf("device has no mapped capabilities: %+v", node)
		}
		projection, err := device.ApplyStatusObservation(device.StatusProjection{}, device.StatusObservation{
			ObservedAt: observedAt, ReceivedAt: observedAt.Add(time.Second), RawReference: "dji/fixture/status",
		}, 30*time.Second)
		if err != nil || projection.Status != device.StatusOnline {
			t.Fatalf("device did not use unified status projection: node=%+v projection=%+v err=%v", node, projection, err)
		}
	}

	dockCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.dock2", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertCapabilityKind(t, dockCapabilities, "dock.debug.control", driver.CapabilityCommand)
	assertCapabilityKind(t, dockCapabilities, "mission.execute", driver.CapabilityCommand)
	for _, capability := range dockCapabilities.Capabilities {
		if capability.Code == "dock.debug.control" && capability.Parameters["productFamily"] != "dock2" {
			t.Fatalf("Dock 2 command profile lost its product boundary: %+v", capability)
		}
	}
	aircraftCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.matrice3td", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertCapabilityKind(t, aircraftCapabilities, "flight.return_home", driver.CapabilityCommand)
	assertStream(t, aircraftCapabilities, "telemetry.primary", driver.StreamTelemetry)
	cameraCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.matrice3td.camera", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertStream(t, cameraCapabilities, "video.primary", driver.StreamVideo)
	assertStream(t, cameraCapabilities, "sensor.primary", driver.StreamSensor)
	sensorCapabilities, _ := device.CalculateEffectiveCapabilities(types.Resolve("dji.dock2.environment-sensor", 1), device.CapabilityReport{}, device.CapabilityReport{})
	assertStream(t, sensorCapabilities, "sensor.primary", driver.StreamSensor)
}

func assertCapabilityKind(t *testing.T, capabilities device.EffectiveCapabilities, code string, kind driver.CapabilityKind) {
	t.Helper()
	for _, capability := range capabilities.Capabilities {
		if capability.Code == code && capability.Kind == kind && capability.Available {
			return
		}
	}
	t.Fatalf("missing available %s capability %s: %+v", kind, code, capabilities.Capabilities)
}

func assertStream(t *testing.T, capabilities device.EffectiveCapabilities, channel string, dataType driver.StreamDataType) {
	t.Helper()
	for _, stream := range capabilities.Streams {
		if stream.ChannelKey == channel && stream.DataType == dataType && stream.Available {
			return
		}
	}
	t.Fatalf("missing available %s stream %s: %+v", dataType, channel, capabilities.Streams)
}

func TestDock2OfficialProductEnumerationsStayDistinct(t *testing.T) {
	tests := []struct {
		key     ProductKey
		typeKey string
	}{
		{ProductKey{3, 2, 0}, "dji.dock2"},
		{ProductKey{0, 91, 0}, "dji.matrice3d"},
		{ProductKey{0, 91, 1}, "dji.matrice3td"},
		{ProductKey{1, 80, 0}, "dji.matrice3d.camera"},
		{ProductKey{1, 81, 0}, "dji.matrice3td.camera"},
	}
	for _, fixture := range tests {
		product, exists := ResolveDock2Product(fixture.key)
		if !exists || product.TypeKey != fixture.typeKey {
			t.Fatalf("official product enum %+v mapped as %+v", fixture.key, product)
		}
		if product.Category == "dock" || product.Category == "aircraft" {
			if product.ValidatedFirmware != "14.03.07.01" {
				t.Fatalf("product %s does not retain the validated firmware fixture: %+v", product.TypeKey, product)
			}
		}
	}
}
