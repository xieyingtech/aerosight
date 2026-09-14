package mission

import (
	"errors"
	"testing"
	"time"
)

func TestStableFailureCodeDoesNotPersistRawErrors(t *testing.T) {
	if got := stableFailureCode(errors.New("TASK_REPORT_INPUT_INVALID: field secret")); got != "TASK_REPORT_INPUT_INVALID" {
		t.Fatalf("stable code changed: %q", got)
	}
	if got := stableFailureCode(errors.New("request to https://secret.example failed")); got != "TASK_STEP_EXECUTION_FAILED" {
		t.Fatalf("raw external error became a task code: %q", got)
	}
}

func TestPausedNonDeviceStepCanResume(t *testing.T) {
	decision, err := Control(Snapshot{RunID: 3, Status: RunPaused, DeviceConnected: false,
		Steps: []Step{{ID: 8, Position: 1, Uses: "report.generate", Status: "paused"}}}, ControlResume, testNow())
	if err != nil || decision.RunStatus != RunRunning || decision.StepStatus != StepPending {
		t.Fatalf("non-device resume failed: decision=%+v err=%v", decision, err)
	}
}

func testNow() time.Time { return time.Unix(0, 0).UTC() }

func TestCancelBusinessWorkflowDoesNotIssueDeviceStop(t *testing.T) {
	for _, status := range []RunStatus{RunQueued, RunRunning, RunPaused} {
		decision, err := Control(Snapshot{RunID: 3, Status: status, Steps: []Step{{Uses: "inspection.observe"}, {Uses: "inspection.detect"}}}, ControlCancel, testNow())
		if err != nil || decision.RunStatus != RunCanceled || decision.IssueCommand != nil || decision.Reason != "operator_canceled_business_run" {
			t.Fatalf("data workflow cancel: %+v %v", decision, err)
		}
	}
	decision, err := Control(Snapshot{RunID: 3, Status: RunRunning, Steps: []Step{{Uses: "device.command"}}}, ControlCancel, testNow())
	if err != nil || decision.IssueCommand == nil {
		t.Fatal("device workflow lost existing control path")
	}
}
