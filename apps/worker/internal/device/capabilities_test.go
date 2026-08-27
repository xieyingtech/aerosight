package device

import (
	"encoding/json"
	"testing"

	"aerosight/worker/internal/driver"
)

func boolPointer(value bool) *bool { return &value }

func resolvedCapabilityFixture() driver.ResolvedDeviceType {
	return driver.ResolvedDeviceType{
		Definition: driver.DeviceTypeDefinition{
			TypeKey: "fixture.camera", Version: 1, DisplayName: "Fixture camera", Category: "camera",
			DriverKey: "fixture.driver", DriverVersionConstraint: "^1.0.0", Status: driver.DeviceTypeActive,
			CapabilityProfile: map[string]json.RawMessage{
				"state.read":        json.RawMessage(`{"enabled":true}`),
				"stream.video.read": json.RawMessage(`{"enabled":true,"maxWidth":1920}`),
			},
		},
		Runtime: driver.Runtime{Manifest: driver.Manifest{
			DriverKey: "fixture.driver", Version: "1.0.0", DisplayName: "Fixture driver", Protocols: []string{"fixture"},
			Capabilities: []driver.CapabilityDefinition{
				{Code: "state.read", Kind: driver.CapabilityRead, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"object"}`)},
				{Code: "stream.video.read", Kind: driver.CapabilityStream, Risk: driver.RiskLow, OutputSchema: json.RawMessage(`{"type":"string"}`)},
				{Code: "motion.control", Kind: driver.CapabilityCommand, Risk: driver.RiskCritical, InputSchema: json.RawMessage(`{"type":"object"}`)},
			},
			Streams: []driver.StreamDefinition{{ChannelKey: "camera.main", CapabilityCode: "stream.video.read", DataType: driver.StreamVideo, Schema: json.RawMessage(`{"codec":"h264"}`)}},
		}},
		Available: true,
	}
}

func findCapability(t *testing.T, result EffectiveCapabilities, code string) EffectiveCapability {
	t.Helper()
	for _, capability := range result.Capabilities {
		if capability.Code == code {
			return capability
		}
	}
	t.Fatalf("capability %q not found", code)
	return EffectiveCapability{}
}

func TestEffectiveCapabilitiesIntersectAllLayers(t *testing.T) {
	firmware := CapabilityReport{Authoritative: true, Capabilities: map[string]CapabilityState{
		"state.read":        {Available: boolPointer(true)},
		"stream.video.read": {Available: boolPointer(false), Reason: "CAMERA_BUSY", Parameters: map[string]any{"maxWidth": 1280.0}},
		"vendor.unsafe":     {Available: boolPointer(true)},
	}}
	runtime := CapabilityReport{Capabilities: map[string]CapabilityState{
		"stream.video.read": {Available: boolPointer(true), Parameters: map[string]any{"latencyClass": "interactive"}},
		"runtime.injected":  {Available: boolPointer(true)},
	}}
	result, err := CalculateEffectiveCapabilities(resolvedCapabilityFixture(), firmware, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Capabilities) != 2 {
		t.Fatalf("device type intersection exposed a driver-only capability: %+v", result.Capabilities)
	}
	video := findCapability(t, result, "stream.video.read")
	if video.Available || video.Reason != "CAMERA_BUSY" {
		t.Fatalf("runtime widened a firmware-disabled capability: %+v", video)
	}
	if video.Risk != driver.RiskLow || string(video.OutputSchema) != `{"type":"string"}` {
		t.Fatalf("reports changed immutable driver security metadata: %+v", video)
	}
	if video.Parameters["maxWidth"] != 1280.0 || video.Parameters["latencyClass"] != "interactive" {
		t.Fatalf("capability parameters were not layered: %+v", video.Parameters)
	}
	if len(result.Streams) != 1 || result.Streams[0].ChannelKey != "camera.main" || result.Streams[0].Available {
		t.Fatalf("stable stream did not inherit capability availability: %+v", result.Streams)
	}
	if len(result.Quarantined) != 2 || result.Quarantined[0].Code != "vendor.unsafe" || result.Quarantined[1].Code != "runtime.injected" {
		t.Fatalf("unknown capabilities were not quarantined: %+v", result.Quarantined)
	}
}

func TestAuthoritativeReportRetainsButDisablesMissingCapability(t *testing.T) {
	result, err := CalculateEffectiveCapabilities(resolvedCapabilityFixture(), CapabilityReport{
		Authoritative: true,
		Capabilities:  map[string]CapabilityState{"state.read": {Available: boolPointer(true)}},
	}, CapabilityReport{})
	if err != nil {
		t.Fatal(err)
	}
	video := findCapability(t, result, "stream.video.read")
	if video.Available || video.Reason != "FIRMWARE_CAPABILITY_NOT_REPORTED" {
		t.Fatalf("missing authoritative firmware capability remained available: %+v", video)
	}
}

func TestUnavailableDeviceTypeMakesCapabilitiesReadOnlyUnavailable(t *testing.T) {
	fixture := resolvedCapabilityFixture()
	fixture.Available = false
	fixture.ReadOnly = true
	fixture.Reason = "DEVICE_DRIVER_UNAVAILABLE"
	result, err := CalculateEffectiveCapabilities(fixture, CapabilityReport{}, CapabilityReport{})
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range result.Capabilities {
		if capability.Available || capability.Reason != "DEVICE_DRIVER_UNAVAILABLE" {
			t.Fatalf("unavailable type exposed capability: %+v", capability)
		}
	}
}

func TestInvalidDeviceTypeCapabilityProfileFailsClosed(t *testing.T) {
	fixture := resolvedCapabilityFixture()
	fixture.Definition.CapabilityProfile["state.read"] = json.RawMessage(`{"enabled":"yes"}`)
	if _, err := CalculateEffectiveCapabilities(fixture, CapabilityReport{}, CapabilityReport{}); err == nil {
		t.Fatal("invalid device type capability profile was accepted")
	}
}
