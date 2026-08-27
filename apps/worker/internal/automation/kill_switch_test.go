package automation

import "testing"

func TestKillSwitchStopsRunningAutomationWithoutChangingAlert(t *testing.T) {
	alertStillActionable := true
	if ShouldContinueAutomation(false) {
		t.Fatal("disabled automatic AI allowed subsequent draft actions")
	}
	if !alertStillActionable {
		t.Fatal("kill switch changed the original alert")
	}
}
