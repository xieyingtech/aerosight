package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) getInspectionObservation(c *gin.Context) {
	s.getInspectionSnapshot(c, "observationId", "select manifest_json from inspection_observations where project_id=$1 and team_id=$2 and id=$3 and sealed_at is not null", "INSPECTION_OBSERVATION")
}

func (s *Server) getInspectionEvidenceSet(c *gin.Context) {
	s.getInspectionSnapshot(c, "evidenceSetId", "select evidence_json from inspection_evidence_sets where project_id=$1 and team_id=$2 and id=$3", "INSPECTION_EVIDENCE_SET")
}

// Queries are fixed server-owned statements; no table or column is client supplied.
func (s *Server) getInspectionSnapshot(c *gin.Context, parameter, query, code string) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, code+"_NOT_FOUND")
		return
	}
	id, err := uuid.Parse(c.Param(parameter))
	if err != nil {
		s.failure(c, 404, code+"_NOT_FOUND")
		return
	}
	access, err := s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	var manifest []byte
	err = s.db.QueryRowContext(c.Request.Context(), query, pid, access.TeamID, id).Scan(&manifest)
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, code+"_NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, code+"_READ_FAILED")
		return
	}
	c.JSON(200, json.RawMessage(manifest))
}
