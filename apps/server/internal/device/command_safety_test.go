package device

import "testing"

func TestActiveFlightTaskControlRetainsConfirmationAndCapabilityChecks(t *testing.T) {
	input := CommandSafetyInput{ProjectID: 1, DeviceProjectID: 1, DeviceID: 2, Capability: "mission.execute", Risk: "high", Availability: "available", Status: "online", ActiveTasks: 1, Confirmation: "CONFIRM 2 mission.execute"}
	if _, err := CheckCommandSafety(input); err == nil {
		t.Fatal("ordinary mission command bypassed active task interlock")
	}
	input.FlightTaskControl = true
	if result, err := CheckCommandSafety(input); err != nil || !result.ActiveTaskOverride {
		t.Fatal("approved existing flight control blocked", result, err)
	}
	input.Confirmation = ""
	if _, err := CheckCommandSafety(input); err == nil {
		t.Fatal("flight control bypassed manual confirmation")
	}
	input.Confirmation = "CONFIRM 2 camera.change"
	input.Capability = "camera.change"
	if _, err := CheckCommandSafety(input); err == nil {
		t.Fatal("override leaked to another capability")
	}
}
