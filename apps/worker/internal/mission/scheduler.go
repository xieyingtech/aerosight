package mission

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type RunStatus string
type StepStatus string
type CommandStatus string

const (
	RunDispatching RunStatus = "dispatching"
	RunRunning     RunStatus = "running"
	RunPaused      RunStatus = "paused"
	RunSucceeded   RunStatus = "succeeded"
	RunFailed      RunStatus = "failed"
	RunCanceling   RunStatus = "canceling"
	RunCanceled    RunStatus = "canceled"

	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"

	CommandSent         CommandStatus = "sent"
	CommandAcknowledged CommandStatus = "acknowledged"
	CommandNacked       CommandStatus = "nacked"
	CommandTimedOut     CommandStatus = "timed_out"
)

type FailurePolicy struct {
	SafeToRetry    bool
	MaxRetries     int
	Backoff        time.Duration
	Timeout        time.Duration
	PauseOnFailure bool
}

type Command struct {
	ID             string
	IdempotencyKey string
	CapabilityCode string
	Action         string
	Status         CommandStatus
	Attempts       int
	Deadline       time.Time
	Priority       int
	Parameters     json.RawMessage
}

type Step struct {
	Position       int
	Status         StepStatus
	CapabilityCode string
	Action         string
	FailurePolicy  FailurePolicy
	Parameters     json.RawMessage
	Command        *Command
}

type Snapshot struct {
	RunID              int
	Status             RunStatus
	DeviceConnected    bool
	SupportsReturnHome bool
	Steps              []Step
}

type Signal struct {
	CommandID string
	Outcome   string
}

type Decision struct {
	RunStatus             RunStatus
	StepPosition          int
	StepStatus            StepStatus
	IssueCommand          *Command
	CompleteCommandID     string
	CompleteCommandStatus CommandStatus
	RetryAt               *time.Time
	RevokeOrdinary        bool
	SafetyUnknown         bool
	Reason                string
}

func stableCommand(runID, position int) (string, string) {
	key := fmt.Sprintf("task-run:%d:step:%d", runID, position)
	return stableUUID(key), key
}

