package flighthub

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

type controlSessionStoreFixture struct {
	session      FlightHubControlSession
	queuedReason string
	finishedCode string
}

func (store *controlSessionStoreFixture) Load(context.Context, int, string, time.Time) (FlightHubControlSession, error) {
	return store.session, nil
}

func (store *controlSessionStoreFixture) BeginAcquire(_ context.Context, _ FlightHubControlSession, _ time.Time) (bool, error) {
	store.session.Status = "acquiring"
	store.session.AcquireAttemptCount = 1
	return true, nil
}

func (store *controlSessionStoreFixture) Activate(context.Context, FlightHubControlSession, time.Time) error {
	store.session.Status = "active"
	return nil
}

func (store *controlSessionStoreFixture) QueueRelease(_ context.Context, _ FlightHubControlSession, code string, _ time.Time) error {
	store.session.Status = "releasing"
	store.queuedReason = code
	return nil
}

func (store *controlSessionStoreFixture) BeginRelease(context.Context, FlightHubControlSession, time.Time) (bool, error) {
	store.session.ReleaseAttemptCount = 1
	return true, nil
}

func (store *controlSessionStoreFixture) RecordReleaseUnconfirmed(_ context.Context, _ FlightHubControlSession, code string, _ time.Time) error {
	store.finishedCode = code
	return nil
}

func (store *controlSessionStoreFixture) Finish(_ context.Context, _ FlightHubControlSession, status, code string, _ time.Time) error {
	store.session.Status = status
	store.finishedCode = code
	return nil
}

func (store *controlSessionStoreFixture) Sweep(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

type controlOwnershipClientFixture struct {
	calls  []string
	output ControlOwnershipOutput
	err    error
}

func (client *controlOwnershipClientFixture) ExecuteControlOwnership(_ context.Context, _, _, action, _ string, _ ControlSelection) (ControlOwnershipOutput, error) {
	client.calls = append(client.calls, action)
	return client.output, client.err
}

type controlSessionTokenResolverFixture struct{}

func (controlSessionTokenResolverFixture) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_REDACTED", nil
}

func validControlSession(now time.Time) FlightHubControlSession {
	return FlightHubControlSession{
		ID: "11111111-1111-4111-8111-111111111111", ProjectID: 3, TeamID: 2, DeviceID: 11,
		ConnectorInstanceID: 7, Status: "requested", DeviceSN: "AIRCRAFT_REDACTED", ConnectorStatus: "connected",
		Controls:       ControlSelection{Flight: true, PayloadIndex: []string{"0-0"}},
		LeaseExpiresAt: now.Add(15 * time.Second), AbsoluteExpiresAt: now.Add(5 * time.Minute),
		FeatureEnabled: true, CapabilityVerified: true, DeviceOnline: true, StateFresh: true,
		ApprovalValid: true, SafetyPolicyCurrent: true, PermissionCurrent: true,
		Instance: connector.Instance{ID: 7, ProjectID: 3}, ProjectUUID: "PROJECT_REDACTED",
	}
}

func ownershipOutput() ControlOwnershipOutput {
	return ControlOwnershipOutput{DroneSN: "AIRCRAFT_REDACTED", Controls: []ControlOwnership{
		{Type: "flight"}, {Type: "payload", PayloadIndex: "0-0"},
	}}
}

func TestControlSessionAcquireAndReleaseAreSingleAttempt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	store := &controlSessionStoreFixture{session: validControlSession(now)}
	client := &controlOwnershipClientFixture{output: ownershipOutput()}
	handler, err := NewControlSessionHandler(store, client, controlSessionTokenResolverFixture{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	event := outbox.Event{ProjectID: 3, TeamID: 2, Payload: []byte(`{"sessionId":"11111111-1111-4111-8111-111111111111"}`)}
	if err := handler.Handler(context.Background(), (*sql.Tx)(nil), event); err != nil || store.session.Status != "active" {
		t.Fatalf("acquire status=%s calls=%#v err=%v", store.session.Status, client.calls, err)
	}
	store.session.Status = "releasing"
	if err := handler.Handler(context.Background(), nil, event); err != nil || store.session.Status != "released" {
		t.Fatalf("release status=%s calls=%#v err=%v", store.session.Status, client.calls, err)
	}
	if len(client.calls) != 2 || client.calls[0] != "control.acquire" || client.calls[1] != "control.release" {
		t.Fatalf("unexpected remote attempts %#v", client.calls)
	}
}

func TestControlSessionRestartAndRevocationFailClosed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	store := &controlSessionStoreFixture{session: validControlSession(now)}
	store.session.Status = "acquiring"
	store.session.AcquireAttemptCount = 1
	client := &controlOwnershipClientFixture{output: ownershipOutput()}
	handler, _ := NewControlSessionHandler(store, client, controlSessionTokenResolverFixture{}, func() time.Time { return now })
	event := outbox.Event{ProjectID: 3, TeamID: 2, Payload: []byte(`{"sessionId":"11111111-1111-4111-8111-111111111111"}`)}
	if err := handler.Handler(context.Background(), nil, event); err != nil || store.session.Status != "releasing" || store.queuedReason != "acquire_interrupted" || len(client.calls) != 0 {
		t.Fatalf("interrupted acquire was retried status=%s reason=%s calls=%#v err=%v", store.session.Status, store.queuedReason, client.calls, err)
	}

	store.session = validControlSession(now)
	store.session.PermissionCurrent = false
	if err := handler.Handler(context.Background(), nil, event); err != nil || store.session.Status != "failed" || store.finishedCode != "safety_gate_failed" || len(client.calls) != 0 {
		t.Fatalf("revoked permission reached upstream status=%s code=%s calls=%#v err=%v", store.session.Status, store.finishedCode, client.calls, err)
	}
}

func TestControlSessionUnknownReleaseKeepsExclusiveQuarantine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	store := &controlSessionStoreFixture{session: validControlSession(now)}
	store.session.Status = "releasing"
	client := &controlOwnershipClientFixture{err: context.DeadlineExceeded}
	handler, _ := NewControlSessionHandler(store, client, controlSessionTokenResolverFixture{}, func() time.Time { return now })
	event := outbox.Event{ProjectID: 3, TeamID: 2, Payload: []byte(`{"sessionId":"11111111-1111-4111-8111-111111111111"}`)}
	if err := handler.Handler(context.Background(), nil, event); err != nil || store.session.Status != "releasing" ||
		store.finishedCode != "release_unconfirmed" || len(client.calls) != 1 {
		t.Fatalf("unknown release cleared exclusivity status=%s code=%s calls=%#v err=%v",
			store.session.Status, store.finishedCode, client.calls, err)
	}
}

func TestControlOwnershipEvidenceMustCoverEveryRequestedTarget(t *testing.T) {
	t.Parallel()
	selection := ControlSelection{Flight: true, PayloadIndex: []string{"0-0", "1-0"}}
	if controlOwnershipMatches(ownershipOutput(), "AIRCRAFT_REDACTED", selection) {
		t.Fatal("partial ownership evidence was accepted")
	}
	complete := ownershipOutput()
	complete.Controls = append(complete.Controls, ControlOwnership{Type: "payload", PayloadIndex: "1-0"})
	if !controlOwnershipMatches(complete, "AIRCRAFT_REDACTED", selection) {
		t.Fatal("complete ownership evidence was rejected")
	}
}
