package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/observability"
	"github.com/gin-gonic/gin"
)

func (s *Server) streamRoutes() {
	s.router.GET("/api/projects/:id/events", s.requireUser, s.projectStream)
	s.router.GET("/api/projects/:id/realtime-channels/:channelId/events", s.requireUser, s.channelStream)
}

func streamCursor(c *gin.Context) int64 {
	value := c.Query("cursor")
	if v, ok := c.Request.Header["Last-Event-Id"]; ok && len(v) > 0 {
		value = v[0]
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func beginStream(c *gin.Context) {
	outcome := "opened"
	if streamCursor(c) > 0 {
		outcome = "resumed"
	}
	_ = observability.DefaultMetrics.Record("aerosight_sse_connections_total", 1, map[string]string{"outcome": outcome})
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "private, no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
}

func endStream() {
	_ = observability.DefaultMetrics.Record("aerosight_sse_connections_total", 1, map[string]string{"outcome": "closed"})
}

func streamFrame(c *gin.Context, id, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	frame := ""
	if id != "" {
		frame = "id: " + id + "\n"
	}
	frame += "event: " + event + "\ndata: " + string(payload) + "\n\n"
	return streamWrite(c, frame)
}
func streamWrite(c *gin.Context, frame string) error {
	rc := http.NewResponseController(c.Writer)
	// Bound slow-client writes without imposing a lifetime on the subscription.
	if err := rc.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := c.Writer.WriteString(frame); err != nil {
		return err
	}
	return rc.Flush()
}

func (s *Server) streamSessionActive(ctx context.Context, token string, uid int32) bool {
	// A fresh context is essential: SCS otherwise returns the cached request session.
	fresh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	loaded, err := s.sessions.Load(fresh, token)
	return err == nil && s.sessions.GetInt(loaded, "userId") == int(uid)
}

func (s *Server) projectStream(c *gin.Context) {
	pid, err := projectID(c)
	uid := currentUser(c).ID
	if err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	if _, err = s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view"); err != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	cursor := streamCursor(c)
	token := s.sessions.Token(c.Request.Context())
	beginStream(c)
	defer endStream()
	if streamWrite(c, ": heartbeat\n\n") != nil {
		return
	}
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		rows, err := s.queries.ReadProjectEvents(c.Request.Context(), sqlcgen.ReadProjectEventsParams{ProjectID: pid, Column2: cursor})
		if err != nil {
			return
		}
		if len(rows) > 500 {
			last := rows[len(rows)-1].Cursor
			_ = streamFrame(c, last, "snapshot.required", gin.H{"reason": "replay_window_exceeded", "snapshotUrl": fmt.Sprintf("/api/projects/%d/snapshot", pid), "resumeCursor": last})
			return
		}
		for _, r := range rows {
			if streamFrame(c, r.Cursor, r.EventType, gin.H{"eventId": r.EventID, "type": r.EventType, "payload": r.PayloadJson, "occurredAt": r.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z")}) != nil {
				return
			}
			cursor, _ = strconv.ParseInt(r.Cursor, 10, 64)
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-poll.C:
		case <-heartbeat.C:
			_, err := s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view")
			if err != nil || !s.streamSessionActive(c.Request.Context(), token, uid) {
				_ = streamFrame(c, "", "access.revoked", gin.H{"reason": "project_membership_revoked"})
				return
			}
			if streamWrite(c, ": heartbeat\n\n") != nil {
				return
			}
		}
	}
}

func (s *Server) authorizedChannel(ctx context.Context, uid, pid int32, cid string) (sqlcgen.ResolveChannelRow, error) {
	target, err := s.queries.ResolveChannel(ctx, sqlcgen.ResolveChannelParams{UserID: uid, ProjectID: pid, ChannelID: cid})
	if err != nil {
		return target, err
	}
	if !strings.HasPrefix(target.CapabilityCode, "stream.") || target.Availability == "unavailable" {
		return target, errors.New("REALTIME_CHANNEL_UNAVAILABLE")
	}
	grants, err := s.queries.ChannelGrants(ctx, sqlcgen.ChannelGrantsParams{UserID: uid, ProjectID: pid, DeviceTypeID: sql.NullInt64{Int64: target.DeviceTypeID, Valid: true}, DeviceID: sql.NullInt32{Int32: target.DeviceID, Valid: true}})
	if err != nil {
		return target, err
	}
	allowed := target.Role == "owner" || target.Role == "admin"
	for _, g := range grants {
		matches := g.ActionPattern == "*" || g.ActionPattern == target.CapabilityCode || (strings.HasSuffix(g.ActionPattern, ".*") && strings.HasPrefix(target.CapabilityCode, strings.TrimSuffix(g.ActionPattern, "*")))
		if !matches {
			continue
		}
		if g.Effect == "deny" {
			return target, errors.New("REALTIME_SUBSCRIPTION_EXPLICITLY_DENIED")
		}
		if g.Effect == "allow" {
			allowed = true
		}
	}
	if !allowed {
		return target, errors.New("REALTIME_SUBSCRIPTION_NOT_GRANTED")
	}
	return target, nil
}

func (s *Server) channelStream(c *gin.Context) {
	pid, err := projectID(c)
	uid := currentUser(c).ID
	cid := c.Param("channelId")
	if err != nil || len(cid) == 0 || len(cid) > 512 {
		s.failure(c, 404, "REALTIME_CHANNEL_NOT_FOUND")
		return
	}
	target, err := s.authorizedChannel(c.Request.Context(), uid, pid, cid)
	if err != nil {
		s.failure(c, 404, "REALTIME_CHANNEL_NOT_FOUND")
		return
	}
	cursor := streamCursor(c)
	token := s.sessions.Token(c.Request.Context())
	beginStream(c)
	defer endStream()
	if streamWrite(c, ": heartbeat\n\n") != nil {
		return
	}
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	auth := time.NewTicker(5 * time.Second)
	defer auth.Stop()
	heart := time.NewTicker(15 * time.Second)
	defer heart.Stop()
	for {
		var rows []sqlcgen.ReadChannelTelemetryRow
		if target.DataType == "telemetry" || target.DataType == "sensor" {
			rows, err = s.queries.ReadChannelTelemetry(c.Request.Context(), sqlcgen.ReadChannelTelemetryParams{ProjectID: pid, DeviceID: target.DeviceID, Column3: cursor})
		} else {
			var events []sqlcgen.ReadChannelEventsRow
			events, err = s.queries.ReadChannelEvents(c.Request.Context(), sqlcgen.ReadChannelEventsParams{ProjectID: pid, Column2: strconv.Itoa(int(target.DeviceID)), Column3: cursor})
			for _, r := range events {
				rows = append(rows, sqlcgen.ReadChannelTelemetryRow(r))
			}
		}
		if err != nil {
			return
		}
		if len(rows) > 500 {
			_ = streamFrame(c, "", "stream.closed", gin.H{"reason": "backpressure_limit_exceeded"})
			return
		}
		for _, r := range rows {
			if streamFrame(c, r.Cursor, "channel.sample", gin.H{"channelId": cid, "eventId": r.EventID, "capturedAt": r.CapturedAt.UTC().Format("2006-01-02T15:04:05.000Z"), "payload": r.PayloadJson, "quality": r.QualityJson}) != nil {
				return
			}
			cursor, _ = strconv.ParseInt(r.Cursor, 10, 64)
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-poll.C:
		case <-auth.C:
			target, err = s.authorizedChannel(c.Request.Context(), uid, pid, cid)
			if err != nil || !s.streamSessionActive(c.Request.Context(), token, uid) {
				_ = streamFrame(c, "", "access.revoked", gin.H{"channelId": cid, "reason": "capability_revoked"})
				return
			}
		case <-heart.C:
			if streamWrite(c, ": heartbeat\n\n") != nil {
				return
			}
		}
	}
}
