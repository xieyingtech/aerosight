package mission

import (
	"encoding/json"
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

func TestMissionCommandRetainsPublishedStepParameters(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	parameters := json.RawMessage(`{"flight_id":"flight-demo","task_type":0}`)
	decision, err := Advance(Snapshot{
		RunID: 41, Status: RunDispatching, DeviceConnected: true,
		Steps: []Step{{Position: 1, Status: StepPending, CapabilityCode: "mission.execute", Action: "prepare", Parameters: parameters, FailurePolicy: FailurePolicy{Timeout: time.Minute}}},
	}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.IssueCommand == nil || string(decision.IssueCommand.Parameters) != string(parameters) {
		t.Fatalf("published step parameters did not reach command ledger: %+v", decision.IssueCommand)
	}
}

func TestNonDeviceStepDispatchesThroughTypedTaskExecutor(t *testing.T) {
	snapshot := Snapshot{RunID: 42, Status: RunRunning, DeviceConnected: false, Steps: []Step{{
		ID: 71, Position: 1, Key: "detect", Uses: "algorithm.run", Status: StepPending,
	}}}
	decision, err := Advance(snapshot, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if decision.InvokeStep == nil || decision.InvokeStep.StepID != 71 || decision.InvokeStep.Uses != "algorithm.run" || decision.StepStatus != StepRunning {
		t.Fatalf("typed step was not dispatched: %+v", decision)
	}
}

func TestCollectStepWaitsForAvailableAssetAfterAck(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	snapshot := Snapshot{RunID: 17, Status: RunRunning, DeviceConnected: true, Steps: []Step{{
		ID: 71, Position: 1, Key: "collect", Uses: "device.collect", Status: StepRunning,
		Command: &Command{ID: "collect-command", Status: CommandSent, Deadline: now.Add(time.Minute)},
	}}}
	decision, err := Advance(snapshot, &Signal{CommandID: "collect-command", Outcome: "ack"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.StepStatus != StepRunning || decision.Reason != "collection_acknowledged" || decision.CompleteCommandStatus != CommandAcknowledged {
		t.Fatalf("collect ACK completed before media was available: %+v", decision)
	}
	snapshot.Steps[0].Command.Status = CommandAcknowledged
	decision, err = Advance(snapshot, nil, now)
	if err != nil || decision.Reason != "awaiting_collection_asset" || decision.StepStatus != StepRunning {
		t.Fatalf("acknowledged collection did not wait for its asset: %+v err=%v", decision, err)
	}
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

func TestUnknownReplyCannotAdvanceMission(t *testing.T) {
	now := time.Now()
	snapshot := fixture()
	dispatched, _ := Advance(snapshot, nil, now)
	snapshot.Status, snapshot.Steps[0].Status, snapshot.Steps[0].Command = RunRunning, StepRunning, dispatched.IssueCommand
	decision, err := Advance(snapshot, &Signal{CommandID: "another-command", Outcome: "ack"}, now.Add(time.Second))
	if err != nil || decision.Reason != "unknown_ack_ignored" || decision.StepStatus != StepRunning || decision.RunStatus != RunRunning {
		t.Fatalf("unknown reply advanced mission: decision=%+v err=%v", decision, err)
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
