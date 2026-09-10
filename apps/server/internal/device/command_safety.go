package device

import (
	"errors"
	"fmt"
	"strings"
)

type CommandSafetyInput struct {
	ProjectID, DeviceProjectID, DeviceID   int32
	Capability, Risk, Availability, Status string
	ActiveTasks                            int32
	Confirmation                           string
}
type CommandSafety struct {
	Allowed              bool `json:"allowed"`
	ConfirmationRequired bool `json:"confirmationRequired"`
	ActiveTaskOverride   bool `json:"activeTaskOverride"`
}

func CheckCommandSafety(in CommandSafetyInput) (CommandSafety, error) {
	var out CommandSafety
	if in.ProjectID != in.DeviceProjectID {
		return out, errors.New("DEVICE_COMMAND_SCOPE_DENIED")
	}
	if in.Availability != "available" {
		return out, errors.New("DEVICE_CAPABILITY_UNAVAILABLE")
	}
	if in.Status != "online" {
		return out, errors.New("DEVICE_COMMAND_DEVICE_NOT_ONLINE")
	}
	returnHome := in.Capability == "flight.return_home"
	if in.ActiveTasks > 0 && !returnHome {
		return out, errors.New("DEVICE_COMMAND_ACTIVE_TASK_CONFLICT")
	}
	critical := in.Risk == "high" || in.Risk == "critical"
	if critical && in.Confirmation != fmt.Sprintf("CONFIRM %d %s", in.DeviceID, in.Capability) {
		return out, errors.New("DEVICE_COMMAND_CONFIRMATION_REQUIRED")
	}
	return CommandSafety{Allowed: true, ConfirmationRequired: critical, ActiveTaskOverride: returnHome && in.ActiveTasks > 0}, nil
}

type CommandGrant struct{ Action, Effect string }

// The HTTP command contract requires an explicit grant for members, including read capabilities.
func AuthorizeCommand(role, action string, grants []CommandGrant) error {
	allowed := role == "owner" || role == "admin"
	for _, grant := range grants {
		match := grant.Action == "*" || grant.Action == action || (strings.HasSuffix(grant.Action, ".*") && strings.HasPrefix(action, strings.TrimSuffix(grant.Action, "*")))
		if !match {
			continue
		}
		if grant.Effect == "deny" {
			return errors.New("DEVICE_CAPABILITY_EXPLICITLY_DENIED")
		}
		if grant.Effect == "allow" {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("DEVICE_CAPABILITY_NOT_GRANTED")
	}
	return nil
}
