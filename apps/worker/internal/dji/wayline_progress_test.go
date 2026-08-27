package dji

import (
	"encoding/json"
	"testing"
)

func TestWaylineProgressFixtureRetainsAssociationAndProgress(t *testing.T) {
	fixture := json.RawMessage(`{
		"result":0,
		"output":{
			"ext":{"flight_id":"flight-demo-1","current_waypoint_index":3,"wayline_mission_state":6},
			"progress":{"current_step":24,"percent":67},
			"status":"in_progress"
		}
	}`)
	progress, err := DecodeWaylineProgress(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if progress.FlightID != "flight-demo-1" || progress.Percent != 67 || progress.CurrentStep != 24 || progress.CurrentWaypoint != 3 || progress.Status != "in_progress" {
		t.Fatalf("wayline progress lost protocol fields: %+v", progress)
	}
}

func TestWaylineProgressRejectsUnmatchedOrInvalidFixture(t *testing.T) {
	for _, fixture := range []string{
		`{"result":0,"output":{"status":"in_progress","progress":{"percent":10},"ext":{}}}`,
		`{"result":0,"output":{"status":"in_progress","progress":{"percent":101},"ext":{"flight_id":"flight"}}}`,
		`{"output":{"status":"ok","progress":{"percent":100},"ext":{"flight_id":"flight"}}}`,
	} {
		if _, err := DecodeWaylineProgress(json.RawMessage(fixture)); err == nil {
			t.Fatalf("invalid progress accepted: %s", fixture)
		}
	}
}
