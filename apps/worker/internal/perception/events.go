package perception

import (
	"errors"
	"fmt"
	"time"
)

type RuleVersion struct {
	ID                  int64
	Version             int
	Label               string
	MinimumConfidence   float64
	Severity            string
	DeduplicationWindow time.Duration
}

type EventCandidate struct {
	ProjectID        int
	DetectionGroupID int64
	Label            string
	Confidence       float64
	DetectedAt       time.Time
}

type PerceptionEventState struct {
	ID                                 string
	RuleVersionID, DetectionGroupID    int64
	DeduplicationKey, Severity, Status string
	FirstDetectedAt, LastDetectedAt    time.Time
	OccurrenceCount, StateVersion      int
}

type EventEvaluation struct {
	Matched          bool
	Create           bool
	DeduplicationKey string
	Severity         string
	OccurrenceCount  int
	Reason           string
}

func EvaluateEventRule(rule RuleVersion, candidate EventCandidate, existing *PerceptionEventState) EventEvaluation {
	if candidate.Label != rule.Label || candidate.Confidence < rule.MinimumConfidence {
		return EventEvaluation{Reason: "candidate does not meet published rule label or confidence"}
	}
	key := fmt.Sprintf("rule-version:%d:group:%d", rule.ID, candidate.DetectionGroupID)
	if existing == nil || existing.RuleVersionID != rule.ID || existing.DetectionGroupID != candidate.DetectionGroupID || terminalEventStatus(existing.Status) {
		return EventEvaluation{Matched: true, Create: true, DeduplicationKey: key, Severity: rule.Severity, OccurrenceCount: 1, Reason: "new rule-version and group evaluation"}
	}
	return EventEvaluation{Matched: true, DeduplicationKey: key, Severity: rule.Severity, OccurrenceCount: existing.OccurrenceCount + 1, Reason: "deduplicated into active event"}
}

func TransitionPerceptionEvent(current, target string, expectedVersion, actualVersion int) (int, error) {
	if expectedVersion != actualVersion {
		return actualVersion, errors.New("perception event version conflict")
	}
	allowed := map[string]map[string]bool{
		"open":          {"acknowledged": true, "investigating": true, "resolved": true, "dismissed": true},
		"acknowledged":  {"investigating": true, "resolved": true, "dismissed": true},
		"investigating": {"acknowledged": true, "resolved": true, "dismissed": true},
		"resolved":      {}, "dismissed": {},
	}
	if !allowed[current][target] {
		return actualVersion, fmt.Errorf("illegal perception event transition %s -> %s", current, target)
	}
	return actualVersion + 1, nil
}

func terminalEventStatus(status string) bool { return status == "resolved" || status == "dismissed" }
