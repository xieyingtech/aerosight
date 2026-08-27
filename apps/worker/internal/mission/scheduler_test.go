package mission

import (
	"testing"
	"time"
)

func fixture() Snapshot {
	return Snapshot{RunID: 42, Status: RunDispatching, DeviceConnected: true, SupportsReturnHome: true, Steps: []Step{
		{Position: 1, Status: StepPending, CapabilityCode: "flight.navigate", Action: "flight.route",
			FailurePolicy: FailurePolicy{SafeToRetry: true, MaxRetries: 2, Backoff: time.Second, Timeout: 5 * time.Second}},
		{Position: 2, Status: StepPending, CapabilityCode: "camera.capture", Action: "camera.capture",
			FailurePolicy: FailurePolicy{SafeToRetry: false, Timeout: 5 * time.Second}},
	}}
}

func TestSimulatorSuccessAdvancesSequentially(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	start, err := Advance(fixture(), nil, now)
	if err != nil || start.IssueCommand == nil || start.IssueCommand.IdempotencyKey != "task-run:42:step:1" {
		t.Fatalf("first step was not dispatched deterministically: %#v %v", start, err)
	}
	snapshot := fixture()
	snapshot.Status = RunRunning
	snapshot.Steps[0].Status = StepRunning
	snapshot.Steps[0].Command = start.IssueCommand
	ack, err := Advance(snapshot, &Signal{CommandID: start.IssueCommand.ID, Outcome: "ack"}, now.Add(time.Second))
	if err != nil || ack.StepStatus != StepSucceeded || ack.RunStatus != RunRunning {
		t.Fatalf("ack did not advance first step: %#v %v", ack, err)
	}
}

func TestNackFailsOrPausesByPinnedPolicy(t *testing.T) {
	now := time.Now()
	snapshot := fixture()
	command, _ := Advance(snapshot, nil, now)
	snapshot.Status, snapshot.Steps[0].Status, snapshot.Steps[0].Command = RunRunning, StepRunning, command.IssueCommand
	decision, _ := Advance(snapshot, &Signal{CommandID: command.IssueCommand.ID, Outcome: "nack"}, now)
	if decision.RunStatus != RunFailed || decision.StepStatus != StepFailed {
		t.Fatalf("nack did not follow failure policy: %#v", decision)
	}
}

func TestDisconnectPausesAndRestartDoesNotDuplicate(t *testing.T) {
	now := time.Now()
	snapshot := fixture()
	dispatched, _ := Advance(snapshot, nil, now)
	snapshot.Status, snapshot.Steps[0].Status, snapshot.Steps[0].Command = RunRunning, StepRunning, dispatched.IssueCommand
	recovered, _ := Advance(snapshot, nil, now.Add(time.Second))
	if recovered.IssueCommand != nil || recovered.Reason != "awaiting_ack" {
		t.Fatalf("restart duplicated in-flight command: %#v", recovered)
	}
	snapshot.DeviceConnected = false
	disconnected, _ := Advance(snapshot, nil, now.Add(time.Second))
	if disconnected.RunStatus != RunPaused || disconnected.Reason != "device_disconnected" {
		t.Fatalf("disconnect did not pause: %#v", disconnected)
	}
}

func TestSafeTimeoutRetriesSameKeyAndUnsafeTimeoutPauses(t *testing.T) {
	now := time.Now()
	snapshot := fixture()
	dispatched, _ := Advance(snapshot, nil, now)
	snapshot.Status, snapshot.Steps[0].Status, snapshot.Steps[0].Command = RunRunning, StepRunning, dispatched.IssueCommand
	retry, _ := Advance(snapshot, nil, now.Add(6*time.Second))
	if retry.IssueCommand == nil || retry.IssueCommand.IdempotencyKey != dispatched.IssueCommand.IdempotencyKey || retry.IssueCommand.Attempts != 2 {
		t.Fatalf("safe retry changed physical idempotency: %#v", retry)
	}
	unsafe := fixture()
	unsafe.Status = RunRunning
	unsafe.Steps[0].Status = StepSucceeded
	unsafe.Steps[1].Status = StepRunning
	unsafe.Steps[1].Command = &Command{ID: "unsafe", IdempotencyKey: "unsafe-key", Status: CommandSent, Attempts: 1, Deadline: now}
	paused, _ := Advance(unsafe, nil, now.Add(time.Second))
	if paused.RunStatus != RunPaused || !paused.SafetyUnknown || paused.IssueCommand != nil {
		t.Fatalf("unsafe timeout retried or hid uncertainty: %#v", paused)
	}
}

func TestPauseResumeCancelReturnAndEmergency(t *testing.T) {
	now := time.Now()
	snapshot := fixture()
	snapshot.Status = RunRunning
	paused, _ := Control(snapshot, ControlPause, now)
	if paused.RunStatus != RunPaused {
		t.Fatalf("pause failed: %#v", paused)
	}
	snapshot.Status = RunPaused
	resumed, _ := Control(snapshot, ControlResume, now)
	if resumed.RunStatus != RunRunning {
		t.Fatalf("resume failed: %#v", resumed)
	}
	canceled, _ := Control(snapshot, ControlCancel, now)
	if canceled.IssueCommand.Action != "flight.return_home" || !canceled.RevokeOrdinary {
		t.Fatalf("safe cancel failed: %#v", canceled)
	}
	emergency, _ := Control(snapshot, ControlEmergency, now)
	if emergency.IssueCommand.Priority != 100 || emergency.IssueCommand.Action != "safety.emergency_stop" || !emergency.RevokeOrdinary {
		t.Fatalf("emergency path is not independent: %#v", emergency)
	}
}
