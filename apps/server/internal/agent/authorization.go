package agent

import (
	"errors"
	"time"
)

var ErrQueuedAuthorizationDenied = errors.New("queued agent authorization denied")

type QueuedContext struct {
	UserID             int
	TeamID             int
	ProjectID          int
	SessionID          int
	RequiredPermission string
	ExpiresAt          time.Time
}

type CurrentAuthorization struct {
	Now               time.Time
	MembershipActive  bool
	TeamRole          string
	PermissionGranted bool
	ProjectTeamID     int
	SessionProjectID  int
	SessionUserID     int
	SessionOpen       bool
}

func AuthorizeQueuedExecution(context QueuedContext, current CurrentAuthorization) error {
	if !current.Now.Before(context.ExpiresAt) {
		return ErrQueuedAuthorizationDenied
	}
	if !current.MembershipActive || !current.SessionOpen {
		return ErrQueuedAuthorizationDenied
	}
	if current.ProjectTeamID != context.TeamID || current.SessionProjectID != context.ProjectID || current.SessionUserID != context.UserID {
		return ErrQueuedAuthorizationDenied
	}
	if context.RequiredPermission == "project:view" || current.TeamRole == "owner" || current.TeamRole == "admin" {
		return nil
	}
	if !current.PermissionGranted {
		return ErrQueuedAuthorizationDenied
	}
	return nil
}
