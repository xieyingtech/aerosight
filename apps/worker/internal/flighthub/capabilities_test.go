package flighthub

import (
	"encoding/csv"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/dji"
)

func TestRecordingAndShareWritesRemainFailClosedWithoutReleasedEndpoints(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve endpoint manifest")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 11
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows[1:] {
		if (strings.Contains(row[2], "/live-shares") || strings.Contains(row[2], "/streams")) && row[1] != http.MethodGet {
			t.Fatalf("unexpected released recording/share write endpoint: %s %s", row[1], row[2])
		}
	}
	for _, code := range []string{"live.recording.control", "live.share.manage"} {
		if readiness := defaultCapabilityReadiness[code]; readiness.Implemented || readiness.Accepted {
			t.Fatalf("%s must remain unavailable until a released write endpoint exists", code)
		}
	}
}

func TestCapabilitiesIntersectEndpointAndDriverManifests(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve endpoint manifest")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/endpoints.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 11
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	endpointDomains := map[string]bool{}
	for _, row := range rows[1:] {
		endpointDomains[row[5]] = true
	}
	driverCapabilities := map[string]bool{}
	for _, capability := range dji.Manifest().Capabilities {
		driverCapabilities[capability.Code] = true
	}
	coveredDomains := map[string]bool{}
	for _, capability := range Capabilities() {
		for _, domain := range capability.EndpointDomains {
			if !endpointDomains[domain] {
				t.Fatalf("capability %s declared unknown endpoint domain %s", capability.Code, domain)
			}
			coveredDomains[domain] = true
		}
		if capability.DriverCapability != "" && !driverCapabilities[capability.DriverCapability] {
			t.Fatalf("capability %s declared unknown driver capability %s", capability.Code, capability.DriverCapability)
		}
		if capability.Kind == connector.CapabilityAction && (capability.DefaultEnabled || capability.FeatureFlag == "") {
			t.Fatalf("action capability does not fail closed: %#v", capability)
		}
	}
	wantedLiveFlags := map[string]string{
		"live.quality.set": FlightHubLiveQualityFeatureFlag, "live.recording.control": FlightHubLiveRecordingFeatureFlag,
		"live.share.manage": FlightHubLiveShareFeatureFlag, "live.converter.create": FlightHubLiveConverterCreateFeatureFlag,
		"live.converter.toggle": FlightHubLiveConverterToggleFeatureFlag, "live.converter.delete": FlightHubLiveConverterDeleteFeatureFlag,
	}
	seenFlags := map[string]string{}
	for _, capability := range Capabilities() {
		wanted, relevant := wantedLiveFlags[capability.Code]
		if !relevant {
			continue
		}
		if capability.FeatureFlag != wanted {
			t.Fatalf("live capability %s uses feature flag %s, want %s", capability.Code, capability.FeatureFlag, wanted)
		}
		if other := seenFlags[capability.FeatureFlag]; other != "" {
			t.Fatalf("live capabilities %s and %s share feature flag %s", other, capability.Code, capability.FeatureFlag)
		}
		seenFlags[capability.FeatureFlag] = capability.Code
	}
	if len(seenFlags) != len(wantedLiveFlags) {
		t.Fatalf("got %d governed live feature flags, want %d", len(seenFlags), len(wantedLiveFlags))
	}
	for _, domain := range []string{"system", "security", "organization", "project", "device", "control", "flight", "live", "geospatial", "model"} {
		if !coveredDomains[domain] {
			t.Fatalf("endpoint domain %s has no connector capability", domain)
		}
	}
}