func stableUUID(key string) string {
	sum := sha256.Sum256([]byte(key))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func nextStep(snapshot Snapshot) *Step {
	for index := range snapshot.Steps {
		if snapshot.Steps[index].Status != StepSucceeded {
			return &snapshot.Steps[index]
		}
	}
	return nil
}

func Advance(snapshot Snapshot, signal *Signal, now time.Time) (Decision, error) {
	if snapshot.Status != RunDispatching && snapshot.Status != RunRunning {
		return Decision{}, fmt.Errorf("run status %q cannot be scheduled", snapshot.Status)
	}
	if !snapshot.DeviceConnected {
		return Decision{RunStatus: RunPaused, Reason: "device_disconnected"}, nil
	}
	step := nextStep(snapshot)
	if step == nil {
		return Decision{RunStatus: RunSucceeded, Reason: "all_steps_succeeded"}, nil
	}
	if step.Command == nil {
		id, key := stableCommand(snapshot.RunID, step.Position)
		parameters := step.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{}`)
		}
		command := &Command{
			ID: id, IdempotencyKey: key, CapabilityCode: step.CapabilityCode, Action: step.Action,
			Status: CommandSent, Attempts: 1, Deadline: now.Add(step.FailurePolicy.Timeout), Parameters: parameters,
		}
		return Decision{RunStatus: RunRunning, StepPosition: step.Position, StepStatus: StepRunning, IssueCommand: command, Reason: "step_dispatched"}, nil
	}
	command := step.Command
	if signal != nil {
		if signal.CommandID != command.ID {
			return Decision{RunStatus: snapshot.Status, StepPosition: step.Position, StepStatus: step.Status, Reason: "unknown_ack_ignored"}, nil
		}
		switch signal.Outcome {
		case "ack":
			if step.Position == snapshot.Steps[len(snapshot.Steps)-1].Position {
				return Decision{RunStatus: RunSucceeded, StepPosition: step.Position, StepStatus: StepSucceeded,
					CompleteCommandID: command.ID, CompleteCommandStatus: CommandAcknowledged, Reason: "final_step_acknowledged"}, nil
			}
			return Decision{RunStatus: RunRunning, StepPosition: step.Position, StepStatus: StepSucceeded,
				CompleteCommandID: command.ID, CompleteCommandStatus: CommandAcknowledged, Reason: "step_acknowledged"}, nil
		case "nack":
			status := RunFailed
			if step.FailurePolicy.PauseOnFailure {
				status = RunPaused
			}
			return Decision{RunStatus: status, StepPosition: step.Position, StepStatus: StepFailed,
				CompleteCommandID: command.ID, CompleteCommandStatus: CommandNacked, Reason: "command_nacked"}, nil
		case "timeout":
			return timeoutDecision(snapshot, *step, now), nil
		default:
			return Decision{}, errors.New("unsupported command signal outcome")
		}
	}
	if !now.Before(command.Deadline) {
		return timeoutDecision(snapshot, *step, now), nil
	}
	return Decision{RunStatus: snapshot.Status, StepPosition: step.Position, StepStatus: step.Status, Reason: "awaiting_ack"}, nil
}

func timeoutDecision(snapshot Snapshot, step Step, now time.Time) Decision {
	command := step.Command
	if step.FailurePolicy.SafeToRetry && command.Attempts <= step.FailurePolicy.MaxRetries {
		retryAt := now.Add(step.FailurePolicy.Backoff)
		retry := *command
		retry.Status = CommandSent
		retry.Attempts++
		retry.Deadline = retryAt.Add(step.FailurePolicy.Timeout)
		return Decision{RunStatus: RunRunning, StepPosition: step.Position, StepStatus: StepRunning,
			IssueCommand: &retry, CompleteCommandID: command.ID, CompleteCommandStatus: CommandTimedOut,
			RetryAt: &retryAt, Reason: "safe_retry_scheduled"}
	}
	return Decision{RunStatus: RunPaused, StepPosition: step.Position, StepStatus: StepRunning,
		CompleteCommandID: command.ID, CompleteCommandStatus: CommandTimedOut,
		SafetyUnknown: !step.FailurePolicy.SafeToRetry, Reason: "command_timeout_requires_operator"}
}

type ControlAction string

const (
	ControlPause     ControlAction = "pause"
	ControlResume    ControlAction = "resume"
	ControlCancel    ControlAction = "cancel"
	ControlEmergency ControlAction = "emergency_stop"
)

func Control(snapshot Snapshot, action ControlAction, now time.Time) (Decision, error) {
	switch action {
	case ControlPause:
		if snapshot.Status != RunRunning && snapshot.Status != RunDispatching {
			return Decision{}, errors.New("run cannot be paused")
		}
		return Decision{RunStatus: RunPaused, Reason: "operator_paused"}, nil
	case ControlResume:
		if snapshot.Status != RunPaused || !snapshot.DeviceConnected {
			return Decision{}, errors.New("run cannot be resumed")
		}
		return Decision{RunStatus: RunRunning, Reason: "operator_resumed"}, nil
	case ControlCancel, ControlEmergency:
		priority := 90
		actionName := "device.stop"
		if action == ControlEmergency {
			priority, actionName = 100, "safety.emergency_stop"
		} else if snapshot.SupportsReturnHome {
			actionName = "flight.return_home"
		}
		command := &Command{ID: stableUUID(fmt.Sprintf("task-run:%d:control:%s", snapshot.RunID, action)),
			IdempotencyKey: fmt.Sprintf("task-run:%d:control:%s", snapshot.RunID, action),
			CapabilityCode: actionName, Action: actionName, Status: CommandSent, Attempts: 1,
			Deadline: now.Add(15 * time.Second), Priority: priority, Parameters: json.RawMessage(`{}`)}
		return Decision{RunStatus: RunCanceling, IssueCommand: command, RevokeOrdinary: true,
			SafetyUnknown: !snapshot.DeviceConnected, Reason: string(action)}, nil
	default:
		return Decision{}, errors.New("unsupported control action")
	}
}
