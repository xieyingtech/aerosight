package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) scopedRead(c *gin.Context, read func(*sqlcgen.Queries, sqlcgen.GetProjectAccessRow) (any, error)) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	tx, err := s.db.BeginTx(c.Request.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.failure(c, 500, "QUERY_FAILED")
		return
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	access, err := s.projectAccess(c.Request.Context(), q, currentUser(c).ID, pid, "project:view")
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, "QUERY_FAILED")
		return
	}
	result, err := read(q, access)
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, "QUERY_FAILED")
		return
	}
	if err = tx.Commit(); err != nil {
		s.failure(c, 500, "QUERY_FAILED")
		return
	}
	c.JSON(200, result)
}
func (s *Server) projectReadRoutes() {
	api := s.router.Group("/api/projects/:id", s.requireUser, s.timeout)
	api.GET("/devices", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.ListProjectDevices(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			return decodeSnapshotRows(raw)
		})
	})
	api.GET("/devices/:deviceId", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("deviceId"), 10, 32)
		if err != nil || id <= 0 {
			s.failure(c, 404, "NOT_FOUND")
			return
		}
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.GetProjectDevice(c.Request.Context(), sqlcgen.GetProjectDeviceParams{ProjectID: a.ProjectID, ID: int32(id)})
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows([]json.RawMessage{raw})
			if err != nil {
				return nil, err
			}
			return rows[0], nil
		})
	})
	api.GET("/device-tree", s.deviceTree)
	api.GET("/replay", s.projectReplay)
}
func (s *Server) deviceTree(c *gin.Context) {
	s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
		raw, err := q.ReadDeviceTree(c.Request.Context(), a.ProjectID)
		if err != nil {
			return nil, err
		}
		devices, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.ReadDeviceRelations(c.Request.Context(), a.ProjectID)
		if err != nil {
			return nil, err
		}
		relations, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		raw, err = q.SnapshotDeviceGrants(c.Request.Context(), sqlcgen.SnapshotDeviceGrantsParams{ProjectID: a.ProjectID, TeamID: a.TeamID, UserID: currentUser(c).ID})
		if err != nil {
			return nil, err
		}
		grants, err := decodeSnapshotRows(raw)
		if err != nil {
			return nil, err
		}
		for _, device := range devices {
			device["rawCapabilities"] = device["capabilities"]
			device["rawChannels"] = device["channels"]
			projectCapabilities(device, a.Role, grants)
		}
		return buildDeviceTree(devices, relations), nil
	})
}
func buildDeviceTree(devices, relations []gin.H) []gin.H {
	byID := map[any]gin.H{}
	parent := map[any]gin.H{}
	for _, d := range devices {
		byID[d["id"]] = d
	}
	for _, r := range relations {
		from, to := r["fromDeviceId"], r["toDeviceId"]
		if byID[from] != nil && byID[to] != nil && from != to && parent[to] == nil {
			parent[to] = r
		}
	}
	var build func(gin.H, map[any]bool) gin.H
	build = func(device gin.H, ancestors map[any]bool) gin.H {
		next := map[any]bool{}
		for key, v := range ancestors {
			next[key] = v
		}
		next[device["id"]] = true
		children := []gin.H{}
		for _, r := range relations {
			if r["fromDeviceId"] == device["id"] && !next[r["toDeviceId"]] && byID[r["toDeviceId"]] != nil {
				children = append(children, build(byID[r["toDeviceId"]], next))
			}
		}
		out := gin.H{}
		for key, v := range device {
			out[key] = v
		}
		out["relationType"] = nil
		if p := parent[device["id"]]; p != nil {
			out["relationType"] = p["relationType"]
		}
		out["children"] = children
		return out
	}
	roots := []gin.H{}
	for _, d := range devices {
		if parent[d["id"]] == nil {
			roots = append(roots, build(d, map[any]bool{}))
		}
	}
	reachable := map[any]bool{}
	var visit func(gin.H)
	visit = func(n gin.H) {
		reachable[n["id"]] = true
		for _, child := range n["children"].([]gin.H) {
			visit(child)
		}
	}
	for _, root := range roots {
		visit(root)
	}
	for _, d := range devices {
		if !reachable[d["id"]] {
			roots = append(roots, build(d, map[any]bool{}))
		}
	}
	return roots
}

