package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/mission"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"math"
	"strconv"
	"strings"
)

func (s *Server) missionControlFailure(c *gin.Context, err error) {
	code := "MISSION_CONTROL_FAILED"
	if err != nil {
		value := err.Error()
		switch value {
		case "PROJECT_ACCESS_DENIED", "TASK_RUN_NOT_FOUND", "TASK_RUN_VERSION_CONFLICT", "TASK_RUN_APPROVAL_NOT_REQUIRED", "TASK_RUN_TRANSITION_REASON_REQUIRED", "INSPECTION_REVIEW_REQUIRED":
			code = value
		default:
			if strings.HasPrefix(value, "TASK_RUN_TRANSITION_INVALID:") {
				code = value
			}
		}
	}
	status := 409
	if code == "PROJECT_ACCESS_DENIED" {
		status = 403
	}
	s.failure(c, status, code)
}
func (s *Server) controlMissionRun(c *gin.Context) {
	bad := func() { s.failure(c, 400, "MISSION_CONTROL_INPUT_INVALID") }
	pid, err := projectID(c)
	if err != nil {
		bad()
		return
	}
	id, err := strconv.ParseInt(c.Param("runId"), 10, 32)
	if err != nil || id <= 0 {
		bad()
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		bad()
		return
	}
	action, _ := body["action"].(string)
	switch action {
	case "pause", "resume", "cancel", "emergency_stop", "approve":
	default:
		bad()
		return
	}
	reason, _ := body["reason"].(string)
	if action != "emergency_stop" && s.limitUser(c, s.writeRate) {
		return
	}
	version, ok := body["expectedVersion"].(float64)
	if !ok || math.Trunc(version) != version || version < math.MinInt32 || version > math.MaxInt32 || strings.TrimSpace(reason) == "" {
		bad()
		return
	}
	permission := "mission:operate"
	if action == "approve" {
		permission = "mission:approve"
	}
	uid := currentUser(c).ID
	access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, permission)
	if err != nil {
		s.missionControlFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	input := gin.H{"projectId": pid, "taskRunId": int32(id), "expectedVersion": int32(version), "action": action, "reason": reason}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "task_run." + action, ResourceType: "task_run", ResourceID: strconv.FormatInt(id, 10), Input: input, PolicyResult: map[string]any{"permission": permission}}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, permission, false), func(w *database.WriteTx) (gin.H, error) {
		row, e := w.Queries.LockMissionControlRun(c.Request.Context(), sqlcgen.LockMissionControlRunParams{ProjectID: pid, ID: int32(id)})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("TASK_RUN_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		if row.StateVersion != int32(version) {
			return nil, errors.New("TASK_RUN_VERSION_CONFLICT")
		}
		if action == "resume" {
			var needsReview bool
			if e = w.Tx.QueryRowContext(c.Request.Context(), `select exists(select 1 from inspection_assessments where project_id=$1 and task_run_id=$2 and status='needs_review')`, pid, id).Scan(&needsReview); e != nil {
				return nil, e
			}
			if needsReview {
				return nil, errors.New("INSPECTION_REVIEW_REQUIRED")
			}
		}
		if action == "approve" {
			if !row.ApprovalRequestID.Valid {
				return nil, errors.New("TASK_RUN_APPROVAL_NOT_REQUIRED")
			}
			if e = w.Queries.ApproveMissionControlRun(c.Request.Context(), sqlcgen.ApproveMissionControlRunParams{ProjectID: pid, TeamID: access.TeamID, ApprovalRequestID: row.ApprovalRequestID.UUID, ApproverUserID: uid, Reason: reason}); e != nil {
				return nil, e
			}
			return gin.H{"id": int32(id), "status": row.Status, "stateVersion": row.StateVersion, "approval": "approved"}, nil
		}
		next, e := mission.ControlNextStatus(row.Status, action, reason)
		if e != nil {
			return nil, e
		}
		updated, e := w.Queries.UpdateMissionControlRun(c.Request.Context(), sqlcgen.UpdateMissionControlRunParams{ProjectID: pid, ID: int32(id), StateVersion: int32(version), Status: next, StateReason: sql.NullString{String: reason, Valid: true}})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("TASK_RUN_VERSION_CONFLICT")
		}
		if e != nil {
			return nil, e
		}
		eventType := "task_run.transitioned"
		payload := gin.H{"taskRunId": int32(id), "from": row.Status, "to": next, "stateVersion": updated.StateVersion, "reason": reason}
		if (action == "cancel" && next == "canceling") || action == "emergency_stop" {
			eventType = "mission.control"
			payload = gin.H{"taskRunId": int32(id), "control": action}
		}
		if _, e = w.Publish(c.Request.Context(), database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: uuid.NewString(), EventType: eventType, Payload: payload}); e != nil {
			return nil, e
		}
		return gin.H{"id": updated.ID, "status": updated.Status, "stateVersion": updated.StateVersion}, nil
	})
	if err != nil {
		s.missionControlFailure(c, err)
		return
	}
	c.JSON(200, result)
}
