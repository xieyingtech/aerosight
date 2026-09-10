package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func publicLiveControlSession(pid int32, row sqlcgen.LockLiveControlSessionRow) gin.H {
	return livePlaybackSession(pid, sqlcgen.LockLivePlaybackRow{ID: row.ID, DeviceID: row.DeviceID, StreamKey: row.StreamKey, SourceType: row.SourceType, Status: row.Status, PlaybackRef: row.PlaybackRef, LastActiveAt: row.LastActiveAt, StatusReason: row.StatusReason})
}
func (s *Server) stopLiveStream(c *gin.Context) {
	if c.GetHeader("X-AeroSight-Mode") == "replay" || c.Query("mode") == "replay" {
		s.failure(c, 409, "REPLAY_CONTROL_FORBIDDEN")
		return
	}
	fail := func() { s.failure(c, 400, "Unable to stop live stream") }
	pid, err := projectID(c)
	if err != nil {
		fail()
		return
	}
	sid, err := strconv.ParseInt(c.Param("streamId"), 10, 64)
	if err != nil || sid <= 0 {
		fail()
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		fail()
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "live_stream.stop", ResourceType: "live_stream", ResourceID: strconv.FormatInt(sid, 10), Input: gin.H{}, PolicyResult: map[string]any{"permission": "mission:operate"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		// Match start's device -> session lock order before command FK checks.
		if _, e := w.Queries.LockLiveControlDevice(ctx, sqlcgen.LockLiveControlDeviceParams{ProjectID: pid, ID: sid}); e != nil {
			return nil, e
		}
		row, e := w.Queries.LockLiveControlSession(ctx, sqlcgen.LockLiveControlSessionParams{ProjectID: pid, ID: sid})
		if e != nil {
			return nil, e
		}
		if row.Status == "stopped" || row.Status == "stopping" || row.SourceType == "dji_flighthub" && row.Status == "failed" {
			return gin.H{"session": publicLiveControlSession(pid, row), "replayed": true}, nil
		}
		next := "stopped"
		if row.SourceType == "dji" && (row.Status == "requested" || row.Status == "starting" || row.Status == "live" || row.Status == "degraded") {
			next = "stopping"
		}
		if row.SourceType == "dji_flighthub" {
			if e = w.Queries.StopFlightHubLiveSession(ctx, sqlcgen.StopFlightHubLiveSessionParams{ProjectID: pid, ID: sid}); e != nil {
				return nil, e
			}
			row, e = w.Queries.LockLiveControlSession(ctx, sqlcgen.LockLiveControlSessionParams{ProjectID: pid, ID: sid})
			if e != nil {
				return nil, e
			}
			next = row.Status
		} else {
			if e = w.Queries.StopLiveControlSession(ctx, sqlcgen.StopLiveControlSessionParams{ProjectID: pid, ID: sid, Status: next, LeaseOwner: "go:" + uuid.NewString()}); e != nil {
				return nil, e
			}
			row.Status = next
			if next == "stopped" {
				row.PlaybackRef = sql.NullString{}
			}
			if next == "stopping" {
				if !row.VendorStreamRef.Valid || row.VendorStreamRef.String == "" {
					return nil, errors.New("DJI_LIVE_VIDEO_ID_MISSING")
				}
				parameters, e := json.Marshal(gin.H{"video_id": row.VendorStreamRef.String})
				if e != nil {
					return nil, e
				}
				safety, e := json.Marshal(gin.H{"liveStreamId": sid})
				if e != nil {
					return nil, e
				}
				command, e := w.Queries.InsertLiveControlCommand(ctx, sqlcgen.InsertLiveControlCommandParams{ID: uuid.New(), ProjectID: pid, TeamID: access.TeamID, DeviceID: row.DeviceID, StreamID: sql.NullInt64{Int64: sid, Valid: true}, CommandKey: "stop", IdempotencyKey: fmt.Sprintf("live-stream:%d:stop", sid), Parameters: parameters, Safety: safety, Priority: 30, ActorUserID: sql.NullInt32{Int32: uid, Valid: true}})
				if e != nil {
					return nil, e
				}
				// On conflict publish the actual command id, never a new uninserted UUID.
				if _, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: "device.command.dispatch:" + command.String(), EventType: "device.command.dispatch", Payload: gin.H{"commandId": command.String()}}); e != nil {
					return nil, e
				}
			}

		}
		event := "live_stream.stopped"
		if next == "stopping" {
			event = "live_stream.stop_requested"
		}
		if _, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: uuid.NewString(), EventType: event, Payload: gin.H{"streamId": sid, "deviceId": row.DeviceID, "status": next}, NoEnqueue: true}); e != nil {
			return nil, e
		}
		return gin.H{"session": publicLiveControlSession(pid, row), "replayed": false}, nil
	})
	if err != nil {
		fail()
		return
	}
	c.JSON(200, result)
}
