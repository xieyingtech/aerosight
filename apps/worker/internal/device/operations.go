package device

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type SafetyDecision struct {
	Allowed  bool
	Reason   string
	Snapshot json.RawMessage
}

type OperationRequest struct {
	Subject        AuthorizationSubject
	Target         CapabilityTarget
	Capability     EffectiveCapability
	Grants         []CapabilityGrant
	IdempotencyKey string
	Parameters     json.RawMessage
	SafetyContext  json.RawMessage
	Deadline       time.Time
	Priority       int
}

type CommandLedgerEntry struct {
	ID                    string
	ProjectID             int
	DeviceID              string
	CapabilityCode        string
	IdempotencyKey        string
	Parameters            json.RawMessage
	SafetyContext         json.RawMessage
	AuthorizationSnapshot json.RawMessage
	Status                string
	Priority              int
	Deadline              time.Time
}

type OperationSafetyPolicy interface {
	Evaluate(context.Context, OperationRequest) (SafetyDecision, error)
}

type CommandLedger interface {
	Append(context.Context, CommandLedgerEntry) (CommandLedgerEntry, error)
}

type OperationGateway struct {
	safety OperationSafetyPolicy
	ledger CommandLedger
	now    func() time.Time
}

func NewOperationGateway(safety OperationSafetyPolicy, ledger CommandLedger, now func() time.Time) (*OperationGateway, error) {
	if safety == nil || ledger == nil {
		return nil, errors.New("operation gateway requires safety policy and command ledger")
	}
	if now == nil {
		now = time.Now
	}
	return &OperationGateway{safety: safety, ledger: ledger, now: now}, nil
}

func (gateway *OperationGateway) Submit(ctx context.Context, request OperationRequest) (CommandLedgerEntry, error) {
	now := gateway.now()
	if request.Capability.Code == "" || request.Capability.Code != request.Target.Action || !request.Capability.Available {
		return CommandLedgerEntry{}, errors.New("operation capability is unavailable or does not match the requested action")
	}
	request.Target.Available = request.Capability.Available
	decision := AuthorizeCapability(request.Subject, request.Target, request.Grants, now)
	if !decision.Allowed {
		return CommandLedgerEntry{}, ErrCapabilityAuthorizationDenied
	}
	if request.IdempotencyKey == "" || len(request.Parameters) == 0 || len(request.SafetyContext) == 0 || !request.Deadline.After(now) {
		return CommandLedgerEntry{}, errors.New("operation request is missing ledger or safety fields")
	}
	safety, err := gateway.safety.Evaluate(ctx, request)
	if err != nil {
		return CommandLedgerEntry{}, err
	}
	if !safety.Allowed {
		return CommandLedgerEntry{}, errors.New("operation safety policy denied the request: " + safety.Reason)
	}
	authorizationSnapshot, err := json.Marshal(map[string]any{
		"userId": request.Subject.UserID, "projectId": request.Subject.ProjectID,
		"deviceId": request.Target.DeviceID, "deviceType": request.Target.DeviceType,
		"capability": request.Capability.Code, "decision": decision.Reason, "evaluatedAt": now,
	})
	if err != nil {
		return CommandLedgerEntry{}, err
	}
	combinedSafety, err := json.Marshal(map[string]any{
		"request": json.RawMessage(request.SafetyContext), "policy": json.RawMessage(safety.Snapshot), "reason": safety.Reason,
	})
	if err != nil {
		return CommandLedgerEntry{}, err
	}
	return gateway.ledger.Append(ctx, CommandLedgerEntry{
		ProjectID: request.Target.ProjectID, DeviceID: request.Target.DeviceID, CapabilityCode: request.Capability.Code,
		IdempotencyKey: request.IdempotencyKey, Parameters: request.Parameters, SafetyContext: combinedSafety,
		AuthorizationSnapshot: authorizationSnapshot, Status: "dispatchable", Priority: request.Priority, Deadline: request.Deadline,
	})
}
