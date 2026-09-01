package flighthub

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/dji"
)

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
		if capability.Kind == connector.CapabilityAction && (capability.DefaultEnabled || capability.FeatureFlag != FlightHubActionFeatureFlag) {
			t.Fatalf("action capability does not fail closed: %#v", capability)
		}
	}
	for _, domain := range []string{"system", "security", "organization", "project", "device", "control", "flight", "live", "geospatial", "model"} {
		if !coveredDomains[domain] {
			t.Fatalf("endpoint domain %s has no connector capability", domain)
		}
	}
}
