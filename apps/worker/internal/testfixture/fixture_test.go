package testfixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSharedAirGroundFixture(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve fixture test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "../../../../test/fixtures/air-ground-projects.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var fixture Fixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Projects) != 2 {
		t.Fatalf("expected two projects, got %d", len(fixture.Projects))
	}

	projectKeys := map[string]bool{}
	deviceIDs := map[string]bool{}
	deviceTypes := map[string]bool{}
	for _, project := range fixture.Projects {
		if projectKeys[project.Key] || project.Owner.Email == "" || len(project.Devices) == 0 {
			t.Fatalf("invalid isolated project fixture: %#v", project)
		}
		projectKeys[project.Key] = true
		for _, device := range project.Devices {
			if deviceIDs[device.ExternalID] || len(device.Capabilities) == 0 {
				t.Fatalf("invalid simulated device fixture: %#v", device)
			}
			deviceIDs[device.ExternalID] = true
			deviceTypes[device.Type] = true
		}
	}
	if !deviceTypes["drone"] || !deviceTypes["ground_robot"] {
		t.Fatalf("fixture must contain drone and ground robot, got %#v", deviceTypes)
	}
}
