package perception

import (
	"testing"
	"time"
)

func TestAlertStormDeduplicatesWithinRuleVersionAndGroup(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	rule := RuleVersion{ID: 7, Version: 2, Label: "suspected-construction", MinimumConfidence: .7, Severity: "high", DeduplicationWindow: time.Hour}
	candidate := EventCandidate{ProjectID: 2, DetectionGroupID: 9, Label: "suspected-construction", Confidence: .91, DetectedAt: now}
	first := EvaluateEventRule(rule, candidate, nil)
	if !first.Create || first.OccurrenceCount != 1 {
		t.Fatalf("first event not created: %+v", first)
	}
	state := &PerceptionEventState{RuleVersionID: 7, DetectionGroupID: 9, DeduplicationKey: first.DeduplicationKey, Severity: "high", Status: "open", OccurrenceCount: 1, FirstDetectedAt: now, LastDetectedAt: now}
	for range 100 {
		evaluation := EvaluateEventRule(rule, candidate, state)
		if evaluation.Create {
			t.Fatal("alert storm created another active event")
		}
		state.OccurrenceCount = evaluation.OccurrenceCount
	}
	if state.OccurrenceCount != 101 {
		t.Fatalf("occurrence count lost updates: %d", state.OccurrenceCount)
	}
}

func TestRuleUpgradeDoesNotRewriteHistoricalEventBasis(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	existing := &PerceptionEventState{RuleVersionID: 7, DetectionGroupID: 9, Status: "open", OccurrenceCount: 3}
	upgraded := RuleVersion{ID: 8, Version: 3, Label: "suspected-construction", MinimumConfidence: .8, Severity: "critical"}
	evaluation := EvaluateEventRule(upgraded, EventCandidate{ProjectID: 2, DetectionGroupID: 9, Label: "suspected-construction", Confidence: .9, DetectedAt: now}, existing)
	if !evaluation.Create || evaluation.DeduplicationKey == existing.DeduplicationKey {
		t.Fatalf("rule upgrade mutated historical event: %+v", evaluation)
	}
}

func TestEventLifecycleUsesOptimisticConcurrencyAndTerminalStates(t *testing.T) {
	version, err := TransitionPerceptionEvent("open", "acknowledged", 2, 2)
	if err != nil || version != 3 {
		t.Fatalf("valid transition failed: %d %v", version, err)
	}
	if _, err := TransitionPerceptionEvent("acknowledged", "investigating", 2, 3); err == nil {
		t.Fatal("stale disposition accepted")
	}
	if _, err := TransitionPerceptionEvent("resolved", "open", 4, 4); err == nil {
		t.Fatal("terminal event reopened")
	}
}
