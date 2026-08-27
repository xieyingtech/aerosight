package device

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aerosight/worker/internal/driver"
)

type safetyFixture struct {
	decision SafetyDecision
	calls    int
}

func (fixture *safetyFixture) Evaluate(context.Context, OperationRequest) (SafetyDecision, error) {
	fixture.calls++
	return fixture.decision, nil
}

type ledgerFixture struct {
	entries []CommandLedgerEntry
}

func (fixture *ledgerFixture) Append(_ context.Context, entry CommandLedgerEntry) (CommandLedgerEntry, error) {
	entry.ID = "command-1"
	fixture.entries = append(fixture.entries, entry)
	return entry, nil
}

func operationFixture(now time.Time) OperationRequest {
	subject, target := authorizationFixture("mission.execute")
	target.DeviceID = "aircraft-1"
	target.DeviceType = TypeReference{Key: "dji.m4d", Version: 1}
	return OperationRequest{
		Subject: subject, Target: target,
		Capability:     EffectiveCapability{Code: "mission.execute", Kind: driver.CapabilityCommand, Risk: driver.RiskHigh, Available: true},
		Grants:         []CapabilityGrant{{ProjectID: 17, UserID: 7, Scope: GrantScopeDevice, DeviceID: "aircraft-1", Action: "mission.execute", Effect: GrantAllow}},
		IdempotencyKey: "mission:42:start", Parameters: json.RawMessage(`{"routeId":"route-1"}`),
		SafetyContext: json.RawMessage(`{"confirmed":true}`), Deadline: now.Add(time.Minute), Priority: 50,
	}
}

func TestOperationGatewayPersistsBeforeAnyDownstreamDispatch(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	safety := &safetyFixture{decision: SafetyDecision{Allowed: true, Reason: "PREFLIGHT_PASSED", Snapshot: json.RawMessage(`{"policyVersion":3}`)}}
	ledger := &ledgerFixture{}
	gateway, err := NewOperationGateway(safety, ledger, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	entry, err := gateway.Submit(context.Background(), operationFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" || entry.Status != "dispatchable" || len(ledger.entries) != 1 || safety.calls != 1 {
		t.Fatalf("authorized operation did not enter the command ledger: entry=%+v calls=%d", entry, safety.calls)
	}
	if len(entry.AuthorizationSnapshot) == 0 || len(entry.SafetyContext) == 0 {
		t.Fatalf("command ledger omitted authorization or safety evidence: %+v", entry)
	}
}

func TestOperationGatewayRejectsCrossProjectBeforeSafetyOrLedger(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	safety := &safetyFixture{decision: SafetyDecision{Allowed: true}}
	ledger := &ledgerFixture{}
	gateway, _ := NewOperationGateway(safety, ledger, func() time.Time { return now })
	request := operationFixture(now)
	request.Target.ProjectID = 18
	if _, err := gateway.Submit(context.Background(), request); !errors.Is(err, ErrCapabilityAuthorizationDenied) {
		t.Fatalf("cross-project operation was not rejected by capability authorization: %v", err)
	}
	if safety.calls != 0 || len(ledger.entries) != 0 {
		t.Fatal("rejected cross-project operation reached safety or command ledger")
	}
}

func TestOperationGatewayRejectsUnknownUnavailableAndUnsafeOperations(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	safety := &safetyFixture{decision: SafetyDecision{Allowed: false, Reason: "ACTIVE_TASK_CONFLICT"}}
	ledger := &ledgerFixture{}
	gateway, _ := NewOperationGateway(safety, ledger, func() time.Time { return now })
	request := operationFixture(now)
	request.Capability.Code = "vendor.direct_method"
	if _, err := gateway.Submit(context.Background(), request); err == nil {
		t.Fatal("capability mismatch bypassed the gateway")
	}
	request = operationFixture(now)
	request.Capability.Available = false
	if _, err := gateway.Submit(context.Background(), request); err == nil {
		t.Fatal("unavailable capability entered the ledger")
	}
	request = operationFixture(now)
	if _, err := gateway.Submit(context.Background(), request); err == nil {
		t.Fatal("safety-denied operation entered the ledger")
	}
	if len(ledger.entries) != 0 {
		t.Fatal("a rejected operation created a command ledger entry")
	}
}
