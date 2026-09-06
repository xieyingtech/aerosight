package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (s *Server) issueReadRoutes() {
	group := s.router.Group("/api/projects/:id/issues", s.requireUser, s.timeout)
	group.GET("", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.ListProjectIssues(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			return decodeSnapshotRows(raw)
		})
	})
	group.GET("/:issueId", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("issueId"), 10, 32)
		if err != nil || id <= 0 {
			s.failure(c, 404, "ISSUE_NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			ctx := c.Request.Context()
			pid := a.ProjectID
			iid := int32(id)
			raw, err := q.ReadProjectIssue(ctx, sqlcgen.ReadProjectIssueParams{ProjectID: pid, ID: iid})
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows([]json.RawMessage{raw})
			if err != nil {
				return nil, err
			}
			result := gin.H{"issue": rows[0]}
			reads := []struct {
				key   string
				query func() ([]json.RawMessage, error)
			}{
				{"events", func() ([]json.RawMessage, error) {
					return q.ReadIssueEvents(ctx, sqlcgen.ReadIssueEventsParams{ProjectID: pid, IssueID: iid})
				}},
				{"links", func() ([]json.RawMessage, error) {
					return q.ReadIssueLinks(ctx, sqlcgen.ReadIssueLinksParams{ProjectID: pid, IssueID: iid})
				}},
				{"detections", func() ([]json.RawMessage, error) {
					return q.ReadIssueDetections(ctx, sqlcgen.ReadIssueDetectionsParams{ProjectID: pid, IssueID: iid})
				}},
				{"assets", func() ([]json.RawMessage, error) {
					return q.ReadIssueAssets(ctx, sqlcgen.ReadIssueAssetsParams{ProjectID: pid, IssueID: iid})
				}},
				{"assignees", func() ([]json.RawMessage, error) {
					return q.ReadIssueAssignees(ctx, sqlcgen.ReadIssueAssigneesParams{ProjectID: pid, IssueID: iid})
				}},
				{"members", func() ([]json.RawMessage, error) { return q.ReadIssueMembers(ctx, pid) }},
				{"agents", func() ([]json.RawMessage, error) { return q.ReadIssueAgents(ctx, pid) }},
				{"drafts", func() ([]json.RawMessage, error) {
					return q.ReadIssueDrafts(ctx, sqlcgen.ReadIssueDraftsParams{ProjectID: pid, IssueID: sql.NullInt32{Int32: iid, Valid: true}})
				}},
			}
			for _, read := range reads {
				raw, e := read.query()
				if e != nil {
					return nil, e
				}
				rows, e := decodeSnapshotRows(raw)
				if e != nil {
					return nil, e
				}
				result[read.key] = rows
			}
			permissions := effectivePermissions(a.Role, a.Permissions)
			result["canHandle"] = permissions["issue:handle"]
			result["canAssign"] = permissions["issue:assign"]
			result["canUseAgent"] = permissions["agent:use"]
			return result, nil
		})
	})
}
