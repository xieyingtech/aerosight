package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTelemetryValidationSupportsBatchesAndRejectsInvalidData(t *testing.T) {
	sequence := int64(1)
	valid := Telemetry{
		ProjectID: 1, TeamID: 2, AdapterID: 3, DeviceID: 4,
		EventID: "event-1", Type: "pose", Sequence: &sequence,
		CapturedAt: time.Now().UTC(), ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"battery":88}`), Quality: json.RawMessage(`{}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.EventID = ""
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "eventId") {
		t.Fatalf("expected missing event error, got %v", err)
	}
	invalid = valid
	invalid.Payload = json.RawMessage(`{`)
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("expected JSON error, got %v", err)
	}
}

func TestTimingClassification(t *testing.T) {
	received := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		captured time.Time
		quality  string
		validity string
	}{
		{received.Add(-time.Second), "trusted", "valid"},
		{received.Add(-3 * time.Minute), "trusted", "late"},
		{received.Add(-11 * time.Minute), "uncertain", "late"},
		{received.Add(11 * time.Minute), "uncertain", "degraded"},
	}
	for _, test := range tests {
		quality, validity := classifyTiming(test.captured, received, 10*time.Minute, 2*time.Minute)
		if quality != test.quality || validity != test.validity {
			t.Fatalf("captured=%s got %s/%s want %s/%s", test.captured, quality, validity, test.quality, test.validity)
		}
	}
}

func TestUnknownCRSIsNotPublishedAsWGS84(t *testing.T) {
	tests := []struct {
		crs       string
		supported bool
		quality   string
	}{
		{"EPSG:4326", true, "usable"},
		{"urn:ogc:def:crs:EPSG::4326", true, "usable"},
		{"EPSG:4490", false, "unusable"},
		{"vendor:local-grid", false, "unusable"},
	}
	for _, test := range tests {
		supported, quality := classifySpatialReference(test.crs)
		if supported != test.supported || quality != test.quality {
			t.Fatalf("crs=%s got supported=%v quality=%s", test.crs, supported, quality)
		}
	}
}
