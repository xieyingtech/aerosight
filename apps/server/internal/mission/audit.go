package mission

import "fmt"

type AuditRequest struct {
	RequestID string `json:"requestId"`
	Action    string `json:"action"`
	ActorType string `json:"actorType"`
	ActorID   int32  `json:"actorId"`
	CreatedAt string `json:"createdAt"`
}
type AuditPreflight struct {
	PolicyVersionID *string `json:"policyVersionId"`
	Allowed         bool    `json:"allowed"`
	Checks          []any   `json:"checks"`
}
type AuditApproval struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	RequiredApprovals int32  `json:"requiredApprovals"`
	ReceivedApprovals int32  `json:"receivedApprovals"`
}
type AuditCommand struct {
	ID             string  `json:"id"`
	Action         string  `json:"action"`
	CapabilityCode string  `json:"capabilityCode"`
	Status         string  `json:"status"`
	Priority       int32   `json:"priority"`
	Attempt        *int32  `json:"attempt"`
	AttemptStatus  *string `json:"attemptStatus"`
	ErrorCode      *string `json:"errorCode"`
}
type AuditTrace struct {
	ProjectID     int32  `json:"projectId"`
	TaskRunID     int32  `json:"taskRunId"`
	TriggerSource string `json:"triggerSource"`
	Correlation   struct {
		RequestID         *string  `json:"requestId"`
		ApprovalRequestID *string  `json:"approvalRequestId"`
		CommandIDs        []string `json:"commandIds"`
	} `json:"correlation"`
	Stages struct {
		Request   *AuditRequest  `json:"request"`
		Preflight AuditPreflight `json:"preflight"`
		Approval  *AuditApproval `json:"approval"`
		Commands  []AuditCommand `json:"commands"`
	} `json:"stages"`
	SafetyState string   `json:"safetyState"`
	Complete    bool     `json:"complete"`
	Missing     []string `json:"missing"`
}

func BuildAuditTrace(pid, runID int32, trigger string, request *AuditRequest, preflight AuditPreflight, approval *AuditApproval, commands []AuditCommand) AuditTrace {
	out := AuditTrace{ProjectID: pid, TaskRunID: runID, TriggerSource: trigger, SafetyState: "not_requested", Missing: []string{}}
	if commands == nil {
		commands = []AuditCommand{}
	}
	if preflight.Checks == nil {
		preflight.Checks = []any{}
	}
	out.Stages.Request = request
	out.Stages.Preflight = preflight
	out.Stages.Approval = approval
	out.Stages.Commands = commands
	out.Correlation.CommandIDs = []string{}
	if request != nil {
		out.Correlation.RequestID = &request.RequestID
	} else {
		out.Missing = append(out.Missing, "request")
	}
	if preflight.PolicyVersionID == nil || *preflight.PolicyVersionID == "" {
		out.Missing = append(out.Missing, "preflight_policy_version")
	}
	if !preflight.Allowed {
		out.Missing = append(out.Missing, "preflight_pass")
	}
	if approval != nil {
		out.Correlation.ApprovalRequestID = &approval.ID
		if approval.Status != "approved" {
			out.Missing = append(out.Missing, "approval")
		}
	}
	if len(commands) == 0 {
		out.Missing = append(out.Missing, "command")
	}
	missingAttempt := false
	for _, command := range commands {
		out.Correlation.CommandIDs = append(out.Correlation.CommandIDs, command.ID)
		attempt := ""
		if command.AttemptStatus != nil {
			attempt = *command.AttemptStatus
		}
		if attempt == "" {
			missingAttempt = true
		}
		if command.Priority >= 90 || command.Action == "safety.emergency_stop" || command.Action == "flight.return_home" {
			out.SafetyState = "unknown"
			if command.Status == "acknowledged" || attempt == "acknowledged" {
				out.SafetyState = "confirmed"
			} else if command.Status == "nacked" || attempt == "nacked" {
				out.SafetyState = "rejected"
			}
		}
	}
	if missingAttempt {
		out.Missing = append(out.Missing, "command_attempt")
	}
	out.Complete = len(out.Missing) == 0
	return out
}
func PlanEmergencyDrill(pid, runID, uid int32, requestID, outcome, now string, connected, capability bool) AuditTrace {
	dispatchable := connected && capability && outcome != "disconnected"
	status := "unknown"
	if dispatchable && outcome != "timeout" {
		status = "nacked"
		if outcome == "ack" {
			status = "acknowledged"
		}
	}
	command := AuditCommand{ID: fmt.Sprintf("drill:%d:%s", runID, requestID), Action: "safety.emergency_stop", CapabilityCode: "safety.emergency_stop", Priority: 100, Status: status}
	attemptStatus := "transport_unavailable"
	if dispatchable {
		one := int32(1)
		command.Attempt = &one
		attemptStatus = status
	}
	command.AttemptStatus = &attemptStatus
	if status == "unknown" {
		code := "DEVICE_DISCONNECTED"
		if connected {
			code = "ACK_TIMEOUT"
		}
		command.ErrorCode = &code
	}
	policy := "drill-policy"
	return BuildAuditTrace(pid, runID, "emergency_stop_drill", &AuditRequest{RequestID: requestID, Action: "safety.emergency_stop_drill", ActorType: "user", ActorID: uid, CreatedAt: now}, AuditPreflight{PolicyVersionID: &policy, Allowed: true, Checks: []any{map[string]any{"code": "DRY_RUN_ONLY", "severity": "pass"}}}, nil, []AuditCommand{command})
}
