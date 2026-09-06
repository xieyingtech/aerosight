package device

import (
	"errors"
	"strings"
	"time"
)

var ErrCapabilityAuthorizationDenied = errors.New("device capability authorization denied")

type GrantScope string
type GrantEffect string

const (
	GrantScopeProject    GrantScope = "project"
	GrantScopeDeviceType GrantScope = "device_type"
	GrantScopeDevice     GrantScope = "device"

	GrantAllow GrantEffect = "allow"
	GrantDeny  GrantEffect = "deny"
)

type CapabilityGrant struct {
	ProjectID  int
	UserID     int
	Scope      GrantScope
	DeviceType TypeReference
	DeviceID   string
	Action     string
	Effect     GrantEffect
	ExpiresAt  *time.Time
}

type AuthorizationSubject struct {
	UserID          int
	ProjectID       int
	MembershipAlive bool
	Role            string
}

type CapabilityTarget struct {
	ProjectID  int
	DeviceID   string
	DeviceType TypeReference
	Action     string
	Available  bool
}

type AuthorizationDecision struct {
	Allowed bool
	Reason  string
}

func validActionPattern(pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		pattern = strings.TrimSuffix(pattern, ".*")
	}
	if pattern == "" || strings.HasPrefix(pattern, ".") || strings.HasSuffix(pattern, ".") {
		return false
	}
	for _, part := range strings.Split(pattern, ".") {
		if part == "" {
			return false
		}
		for _, character := range part {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
				return false
			}
		}
	}
	return true
}

func actionMatches(pattern, action string) bool {
	if !validActionPattern(pattern) || !validActionPattern(action) || strings.HasSuffix(action, ".*") || action == "*" {
		return false
	}
	if pattern == "*" || pattern == action {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(action, prefix)
	}
	return false
}

func grantMatches(grant CapabilityGrant, subject AuthorizationSubject, target CapabilityTarget, now time.Time) bool {
	if grant.ProjectID != target.ProjectID || grant.UserID != subject.UserID || !actionMatches(grant.Action, target.Action) {
		return false
	}
	if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
		return false
	}
	switch grant.Scope {
	case GrantScopeProject:
		return grant.DeviceID == "" && grant.DeviceType == (TypeReference{})
	case GrantScopeDeviceType:
		return grant.DeviceID == "" && grant.DeviceType == target.DeviceType
	case GrantScopeDevice:
		return grant.DeviceType == (TypeReference{}) && grant.DeviceID == target.DeviceID
	default:
		return false
	}
}

func roleAllows(role, action string) bool {
	switch role {
	case "owner", "admin":
		return true
	case "member":
		return action == "state.read"
	default:
		return false
	}
}

func AuthorizeCapability(subject AuthorizationSubject, target CapabilityTarget, grants []CapabilityGrant, now time.Time) AuthorizationDecision {
	if !subject.MembershipAlive || subject.UserID <= 0 || subject.ProjectID <= 0 || target.ProjectID != subject.ProjectID || target.DeviceID == "" {
		return AuthorizationDecision{Reason: "CAPABILITY_SCOPE_DENIED"}
	}
	if !validActionPattern(target.Action) || target.Action == "*" || strings.HasSuffix(target.Action, ".*") {
		return AuthorizationDecision{Reason: "CAPABILITY_ACTION_UNKNOWN"}
	}
	if !target.Available {
		return AuthorizationDecision{Reason: "CAPABILITY_UNAVAILABLE"}
	}
	allow := roleAllows(subject.Role, target.Action)
	for _, grant := range grants {
		if !grantMatches(grant, subject, target, now) {
			continue
		}
		if grant.Effect == GrantDeny {
			return AuthorizationDecision{Reason: "CAPABILITY_EXPLICITLY_DENIED"}
		}
		if grant.Effect == GrantAllow {
			allow = true
		}
	}
	if !allow {
		return AuthorizationDecision{Reason: "CAPABILITY_NOT_GRANTED"}
	}
	return AuthorizationDecision{Allowed: true, Reason: "CAPABILITY_AUTHORIZED"}
}

func RequireCapability(subject AuthorizationSubject, target CapabilityTarget, grants []CapabilityGrant, now time.Time) error {
	if !AuthorizeCapability(subject, target, grants, now).Allowed {
		return ErrCapabilityAuthorizationDenied
	}
	return nil
}