type replayInput struct {
	From, To    time.Time
	DeviceTypes []string
	BBox        []float64
}

func parseReplay(values url.Values, now time.Time) (replayInput, error) {
	out := replayInput{To: now, DeviceTypes: []string{}}
	parseDate := func(raw string) (time.Time, error) {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02"} {
			if date, err := time.Parse(layout, raw); err == nil {
				return date, nil
			}
		}
		return time.Time{}, errors.New("INVALID_REPLAY_WINDOW")
	}
	var err error
	if raw, ok := values["to"]; ok {
		out.To, err = parseDate(raw[0])
		if err != nil {
			return out, err
		}
	}
	out.From = out.To.Add(-time.Hour)
	if raw, ok := values["from"]; ok {
		out.From, err = parseDate(raw[0])
		if err != nil {
			return out, err
		}
	}
	if !out.From.Before(out.To) {
		return out, errors.New("INVALID_REPLAY_WINDOW")
	}
	if out.To.Sub(out.From) > 7*24*time.Hour {
		return out, errors.New("REPLAY_WINDOW_TOO_LARGE")
	}
	for _, value := range strings.Split(values.Get("deviceTypes"), ",") {
		if v := strings.TrimSpace(value); v != "" {
			out.DeviceTypes = append(out.DeviceTypes, v)
		}
	}
	if raw := values.Get("bbox"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) != 4 {
			return out, errors.New("INVALID_REPLAY_BBOX")
		}
		for _, part := range parts {
			v := 0.0
			if strings.TrimSpace(part) != "" {
				v, err = strconv.ParseFloat(strings.TrimSpace(part), 64)
			}
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return out, errors.New("INVALID_REPLAY_BBOX")
			}
			out.BBox = append(out.BBox, v)
		}
		b := out.BBox
		if b[0] >= b[2] || b[1] >= b[3] || b[0] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 {
			return out, errors.New("INVALID_REPLAY_BBOX")
		}
	}
	return out, nil
}
func (s *Server) projectReplay(c *gin.Context) {
	if _, err := projectID(c); err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	input, err := parseReplay(c.Request.URL.Query(), time.Now())
	if err != nil {
		s.failure(c, 400, err.Error())
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-AeroSight-Mode", "replay")
	s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
		rawPoses, err := q.ReplayPoses(c.Request.Context(), sqlcgen.ReplayPosesParams{ProjectID: a.ProjectID, CapturedAt: input.From, CapturedAt_2: input.To, Column4: input.DeviceTypes, Column5: input.BBox})
		if err != nil {
			return nil, err
		}
		rawMedia, err := q.ReplayMedia(c.Request.Context(), sqlcgen.ReplayMediaParams{ProjectID: a.ProjectID, CapturedAt: sql.NullTime{Time: input.From, Valid: true}, CapturedAt_2: sql.NullTime{Time: input.To, Valid: true}})
		if err != nil {
			return nil, err
		}
		rawEvents, err := q.ReplayEvents(c.Request.Context(), sqlcgen.ReplayEventsParams{ProjectID: a.ProjectID, OccurredAt: input.From, OccurredAt_2: input.To})
		if err != nil {
			return nil, err
		}
		truncated := len(rawPoses) > 5000 || len(rawMedia) > 1000 || len(rawEvents) > 2000
		poses, err := decodeSnapshotRows(rawPoses[:min(len(rawPoses), 5000)])
		if err != nil {
			return nil, err
		}
		media, err := decodeSnapshotRows(rawMedia[:min(len(rawMedia), 1000)])
		if err != nil {
			return nil, err
		}
		events, err := decodeSnapshotRows(rawEvents[:min(len(rawEvents), 2000)])
		if err != nil {
			return nil, err
		}
		return gin.H{"projectId": a.ProjectID, "mode": "replay", "window": gin.H{"from": timestamp(input.From), "to": timestamp(input.To)}, "filters": gin.H{"deviceTypes": input.DeviceTypes, "bbox": input.BBox}, "poses": poses, "media": media, "events": events, "truncated": truncated}, nil
	})
}
