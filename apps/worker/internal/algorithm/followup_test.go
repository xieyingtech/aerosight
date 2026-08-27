package algorithm

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMaliciousProviderOutputCannotCreateDeviceCommand(t *testing.T) {
	malicious := CanonicalResult{Kind: ResultCustom, Custom: json.RawMessage(`{
		"deviceCommand":{"capability":"flight.return_home","deviceId":99},
		"status":"approved","parameters":{"confirm":true}
	}`)}
	draft, err := PlanAlgorithmFollowUp(FollowUpRule{
		RuleID: 4, ProjectID: 17, Kind: FollowUpTaskDraft, Title: "人工复核任务", RequiresConfirmation: true,
	}, "run-42", malicious, time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(draft)
	for _, forbidden := range []string{"flight.return_home", "deviceCommand", "deviceId", "approved"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("provider-controlled command field escaped into follow-up draft: %s", encoded)
		}
	}
	if draft.Status != "pending_review" || !draft.RequiresReview || draft.EvidenceReference != "algorithm-run:run-42" {
		t.Fatalf("follow-up skipped review or provenance: %+v", draft)
	}
}

func TestAlgorithmFollowUpRejectsExecutableKinds(t *testing.T) {
	result := CanonicalResult{Kind: ResultScalar, Scalar: &ScalarResult{Value: 1}}
	if _, err := PlanAlgorithmFollowUp(FollowUpRule{RuleID: 1, ProjectID: 17, Kind: "device_command", Title: "unsafe"}, "run-1", result, time.Now()); err == nil {
		t.Fatal("algorithm follow-up accepted an executable device command kind")
	}
}
