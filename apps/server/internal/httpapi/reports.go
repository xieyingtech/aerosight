package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) reportFailure(c *gin.Context, err error, export bool) {
	code := "REPORT_PUBLISH_FAILED"
	if export {
		code = "REPORT_EXPORT_FAILED"
	}
	if err != nil {
		switch err.Error() {
		case "PROJECT_ACCESS_DENIED", "REPORT_DRAFT_NOT_FOUND", "REPORT_FAILED_NOT_PUBLISHABLE", "REPORT_INCOMPLETE_CONFIRMATION_REQUIRED", "PUBLISHED_REPORT_NOT_FOUND":
			code = err.Error()
		}
	}
	status := 409
	if export || code == "PROJECT_ACCESS_DENIED" {
		status = 403
	}
	s.failure(c, status, code)
}
func (s *Server) publishReport(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.reportFailure(c, errors.New("PROJECT_ACCESS_DENIED"), false)
		return
	}
	id, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		s.reportFailure(c, errors.New("REPORT_DRAFT_NOT_FOUND"), false)
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		s.failure(c, 400, "REPORT_PUBLISH_INPUT_INVALID")
		return
	}
	allow := body["allowIncomplete"] == true
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		s.reportFailure(c, errors.New("PROJECT_ACCESS_DENIED"), false)
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "generated_report.publish", ResourceType: "generated_report", ResourceID: id.String(), Input: gin.H{"allowIncomplete": allow}, PolicyResult: map[string]any{"permission": "mission:operate"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		if _, e := w.Queries.LockReportPublication(ctx, sqlcgen.LockReportPublicationParams{ProjectID: pid, ID: id}); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				e = errors.New("REPORT_DRAFT_NOT_FOUND")
			}
			return nil, e
		}
		draft, e := w.Queries.LockReportDraft(ctx, sqlcgen.LockReportDraftParams{ProjectID: pid, GeneratedReportID: id})
		if errors.Is(e, sql.ErrNoRows) {
			e = errors.New("REPORT_DRAFT_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		if draft.Completeness == "failed" {
			return nil, errors.New("REPORT_FAILED_NOT_PUBLISHABLE")
		}
		if draft.Completeness == "incomplete" && !allow {
			return nil, errors.New("REPORT_INCOMPLETE_CONFIRMATION_REQUIRED")
		}
		var content struct {
			Evidence []struct {
				AssetID *int32 `json:"assetId"`
			} `json:"evidence"`
		}
		if e = json.Unmarshal(draft.ContentJson, &content); e != nil {
			return nil, e
		}
		retained := []int32{}
		seen := map[int32]bool{}
		for _, ref := range content.Evidence {
			if ref.AssetID != nil && *ref.AssetID != 0 && !seen[*ref.AssetID] {
				retained = append(retained, *ref.AssetID)
				seen[*ref.AssetID] = true
			}
		}
		if e = w.Queries.RetirePublishedReport(ctx, sqlcgen.RetirePublishedReportParams{ProjectID: pid, GeneratedReportID: id}); e != nil {
			return nil, e
		}
		if e = w.Queries.PublishReportVersion(ctx, sqlcgen.PublishReportVersionParams{ProjectID: pid, ID: draft.ID, PublishedByUserID: sql.NullInt32{Int32: uid, Valid: true}}); e != nil {
			return nil, e
		}
		if e = w.Queries.PointPublishedReport(ctx, sqlcgen.PointPublishedReportParams{ProjectID: pid, ID: id, CurrentPublishedVersionID: uuid.NullUUID{UUID: draft.ID, Valid: true}}); e != nil {
			return nil, e
		}
		if len(retained) > 0 {
			if e = w.Queries.RetainReportAssets(ctx, sqlcgen.RetainReportAssetsParams{ProjectID: pid, ReportID: id.String(), AssetIds: retained}); e != nil {
				return nil, e
			}
		}
		return gin.H{"reportId": id.String(), "versionId": draft.ID.String(), "status": "published", "completeness": draft.Completeness, "retainedAssetIds": retained}, nil
	})
	if err != nil {
		s.reportFailure(c, err, false)
		return
	}
	c.JSON(200, result)
}
func (s *Server) exportReport(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.reportFailure(c, errors.New("PROJECT_ACCESS_DENIED"), true)
		return
	}
	id, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		s.reportFailure(c, errors.New("PUBLISHED_REPORT_NOT_FOUND"), true)
		return
	}
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		s.reportFailure(c, err, true)
		return
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err = s.projectAccess(ctx, q, currentUser(c).ID, pid, "report:export"); err != nil {
		s.reportFailure(c, errors.New("PROJECT_ACCESS_DENIED"), true)
		return
	}
	raw, err := q.ExportPublishedReport(ctx, sqlcgen.ExportPublishedReportParams{ProjectID: pid, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("PUBLISHED_REPORT_NOT_FOUND")
	}
	if err != nil {
		s.reportFailure(c, err, true)
		return
	}
	rows, err := decodeSnapshotRows([]json.RawMessage{raw})
	if err != nil {
		s.reportFailure(c, err, true)
		return
	}
	output, err := json.MarshalIndent(rows[0], "", "  ")
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.reportFailure(c, err, true)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="aerosight-report-`+id.String()+`.json"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "application/json; charset=utf-8", output)
}
