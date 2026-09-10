package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"strconv"
)

func (s *Server) legacyEventRoutes() {
	group := s.router.Group("/api/projects/:id/events/:eventId", s.requireUser, s.timeout)
	readonly := func(c *gin.Context) { c.JSON(410, gin.H{"error": "LEGACY_EVENT_READ_ONLY", "issuesHref": "../issues"}) }
	group.POST("/actions", readonly)
	group.POST("/agent-drafts", readonly)
	group.GET("", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("eventId"))
		if err != nil {
			s.failure(c, 404, "PERCEPTION_EVENT_NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.GetLegacyPerceptionEvent(c.Request.Context(), sqlcgen.GetLegacyPerceptionEventParams{ProjectID: a.ProjectID, ID: id})
			if err != nil {
				return nil, err
			}
			events, err := decodeSnapshotRows([]json.RawMessage{raw})
			if err != nil {
				return nil, err
			}
			event := events[0]
			groupID, err := strconv.ParseInt(event["detectionGroupId"].(string), 10, 64)
			if err != nil {
				return nil, err
			}
			rawDetections, err := q.GetLegacyPerceptionDetections(c.Request.Context(), sqlcgen.GetLegacyPerceptionDetectionsParams{ProjectID: a.ProjectID, DetectionGroupID: groupID})
			if err != nil {
				return nil, err
			}
			detections, err := decodeSnapshotRows(rawDetections)
			if err != nil {
				return nil, err
			}
			mapped := 0
			for _, d := range detections {
				if d["locationQuality"] == nil {
					d["locationQuality"] = "unavailable"
				}
				if d["projectionMethod"] == nil {
					d["projectionMethod"] = "image-only"
				}
				if d["geographicGeometry"] != nil {
					mapped++
				}
			}
			event["title"] = "疑似违建"
			event["disclaimer"] = "该结果为算法生成的巡检线索，不构成法律意义上的违建认定。"
			event["hasMapLocation"] = mapped > 0
			event["locationSummary"] = "位置不可用，仅展示影像内标注"
			if mapped > 0 {
				event["locationSummary"] = fmt.Sprintf("%d 条检测具有可用地理位置", mapped)
			}
			rawFeedback, err := q.GetLegacyPerceptionFeedback(c.Request.Context(), sqlcgen.GetLegacyPerceptionFeedbackParams{ProjectID: a.ProjectID, PerceptionEventID: id})
			if err != nil {
				return nil, err
			}
			feedback, err := decodeSnapshotRows(rawFeedback)
			if err != nil {
				return nil, err
			}
			actions := []string{}
			if effectivePermissions(a.Role, a.Permissions)["event:handle"] && event["status"] != "resolved" && event["status"] != "dismissed" {
				actions = []string{"confirm", "false_positive", "category_correction", "assign", "investigate", "dismiss", "resolve"}
			}
			return gin.H{"event": event, "detections": detections, "feedback": feedback, "actions": actions}, nil
		})
	})
}
