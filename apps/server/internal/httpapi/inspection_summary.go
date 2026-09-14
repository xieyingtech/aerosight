package httpapi

import (
	"aerosight/server/internal/report"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
)

func (s *Server) readInspectionSummary(c *gin.Context) {
	pid, err := projectID(c)
	runID, runErr := missionRunID(c)
	if err != nil || runErr != nil {
		s.failure(c, 404, "TASK_RUN_NOT_FOUND")
		return
	}
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.failure(c, 500, "INSPECTION_SUMMARY_READ_FAILED")
		return
	}
	defer tx.Rollback()
	if _, err = s.projectAccess(ctx, s.queries.WithTx(tx), currentUser(c).ID, pid, "project:view"); err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	summary, err := report.ReadInspectionSummary(ctx, tx, int(pid), int(runID))
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "TASK_RUN_NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, "INSPECTION_SUMMARY_READ_FAILED")
		return
	}
	if err = tx.Commit(); err != nil {
		s.failure(c, 500, "INSPECTION_SUMMARY_READ_FAILED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, summary)
}
