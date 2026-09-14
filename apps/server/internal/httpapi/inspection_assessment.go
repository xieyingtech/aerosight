package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"aerosight/server/internal/database"
	"aerosight/server/internal/inspection"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) inspectionAssessmentFailure(c *gin.Context, err error) {
	code, status := err.Error(), 400
	if errors.Is(err, sql.ErrNoRows) {
		code, status = "INSPECTION_ASSESSMENT_NOT_FOUND", 404
	} else if strings.Contains(code, "CONFLICT") || strings.Contains(code, "CANCELED") || strings.Contains(code, "EXPIRED") || strings.Contains(code, "NOT_PENDING") {
		status = 409
	} else if strings.Contains(code, "ACCESS_DENIED") {
		status = 403
	} else if !strings.HasPrefix(code, "INSPECTION_") {
		code, status = "INSPECTION_ASSESSMENT_FAILED", 500
	}
	s.failure(c, status, code)
}

func (s *Server) inspectionAssessment(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "INSPECTION_ASSESSMENT_NOT_FOUND")
		return
	}
	id, err := uuid.Parse(c.Param("assessmentId"))
	if err != nil {
		s.failure(c, 404, "INSPECTION_ASSESSMENT_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	permission := "project:view"
	if c.Request.Method == "POST" {
		permission = "issue:handle"
	}
	access, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, permission)
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	if c.Request.Method == "GET" {
		var raw []byte
		err = s.db.QueryRowContext(c.Request.Context(), `select jsonb_build_object('id',a.id,'taskRunId',a.task_run_id,'taskRunStepId',a.task_run_step_id,'evidenceSetId',a.evidence_set_id,'status',a.status,'runStatus',(select status from task_runs where id=a.task_run_id and project_id=a.project_id),'revision',a.revision,'providerId',a.provider_id,'modelVersion',a.model_version,'promptVersion',a.prompt_version,'evidenceHash',a.evidence_hash,'originalOutput',a.original_output,'failureCode',a.failure_code,'revisions',coalesce((select jsonb_agg(jsonb_build_object('revision',r.revision,'source',r.source,'decisions',r.decisions_json,'reviewedByUserId',r.reviewed_by_user_id,'createdAt',r.created_at) order by r.revision) from inspection_assessment_revisions r where r.assessment_id=a.id and r.project_id=a.project_id),'[]'::jsonb)) from inspection_assessments a where a.id=$1 and a.project_id=$2 and a.team_id=$3`, id, pid, access.TeamID).Scan(&raw)
		if err != nil {
			s.inspectionAssessmentFailure(c, err)
			return
		}
		var model map[string]any
		if json.Unmarshal(raw, &model) != nil {
			s.failure(c, 500, "INSPECTION_ASSESSMENT_FAILED")
			return
		}
		model["canReview"] = effectivePermissions(access.Role, access.Permissions)["issue:handle"]
		c.JSON(200, model)
		return
	}
	var input inspection.ReviewInput
	if strictJSON(c, &input) != nil {
		s.failure(c, 400, "INSPECTION_REVIEW_INPUT_INVALID")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "inspection.assessment.review", ResourceType: "inspection_assessment", ResourceID: id.String(), Input: input}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, permission, false), func(w *database.WriteTx) (inspection.ReviewResult, error) {
		return inspection.ReviewAssessment(c.Request.Context(), w.Tx, inspection.Scope{ProjectID: int(pid), TeamID: int(access.TeamID)}, uid, id.String(), input)
	})
	if err != nil {
		s.inspectionAssessmentFailure(c, err)
		return
	}
	c.JSON(200, result)
}
