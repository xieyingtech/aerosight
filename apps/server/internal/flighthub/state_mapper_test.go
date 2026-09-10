package flighthub

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func mappedFixtureState(t *testing.T, name, serial string) MappedDeviceState {
	t.Helper()
	item := loadDeviceFixture(t)[name]
	snapshot, err := deviceFixtureClient(t, item).GetDeviceState(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", serial)
	if err != nil {
		t.Fatal(err)
	}
	return MapDeviceState(snapshot)
}

func TestVersionedStateMapperFieldMatrixForDock2AndM3TD(t *testing.T) {
	dock := mappedFixtureState(t, "state-dock2", "DOCK_REDACTED_01")
	aircraft := mappedFixtureState(t, "state-m3td", "AIRCRAFT_REDACTED_01")
	tests := []struct {
		name   string
		state  MappedDeviceState
		assert func(MappedDeviceState) bool
	}{
		{"dock position", dock, func(value MappedDeviceState) bool {
			return value.Position != nil && value.Position.Validity == "valid" && value.Position.HeightMeters != nil
		}},
		{"dock mode", dock, func(value MappedDeviceState) bool { return value.Mode == "0" }},
		{"dock network", dock, func(value MappedDeviceState) bool { return value.Network != nil && value.Network.Quality == "4" }},
		{"dock battery", dock, func(value MappedDeviceState) bool { return value.Battery != nil && value.Battery.StoreMode == "1" }},
		{"dock environment", dock, func(value MappedDeviceState) bool {
			return value.Environment != nil && value.Environment.TemperatureCelsius != nil
		}},
		{"dock live", dock, func(value MappedDeviceState) bool {
			return value.Live != nil && value.Live.Available && !value.Live.Active
		}},
		{"dock stream channel", dock, func(value MappedDeviceState) bool {
			return reflect.DeepEqual(value.StreamChannels, []StreamChannelState{{
				CameraIndex: "165-0-7", DisplayName: "Dock 2 Camera", Availability: "available",
			}})
		}},
		{"aircraft position", aircraft, func(value MappedDeviceState) bool {
			return value.Position != nil && value.Position.Validity == "valid" && value.DeviceKind == "aircraft"
		}},
		{"aircraft attitude and heading", aircraft, func(value MappedDeviceState) bool {
			return value.Attitude != nil && value.Attitude.HeadingDegrees != nil && *value.Attitude.HeadingDegrees == 180 && value.Attitude.PitchDegrees != nil && value.Attitude.RollDegrees != nil
		}},
		{"aircraft mode", aircraft, func(value MappedDeviceState) bool { return value.Mode == "14" }},
		{"aircraft battery", aircraft, func(value MappedDeviceState) bool {
			return value.Battery != nil && value.Battery.Percent != nil && *value.Battery.Percent == 76
		}},
		{"aircraft live", aircraft, func(value MappedDeviceState) bool { return value.Live != nil && value.Live.Active }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.state.MapperVersion != StateMapperVersion || !test.state.KnownModel || !test.assert(test.state) {
				t.Fatalf("mapped state=%#v", test.state)
			}
		})
	}
	if dock.Position.CoordinateReference != "unverified" || aircraft.Position.CoordinateReference != "unverified" {
		t.Fatalf("coordinate reference must remain explicit pending field acceptance: dock=%#v aircraft=%#v", dock.Position, aircraft.Position)
	}
	if len(aircraft.StreamChannels) != 0 {
		t.Fatalf("aircraft state must not invent a camera channel without an official camera index: %#v", aircraft.StreamChannels)
	}
}

func TestStateMapperRejectsInvalidCoordinatesWithoutDiscardingOtherTelemetry(t *testing.T) {
	mapped := mappedFixtureState(t, "state-invalid-coordinates", "INVALID_COORD_REDACTED")
	if mapped.Position == nil || mapped.Position.Validity != "invalid" || mapped.Position.Reason != "coordinate_out_of_range" {
		t.Fatalf("invalid coordinate mapping=%#v", mapped.Position)
	}
	if mapped.Mode != "" || !mapped.KnownModel {
		t.Fatalf("known model metadata was lost: %#v", mapped)
	}

	snapshot := DeviceStateSnapshot{SN: "ZERO_REDACTED", Model: DeviceModel{Key: "0-91-1", Class: "drone"}, State: map[string]json.RawMessage{
		"longitude": json.RawMessage(`0`), "latitude": json.RawMessage(`0`), "mode_code": json.RawMessage(`"standby"`),
	}}
	zero := MapDeviceState(snapshot)
	if zero.Position == nil || zero.Position.Reason != "coordinate_zero_sentinel" || zero.Mode != "standby" {
		t.Fatalf("zero sentinel mapping=%#v", zero)
	}
}

func TestUnknownModelAndFieldsOnlyProduceDiagnosticsAndNeverExpandCapabilities(t *testing.T) {
	unknown := mappedFixtureState(t, "state-unknown-model", "UNKNOWN_REDACTED")
	if unknown.KnownModel || len(unknown.CapabilityEvidence) != 0 || unknown.Position != nil || len(unknown.StreamChannels) != 0 || !reflect.DeepEqual(unknown.Diagnostics, []StateFieldDiagnostic{{Name: "future_field", JSONType: "object"}}) {
		t.Fatalf("unknown model mapping=%#v", unknown)
	}

	item := loadDeviceFixture(t)["state-dock2"]
	snapshot, err := deviceFixtureClient(t, item).GetDeviceState(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "DOCK_REDACTED_01")
	if err != nil {
		t.Fatal(err)
	}
	baseline := MapDeviceState(snapshot)
	snapshot.State["future_control_capability"] = json.RawMessage(`{"enabled":true}`)
	withUnknown := MapDeviceState(snapshot)
	if !reflect.DeepEqual(baseline.CapabilityEvidence, withUnknown.CapabilityEvidence) || !reflect.DeepEqual(withUnknown.Diagnostics, []StateFieldDiagnostic{{Name: "future_control_capability", JSONType: "object"}}) {
		t.Fatalf("unknown field changed effective evidence: baseline=%#v mapped=%#v", baseline.CapabilityEvidence, withUnknown)
	}
}

func TestDockStreamChannelDegradesWhenLiveStateIsMissing(t *testing.T) {
	mapped := MapDeviceState(DeviceStateSnapshot{
		SN: "DOCK_REDACTED", Model: DeviceModel{Key: "3-2-0", Class: "airport"},
		State: map[string]json.RawMessage{"mode_code": json.RawMessage(`0`)},
	})
	want := []StreamChannelState{{
		CameraIndex: "165-0-7", DisplayName: "Dock 2 Camera", Availability: "degraded",
		AvailabilityReason: "live_status_unavailable",
	}}
	if !reflect.DeepEqual(mapped.StreamChannels, want) {
		t.Fatalf("degraded dock stream channel=%#v", mapped.StreamChannels)
	}
}
