package mission

import (
	"reflect"
	"testing"
)

func TestAuditTraceCompletenessAndLastEmergency(t *testing.T) {
	policy := "4"
	ack := "acknowledged"
	nack := "nacked"
	request := &AuditRequest{RequestID: "request", ActorType: "agent", ActorID: 9}
	approval := &AuditApproval{ID: "approval", Status: "pending", RequiredApprovals: 2, ReceivedApprovals: 1}
	commands := []AuditCommand{{ID: "earlier", Status: "acknowledged", Priority: 100, AttemptStatus: &ack}, {ID: "latest", Status: "sent", Action: "flight.return_home", AttemptStatus: &nack}}
	result := BuildAuditTrace(1, 2, "agent", request, AuditPreflight{PolicyVersionID: &policy, Allowed: true}, approval, commands)
	if result.SafetyState != "rejected" || result.Complete || !reflect.DeepEqual(result.Missing, []string{"approval"}) || *result.Correlation.ApprovalRequestID != "approval" {
		t.Fatalf("pending %+v", result)
	}
	approval.Status = "approved"
	commands[1].AttemptStatus = nil
	result = BuildAuditTrace(1, 2, "agent", request, AuditPreflight{PolicyVersionID: &policy, Allowed: true}, approval, commands)
	if result.SafetyState != "unknown" || !reflect.DeepEqual(result.Missing, []string{"command_attempt"}) {
		t.Fatalf("missing attempt %+v", result)
	}
	commands[1].AttemptStatus = &ack
	result = BuildAuditTrace(1, 2, "agent", request, AuditPreflight{PolicyVersionID: &policy, Allowed: true}, approval, commands)
	if !result.Complete || result.SafetyState != "confirmed" || !reflect.DeepEqual(result.Correlation.CommandIDs, []string{"earlier", "latest"}) {
		t.Fatalf("complete %+v", result)
	}
	result = PlanEmergencyDrill(1, 2, 3, "request", "ack", "2026-09-06T00:00:00.000Z", true, false)
	if result.SafetyState != "unknown" || result.Stages.Commands[0].Attempt != nil || *result.Stages.Commands[0].AttemptStatus != "transport_unavailable" {
		t.Fatalf("undeclared capability %+v", result)
	}
}
