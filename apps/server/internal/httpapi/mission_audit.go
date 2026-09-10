package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/mission"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func (s *Server) missionAuditFailure(c *gin.Context, err error) {
	code, status := "MISSION_AUDIT_FAILED", 500
	if errors.Is(err, sql.ErrNoRows) {
		code, status = "TASK_RUN_NOT_FOUND", 404
	}
	if err != nil && err.Error() == "PROJECT_ACCESS_DENIED" {
		code, status = "PROJECT_ACCESS_DENIED", 403
	}
	s.failure(c, status, code)
}
func missionRunID(c *gin.Context) (int32, error) {
	id, err := strconv.ParseInt(c.Param("runId"), 10, 32)
	if err != nil || id <= 0 {
		return 0, sql.ErrNoRows
	}
	return int32(id), nil
}
func (s *Server) getMissionAuditTrace(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.missionAuditFailure(c, sql.ErrNoRows)
		return
	}
	id, err := missionRunID(c)
	if err != nil {
		s.missionAuditFailure(c, err)
		return
	}
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		s.missionAuditFailure(c, err)
		return
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err = s.projectAccess(ctx, q, currentUser(c).ID, pid, "safety:manage"); err != nil {
		s.missionAuditFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	result, err := readMissionAudit(ctx, q, pid, id)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.missionAuditFailure(c, err)
		return
	}
	c.JSON(200, result)
}
func readMissionAudit(ctx context.Context, q *sqlcgen.Queries, pid, id int32) (mission.AuditTrace, error) {
	var empty mission.AuditTrace
	run, err := q.GetMissionAuditRun(ctx, sqlcgen.GetMissionAuditRunParams{ProjectID: pid, ID: id})
	if err != nil {
		return empty, err
	}
	versionID := sql.NullString{}
	if run.TaskVersionID.Valid {
		versionID = sql.NullString{String: strconv.FormatInt(run.TaskVersionID.Int64, 10), Valid: true}
	}
	request, err := q.GetMissionAuditRequest(ctx, sqlcgen.GetMissionAuditRequestParams{ProjectID: pid, RunID: strconv.Itoa(int(id)), VersionID: versionID})
	var requested *mission.AuditRequest
	if err == nil {
		actorType := "user"
		if request.ActorAgentID.Int32 != 0 || run.TriggerSource == "agent" {
			actorType = "agent"
		}
		actorID := request.ActorUserID.Int32
		if request.ActorAgentID.Valid {
			actorID = request.ActorAgentID.Int32
		}
		requested = &mission.AuditRequest{RequestID: request.RequestID, Action: request.Action, ActorType: actorType, ActorID: actorID, CreatedAt: timestamp(request.CreatedAt)}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var approval *mission.AuditApproval
	if run.ApprovalRequestID.Valid {
		row, e := q.GetMissionAuditApproval(ctx, sqlcgen.GetMissionAuditApprovalParams{ProjectID: pid, ID: run.ApprovalRequestID.UUID})
		if e == nil {
			approval = &mission.AuditApproval{ID: row.ID.String(), Status: row.Status, RequiredApprovals: row.RequiredApprovals, ReceivedApprovals: row.ReceivedApprovals}
		} else if !errors.Is(e, sql.ErrNoRows) {
			return empty, e
		}
	}
	raw, err := q.GetMissionAuditCommands(ctx, sqlcgen.GetMissionAuditCommandsParams{ProjectID: pid, TaskRunID: sql.NullInt32{Int32: id, Valid: true}})
	if err != nil {
		return empty, err
	}
	commands := make([]mission.AuditCommand, 0, len(raw))
	for _, row := range raw {
		var command mission.AuditCommand
		if err = json.Unmarshal(row, &command); err != nil {
			return empty, err
		}
		commands = append(commands, command)
	}
	var preflight map[string]any
	if err = json.Unmarshal(run.PreflightSnapshotJson, &preflight); err != nil {
		return empty, err
	}
	checks, _ := preflight["checks"].([]any)
	policy := mission.AuditPreflight{Allowed: preflight["allowed"] == true, Checks: checks}
	if run.SafetyPolicyVersionID.Valid {
		value := strconv.FormatInt(run.SafetyPolicyVersionID.Int64, 10)
		policy.PolicyVersionID = &value
	}
	return mission.BuildAuditTrace(pid, id, run.TriggerSource, requested, policy, approval, commands), nil
}
func (s *Server) runEmergencyStopDrill(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.missionAuditFailure(c, sql.ErrNoRows)
		return
	}
	id, err := missionRunID(c)
	if err != nil {
		s.missionAuditFailure(c, err)
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		s.failure(c, 400, "EMERGENCY_STOP_DRILL_OUTCOME_INVALID")
		return
	}
	if body["dryRun"] != true {
		s.failure(c, 400, "EMERGENCY_STOP_DRILL_MUST_BE_DRY_RUN")
		return
	}
	outcome, _ := body["outcome"].(string)
	switch outcome {
	case "ack", "nack", "timeout", "disconnected":
	default:
		s.failure(c, 400, "EMERGENCY_STOP_DRILL_OUTCOME_INVALID")
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "safety:manage")
	if err != nil {
		s.missionAuditFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	requestID := c.GetHeader("X-Request-ID")
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: requestID, Action: "safety.emergency_stop_drill", ResourceType: "task_run", ResourceID: strconv.Itoa(int(id)), Input: gin.H{"dryRun": true, "outcome": outcome}, PolicyResult: map[string]any{"permission": "safety:manage", "effect": "none"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "safety:manage", false), func(w *database.WriteTx) (mission.AuditTrace, error) {
		row, e := w.Queries.GetEmergencyDrillDevice(ctx, sqlcgen.GetEmergencyDrillDeviceParams{ProjectID: pid, ID: id})
		if e != nil {
			return mission.AuditTrace{}, e
		}
		return mission.PlanEmergencyDrill(pid, id, uid, requestID, outcome, timestamp(time.Now()), row.Connected, row.CapabilityDeclared), nil
	})
	if err != nil {
		s.missionAuditFailure(c, err)
		return
	}
	c.JSON(200, result)
}
