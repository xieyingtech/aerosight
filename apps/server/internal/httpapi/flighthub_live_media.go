package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"regexp"
	"sort"

	"github.com/gin-gonic/gin"
)

var liveMediaSecretKey = regexp.MustCompile(`(?i)url|token|credential|password|secret|username|server(ip|port)|device(password|id)|local(port|channel)|rtsp`)

func safeLiveMediaSummary(value any) gin.H {
	out := gin.H{}
	items, _ := value.(map[string]any)
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := items[key]
		if liveMediaSecretKey.MatchString(key) {
			continue
		}
		switch value.(type) {
		case string, float64, bool:
			out[key] = value
		}
		if len(out) == 16 {
			break
		}
	}
	return out
}

func (s *Server) readFlightHubLiveMedia(c *gin.Context) {
	s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
		ctx := c.Request.Context()
		raw, err := q.ReadFlightHubMediaChannels(ctx, a.ProjectID)
		if err != nil {
			return nil, err
		}
		channels, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFlightHubMediaSessions(ctx, a.ProjectID)
		if err != nil {
			return nil, err
		}
		sessions, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadFlightHubMediaResources(ctx, a.ProjectID)
		if err != nil {
			return nil, err
		}
		resources, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		result := gin.H{"channels": channels, "sessions": sessions, "recordings": []gin.H{}, "shares": []gin.H{}, "converters": []gin.H{}}
		for _, row := range resources {
			item := gin.H{"id": row["id"], "connectorId": row["connectorId"], "status": row["status"], "summary": safeLiveMediaSummary(row["summary"]), "updatedAt": row["updatedAt"]}
			key := "converters"
			switch row["kind"] {
			case "recording":
				key = "recordings"
			case "live-share":
				key = "shares"
			}
			result[key] = append(result[key].([]gin.H), item)
		}
		return result, nil
	})
}
