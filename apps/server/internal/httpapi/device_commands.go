package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type deviceCommandInput struct {
	DeviceID                                int32
	Capability, Key, IdempotencyKey, Reason string
	Parameters                              map[string]any
	Confirmation                            *string
	Deadline                                float64
}

func parseDeviceCommand(c *gin.Context) (deviceCommandInput, error) {
	out := deviceCommandInput{Deadline: 30}
	bad := errors.New("DEVICE_COMMAND_INPUT_INVALID")
	id, err := strconv.ParseInt(c.Param("deviceId"), 10, 32)
	if err != nil || id <= 0 {
		return out, bad
	}
	out.DeviceID = int32(id)
	var body map[string]any
	decoder := json.NewDecoder(c.Request.Body)
	if decoder.Decode(&body) != nil {
		return out, bad
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return out, bad
	}
	for key, dest := range map[string]*string{"capabilityCode": &out.Capability, "commandKey": &out.Key, "idempotencyKey": &out.IdempotencyKey, "reason": &out.Reason} {
		v, ok := body[key].(string)
		if !ok {
			return out, bad
		}
		*dest = v
	}
	if strings.TrimSpace(out.Capability) == "" || strings.TrimSpace(out.Key) == "" || strings.TrimSpace(out.Reason) == "" || utf16Length(out.IdempotencyKey) < 8 || utf16Length(out.IdempotencyKey) > 128 {
		return out, bad
	}
	var ok bool
	out.Parameters, ok = body["parameters"].(map[string]any)
	if !ok || out.Parameters == nil {
		return out, bad
	}
	if confirmation, ok := body["confirmation"].(string); ok {
		out.Confirmation = &confirmation
	}
	if value, present := body["deadlineSeconds"]; present {
		seconds, ok := value.(float64)
		if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return out, bad
		}
		out.Deadline = seconds
	}
	out.Deadline = math.Min(300, math.Max(5, out.Deadline))
	return out, nil
}
func (s *Server) deviceCommandFailure(c *gin.Context, err error) {
	code := "DEVICE_COMMAND_FAILED"
	if err != nil {
		candidate := err.Error()
		switch candidate {
		case "DEVICE_CAPABILITY_NOT_FOUND", "DEVICE_COMMAND_SCOPE_DENIED", "DEVICE_CAPABILITY_UNAVAILABLE", "DEVICE_COMMAND_DEVICE_NOT_ONLINE", "DEVICE_COMMAND_ACTIVE_TASK_CONFLICT", "DEVICE_COMMAND_CONFIRMATION_REQUIRED", "DEVICE_CAPABILITY_EXPLICITLY_DENIED", "DEVICE_CAPABILITY_NOT_GRANTED", "PROJECT_ACCESS_DENIED", "device control is forbidden in replay mode":
			code = candidate
		}
	}
	status := 409
	if strings.Contains(code, "DENIED") || strings.Contains(code, "NOT_GRANTED") {
		status = 403
	}
	s.failure(c, status, code)
}
func (s *Server) submitDeviceCommand(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 400, "DEVICE_COMMAND_INPUT_INVALID")
		return
	}
	input, err := parseDeviceCommand(c)
	if err != nil {
		s.failure(c, 400, "DEVICE_COMMAND_INPUT_INVALID")
		return
	}
	if c.GetHeader("X-AeroSight-Mode") == "replay" || c.Query("mode") == "replay" {
		s.deviceCommandFailure(c, errors.New("device control is forbidden in replay mode"))
		return
	}
	uid := currentUser(c).ID
	access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view")
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("PROJECT_ACCESS_DENIED")
	}
	if err != nil {
		s.deviceCommandFailure(c, err)
		return
	}
	confirmationPresent := input.Confirmation != nil && *input.Confirmation != ""
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: input.IdempotencyKey, Action: "device_command.submit", ResourceType: "device", ResourceID: strconv.Itoa(int(input.DeviceID)), Input: gin.H{"deviceId": input.DeviceID, "capabilityCode": input.Capability, "commandKey": input.Key, "parameters": input.Parameters, "reason": input.Reason, "confirmationPresent": confirmationPresent}, PolicyResult: map[string]any{"boundary": "capability-rbac+safety-interlock+second-confirmation"}}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "project:view", false), func(w *database.WriteTx) (gin.H, error) {
		return s.writeDeviceCommand(c.Request.Context(), w, uid, pid, access.TeamID, input)
	})
	if err != nil {
		s.deviceCommandFailure(c, err)
		return
	}
	c.JSON(200, result)
}
func (s *Server) writeDeviceCommand(ctx context.Context, w *database.WriteTx, uid, pid, teamID int32, input deviceCommandInput) (gin.H, error) {
	q := w.Queries
	target, err := q.LockDeviceCommandTarget(ctx, sqlcgen.LockDeviceCommandTargetParams{ProjectID: pid, ID: input.DeviceID, CapabilityCode: input.Capability})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("DEVICE_CAPABILITY_NOT_FOUND")
	}
	if err != nil {
		return nil, err
	}
	// Use the role read under the transaction's membership lock, not the earlier HTTP precheck.
	membership, err := q.LockProjectMembership(ctx, sqlcgen.LockProjectMembershipParams{UserID: uid, ProjectID: pid})
	if err != nil {
		return nil, err
	}
	rawGrants, err := q.LockDeviceCommandGrants(ctx, sqlcgen.LockDeviceCommandGrantsParams{ProjectID: pid, TeamID: teamID, UserID: uid, DeviceTypeID: sql.NullInt64{Int64: target.DeviceTypeID, Valid: true}, DeviceID: sql.NullInt32{Int32: input.DeviceID, Valid: true}})
	if err != nil {
		return nil, err
	}
	grants := make([]device.CommandGrant, 0, len(rawGrants))
	for _, g := range rawGrants {
		grants = append(grants, device.CommandGrant{Action: g.ActionPattern, Effect: g.Effect})
	}
	if err = device.AuthorizeCommand(membership.Role, input.Capability, grants); err != nil {
		return nil, err
	}
	existing, err := q.FindExistingDeviceCommand(ctx, sqlcgen.FindExistingDeviceCommandParams{ProjectID: pid, DeviceID: input.DeviceID, IdempotencyKey: input.IdempotencyKey})
	if err == nil {
		return gin.H{"id": existing.ID, "status": existing.Status, "reused": true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	conflicts, err := q.CountDeviceCommandConflicts(ctx, sqlcgen.CountDeviceCommandConflictsParams{ProjectID: pid, SelectedDeviceID: sql.NullInt32{Int32: input.DeviceID, Valid: true}})
	if err != nil {
		return nil, err
	}
	confirmation := ""
	if input.Confirmation != nil {
		confirmation = *input.Confirmation
	}
	safety, err := device.CheckCommandSafety(device.CommandSafetyInput{ProjectID: pid, DeviceProjectID: target.ProjectID, DeviceID: input.DeviceID, Capability: input.Capability, Risk: target.RiskLevel, Availability: target.Availability, Status: target.Status, ActiveTasks: conflicts, Confirmation: confirmation})
	if err != nil {
		return nil, err
	}
	priority := int32(10)
	switch {
	case input.Capability == "flight.return_home":
		priority = 100
	case target.RiskLevel == "critical":
		priority = 90
	case target.RiskLevel == "high":
		priority = 80
	}
	safetyRaw, err := json.Marshal(gin.H{"reason": input.Reason, "confirmation": input.Confirmation, "confirmedByUserId": uid, "confirmedAt": timestamp(time.Now()), "allowed": safety.Allowed, "confirmationRequired": safety.ConfirmationRequired, "activeTaskOverride": safety.ActiveTaskOverride})
	if err != nil {
		return nil, err
	}
	parameters, err := json.Marshal(input.Parameters)
	if err != nil {
		return nil, err
	}
	commandID := uuid.New()
	command, err := q.InsertDeviceCommand(ctx, sqlcgen.InsertDeviceCommandParams{ID: commandID, ProjectID: pid, TeamID: teamID, DeviceID: input.DeviceID, CommandKey: input.Key, IdempotencyKey: input.IdempotencyKey, CapabilityCode: input.Capability, Parameters: parameters, Safety: safetyRaw, Priority: priority, DeadlineSeconds: input.Deadline, UserID: sql.NullInt32{Int32: uid, Valid: true}})
	if err != nil {
		return nil, err
	}
	if _, err = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: teamID, EventID: "device.command.dispatch:" + command.ID, EventType: "device.command.dispatch", Payload: gin.H{"commandId": command.ID}}); err != nil {
		return nil, err
	}
	return gin.H{"id": command.ID, "status": command.Status, "reused": command.ID != commandID.String()}, nil
}
