package agent

import (
	"errors"
	"testing"
	"time"
)

func TestQueuedExecutionFailsAfterPermissionRevocation(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	context := QueuedContext{UserID: 7, TeamID: 11, ProjectID: 17, SessionID: 23, RequiredPermission: "mission:operate", ExpiresAt: now.Add(time.Minute)}
	current := CurrentAuthorization{Now: now, MembershipActive: true, TeamRole: "member", PermissionGranted: false, ProjectTeamID: 11, SessionProjectID: 17, SessionUserID: 7, SessionOpen: true}
	if !errors.Is(AuthorizeQueuedExecution(context, current), ErrQueuedAuthorizationDenied) {
		t.Fatal("revoked queued permission must fail closed")
	}
}

func TestQueuedExecutionRejectsExpiredOrChangedScope(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	context := QueuedContext{UserID: 7, TeamID: 11, ProjectID: 17, SessionID: 23, RequiredPermission: "agent:use", ExpiresAt: now}
	current := CurrentAuthorization{Now: now, MembershipActive: true, TeamRole: "member", PermissionGranted: true, ProjectTeamID: 11, SessionProjectID: 17, SessionUserID: 7, SessionOpen: true}
	if AuthorizeQueuedExecution(context, current) == nil {
		t.Fatal("expired queued context must fail closed")
	}
	context.ExpiresAt = now.Add(time.Minute)
	current.SessionProjectID = 99
	if AuthorizeQueuedExecution(context, current) == nil {
		t.Fatal("changed session project must fail closed")
	}
}

func TestQueuedExecutionAllowsCurrentScopedPermission(t *testing.T) {
	now := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	context := QueuedContext{UserID: 7, TeamID: 11, ProjectID: 17, SessionID: 23, RequiredPermission: "mission:operate", ExpiresAt: now.Add(time.Minute)}
	current := CurrentAuthorization{Now: now, MembershipActive: true, TeamRole: "member", PermissionGranted: true, ProjectTeamID: 11, SessionProjectID: 17, SessionUserID: 7, SessionOpen: true}
	if err := AuthorizeQueuedExecution(context, current); err != nil {
		t.Fatalf("current scoped permission rejected: %v", err)
	}
}
