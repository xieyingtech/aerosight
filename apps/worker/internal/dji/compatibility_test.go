package dji

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"aerosight/worker/internal/device"
	"aerosight/worker/internal/driver"
)

func allDJITypes(t *testing.T) *driver.DeviceTypeRegistry {
	t.Helper()
	drivers := driver.NewRegistry()
	if err := RegisterDriver(drivers, func(context.Context, driver.AdapterConfig) error { return nil }); err != nil {
		t.Fatal(err)
	}
	types := driver.NewDeviceTypeRegistry(drivers)
	for _, register := range []func(*driver.DeviceTypeRegistry) error{
		RegisterUnknownDJIDeviceType, RegisterDock2DeviceTypes, RegisterDock3DeviceTypes,
	} {
		if err := register(types); err != nil {
			t.Fatal(err)
		}
	}
	return types
}

func TestUnknownProductFixtureCreatesReadOnlyDeviceWithoutSyntheticControls(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/dji/dock2-unknown-aircraft.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	nodes, err := ExpandDock2Topology("DOCK2-DEMO-UNKNOWN", fixture.Data)
	if err != nil {
		t.Fatal(err)
	}
	var unknown *ProductNode
	for index := range nodes {
		if nodes[index].ExternalID == "UNKNOWN-AIRCRAFT-001" {
			unknown = &nodes[index]
		}
		if nodes[index].ParentExternalID == "UNKNOWN-AIRCRAFT-001" {
			t.Fatalf("unknown aircraft received guessed component devices: %+v", nodes[index])
		}
	}
	if unknown == nil || unknown.TypeKey != UnknownDeviceTypeKey || !unknown.ReadOnly || unknown.CompatibilityReason != "DJI_PRODUCT_ENUM_UNKNOWN" {
		t.Fatalf("unknown enum did not degrade read-only: %+v", unknown)
	}
	resolved := allDJITypes(t).Resolve(UnknownDeviceTypeKey, 1)
	effective, err := device.CalculateEffectiveCapabilities(resolved, device.CapabilityReport{}, device.CapabilityReport{})
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Capabilities) != 1 || effective.Capabilities[0].Code != "state.read" || effective.Capabilities[0].Kind == driver.CapabilityCommand {
		t.Fatalf("unknown DeviceType exposed non-diagnostic capabilities: %+v", effective)
	}
}

func TestUnvalidatedFirmwareDisablesEveryCommandButPreservesReadOnlyData(t *testing.T) {
	types := allDJITypes(t)
	resolved := types.Resolve("dji.matrice3td", 1)
	compatibility := CheckProductCompatibility("dock2", ProductKey{Domain: 0, Type: 91, Subtype: 1}, "99.99.99.99")
	if compatibility.State != "degraded" || !compatibility.ReadOnly || compatibility.Reason != "DJI_FIRMWARE_NOT_VALIDATED" {
		t.Fatalf("unknown firmware was treated as compatible: %+v", compatibility)
	}
	report := RestrictCapabilitiesForCompatibility(resolved, compatibility)
	effective, err := device.CalculateEffectiveCapabilities(resolved, report, device.CapabilityReport{})
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range effective.Capabilities {
		if capability.Kind == driver.CapabilityCommand && capability.Available {
			t.Fatalf("unknown firmware exposed control capability: %+v", capability)
		}
	}
	assertStream(t, effective, "telemetry.primary", driver.StreamTelemetry)

	validated := CheckProductCompatibility("dock2", ProductKey{Domain: 0, Type: 91, Subtype: 1}, "14.03.07.01")
	if validated.State != "compatible" || validated.ReadOnly {
		t.Fatalf("validated firmware was degraded: %+v", validated)
	}
	validatedEffective, err := device.CalculateEffectiveCapabilities(resolved, RestrictCapabilitiesForCompatibility(resolved, validated), device.CapabilityReport{})
	if err != nil {
		t.Fatal(err)
	}
	assertCapabilityKind(t, validatedEffective, "flight.return_home", driver.CapabilityCommand)
}

func TestUnknownGatewayEnumRemainsVisibleAndReadOnly(t *testing.T) {
	payload := json.RawMessage(`{"domain":"3","type":999,"sub_type":9,"thing_version":"future"}`)
	nodes, err := ExpandDock3Topology("FUTURE-DOCK-001", payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].TypeKey != UnknownDeviceTypeKey || !nodes[0].ReadOnly {
		t.Fatalf("unknown gateway was hidden or given guessed children: %+v", nodes)
	}
	compatibility := CheckProductCompatibility("dock3", ProductKey{Domain: 3, Type: 999, Subtype: 9}, "future")
	if compatibility.Reason != "DJI_PRODUCT_ENUM_UNKNOWN" || compatibility.TypeKey != UnknownDeviceTypeKey {
		t.Fatalf("missing compatibility diagnostic: %+v", compatibility)
	}
}
