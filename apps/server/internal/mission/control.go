package mission

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var runTransitions = map[string][]string{
	"queued": {"blocked", "ready", "canceled"}, "blocked": {"queued", "ready", "canceling", "canceled"},
	"ready": {"blocked", "dispatching", "canceling", "canceled"}, "dispatching": {"running", "paused", "failed", "canceling"},
	"running": {"paused", "succeeded", "failed", "canceling"}, "paused": {"running", "failed", "canceling"},
	"canceling": {"canceled", "failed"}, "succeeded": {}, "failed": {}, "canceled": {},
}

func ControlNextStatus(current, action, reason string) (string, error) {
	next := "canceling"
	switch action {
	case "pause":
		next = "paused"
	case "resume":
		next = "running"
	case "cancel":
		if current == "queued" {
			next = "canceled"
		}
	case "emergency_stop":
	default:
		return "", errors.New("MISSION_CONTROL_INPUT_INVALID")
	}
	if !(action == "emergency_stop" && current == "canceling") && !slices.Contains(runTransitions[current], next) {
		return "", fmt.Errorf("TASK_RUN_TRANSITION_INVALID:%s:%s", current, next)
	}
	if strings.TrimSpace(reason) == "" {
		return "", errors.New("TASK_RUN_TRANSITION_REASON_REQUIRED")
	}
	return next, nil
}
