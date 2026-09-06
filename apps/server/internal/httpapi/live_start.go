package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) liveStartFailure(c *gin.Context, err error) {
	code := "LIVE_STREAM_START_FAILED"
	switch err.Error() {
	case "INVALID_STREAM_KEY", "DEVICE_NOT_FOUND", "LIVE_STREAM_DEVICE_OFFLINE", "LIVE_STREAM_NOT_SUPPORTED", "LIVE_STREAM_ADAPTER_UNAVAILABLE", "LIVE_STREAM_CHANNEL_NOT_FOUND", "LIVE_STREAM_CONCURRENCY_CONFLICT", "DJI_LIVE_TOPOLOGY_NOT_FOUND", "DJI_LIVE_VIDEO_IDENTITY_INVALID", "LIVE_STREAM_MEDIA_INGEST_PROFILE_REQUIRED", "LIVE_STREAM_ADAPTER_REQUIRED", "LIVE_STREAM_PUBLISH_CREDENTIALS_REQUIRED", "LIVE_STREAM_RTMP_URL_REQUIRED", "LIVE_STREAM_PUBLISH_CREDENTIALS_INVALID", "PROJECT_ACCESS_DENIED", "DEVICE_CAPABILITY_EXPLICITLY_DENIED", "DEVICE_CAPABILITY_NOT_GRANTED", "device control is forbidden in replay mode":
		code = err.Error()
	}
	s.failure(c, 409, code)
}
func (s *Server) startLiveStream(c *gin.Context) {
	fail := func(err error) { s.liveStartFailure(c, err) }
	if c.GetHeader("X-AeroSight-Mode") == "replay" || c.Query("mode") == "replay" {
		fail(errors.New("device control is forbidden in replay mode"))
		return
	}
	pid, err := projectID(c)
	if err != nil {
		fail(err)
		return
	}
	id, err := strconv.ParseInt(c.Param("deviceId"), 10, 32)
	if err != nil || id <= 0 {
		fail(errors.New("DEVICE_NOT_FOUND"))
		return
	}
	did := int32(id)
	var body map[string]any
	if err := strictJSON(c, &body); err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			fail(err)
			return
		}
		body = map[string]any{}
	}
	requested := ""
	if v, present := body["streamKey"]; present && v != nil {
		var ok bool
		requested, ok = v.(string)
		if !ok {
			fail(errors.New("INVALID_STREAM_KEY"))
			return
		}
		requested = strings.TrimSpace(requested)
	}
	if requested != "" && !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`).MatchString(requested) {
		fail(errors.New("INVALID_STREAM_KEY"))
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		fail(errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	var auditKey any
	if requested != "" {
		auditKey = requested
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "live_stream.start", ResourceType: "live_stream", Input: gin.H{"deviceId": did, "streamKey": auditKey}, PolicyResult: map[string]any{"permission": "mission:operate"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		target, e := w.Queries.LockLiveStartDevice(ctx, sqlcgen.LockLiveStartDeviceParams{ProjectID: pid, ID: did})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("DEVICE_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		if target.Status != "online" {
			return nil, errors.New("LIVE_STREAM_DEVICE_OFFLINE")
		}
		control := slices.Contains(target.Capabilities, "stream.video.control")
		if !control && !slices.Contains(target.Capabilities, "camera.live") {
			return nil, errors.New("LIVE_STREAM_NOT_SUPPORTED")
		}
		kind := target.AdapterType.String
		if kind != "simulator" && kind != "dji" {
			return nil, errors.New("LIVE_STREAM_ADAPTER_UNAVAILABLE")
		}
		if control {
			membership, e := w.Queries.LockProjectMembership(ctx, sqlcgen.LockProjectMembershipParams{ProjectID: pid, UserID: uid})
			if e != nil {
				return nil, e
			}
			raw, e := w.Queries.LockDeviceCommandGrants(ctx, sqlcgen.LockDeviceCommandGrantsParams{ProjectID: pid, TeamID: access.TeamID, UserID: uid, DeviceTypeID: sql.NullInt64{Int64: target.DeviceTypeID, Valid: true}, DeviceID: sql.NullInt32{Int32: did, Valid: true}})
			if e != nil {
				return nil, e
			}
			grants := []device.CommandGrant{}
			for _, g := range raw {
				grants = append(grants, device.CommandGrant{Action: g.ActionPattern, Effect: g.Effect})
			}
			if e = device.AuthorizeCommand(membership.Role, "stream.video.control", grants); e != nil {
				return nil, e
			}
		}
		channel, e := w.Queries.ReadLiveStartChannel(ctx, sqlcgen.ReadLiveStartChannelParams{ProjectID: pid, DeviceID: did, StreamKey: sql.NullString{String: requested, Valid: requested != ""}})
		var channelID sql.NullInt64
		key := requested
		if errors.Is(e, sql.ErrNoRows) {
			if kind != "simulator" {
				return nil, errors.New("LIVE_STREAM_CHANNEL_NOT_FOUND")
			}
		} else if e != nil {
			return nil, e
		} else {
			channelID = sql.NullInt64{Int64: channel.ID, Valid: true}
			key = channel.ChannelKey
		}
		if key == "" {
			key = "camera.main"
		}
		existing, e := w.Queries.FindLiveStartReplay(ctx, sqlcgen.FindLiveStartReplayParams{ProjectID: pid, DeviceID: did, StreamKey: key})
		if e == nil {
			row, e := w.Queries.LockLiveControlSession(ctx, sqlcgen.LockLiveControlSessionParams{ProjectID: pid, ID: existing})
			return gin.H{"session": publicLiveControlSession(pid, row), "replayed": true}, e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		count, e := w.Queries.CountLiveStartSessions(ctx, sqlcgen.CountLiveStartSessionsParams{ProjectID: pid, DeviceID: did})
		if e != nil {
			return nil, e
		}
		if count >= target.MaxConcurrentSessions {
			return nil, errors.New("LIVE_STREAM_CONCURRENCY_CONFLICT")
		}
		random := make([]byte, 24)
		if _, e = rand.Read(random); e != nil {
			return nil, e
		}
		ingest := "demo/aerosight/" + base64.RawURLEncoding.EncodeToString(random)
		var vendor, playback sql.NullString
		var endpoint string
		status := "live"
		if kind == "dji" {
			topology, e := w.Queries.ReadDJILiveTopology(ctx, sqlcgen.ReadDJILiveTopologyParams{ProjectID: pid, DeviceID: sql.NullInt32{Int32: did, Valid: true}})
			if errors.Is(e, sql.ErrNoRows) {
				return nil, errors.New("DJI_LIVE_TOPOLOGY_NOT_FOUND")
			}
			if e != nil {
				return nil, e
			}
			if strings.TrimSpace(topology.ExternalDeviceID) == "" || topology.CameraType <= 0 || topology.CameraSubtype < 0 {
				return nil, errors.New("DJI_LIVE_VIDEO_IDENTITY_INVALID")
			}
			vendor = sql.NullString{String: fmt.Sprintf("%s/%d-%d-0/normal-0", topology.ExternalDeviceID, topology.CameraType, topology.CameraSubtype), Valid: true}
			if !target.MediaIngestBaseUrl.Valid || target.MediaIngestBaseUrl.String == "" {
				return nil, errors.New("LIVE_STREAM_MEDIA_INGEST_PROFILE_REQUIRED")
			}
			if !target.AdapterID.Valid {
				return nil, errors.New("LIVE_STREAM_ADAPTER_REQUIRED")
			}
			var envelope credentials.Envelope
			var credential struct {
				User     string `json:"mediaPublishUser"`
				Password string `json:"mediaPublishPassword"`
			}
			if !target.CredentialEnvelopeJson.Valid || json.Unmarshal(target.CredentialEnvelopeJson.RawMessage, &envelope) != nil || credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("device-adapter", target.AdapterID.Int64, pid), &credential) != nil || credential.User == "" || credential.Password == "" {
				return nil, errors.New("LIVE_STREAM_PUBLISH_CREDENTIALS_REQUIRED")
			}
			parsed, e := url.Parse(target.MediaIngestBaseUrl.String)
			if e != nil || parsed.Hostname() == "" || (parsed.Scheme != "rtmp" && parsed.Scheme != "rtmps") {
				return nil, errors.New("LIVE_STREAM_RTMP_URL_REQUIRED")
			}
			if parsed.User != nil {
				return nil, errors.New("LIVE_STREAM_PUBLISH_CREDENTIALS_INVALID")
			}
			parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/" + ingest
			endpoint = parsed.String()
			status = "requested"
		} else {
			playback = sql.NullString{String: fmt.Sprintf("simulator://devices/%d/%s", did, key), Valid: true}
		}
		sid, e := w.Queries.InsertLiveStartSession(ctx, sqlcgen.InsertLiveStartSessionParams{ProjectID: pid, TeamID: access.TeamID, DeviceID: did, AdapterID: target.AdapterID, StreamKey: key, ChannelID: channelID, SourceType: kind, Status: status, IngestRef: sql.NullString{String: ingest, Valid: true}, VendorRef: vendor, PlaybackRef: playback, ActorUserID: sql.NullInt32{Int32: uid, Valid: true}, LeaseOwner: sql.NullString{String: "go:" + uuid.NewString(), Valid: true}})
		if e != nil {
			return nil, e
		}
		if kind == "dji" {
			parameters, e := json.Marshal(gin.H{"url_type": 1, "url": endpoint, "video_id": vendor.String, "video_quality": 3})
			if e != nil {
				return nil, e
			}
			safety, e := json.Marshal(gin.H{"liveStreamId": sid, "serverDerivedDestination": true})
			if e != nil {
				return nil, e
			}
			command, e := w.Queries.InsertLiveControlCommand(ctx, sqlcgen.InsertLiveControlCommandParams{ID: uuid.New(), ProjectID: pid, TeamID: access.TeamID, DeviceID: did, StreamID: sql.NullInt64{Int64: sid, Valid: true}, CommandKey: "start", IdempotencyKey: fmt.Sprintf("live-stream:%d:start", sid), Parameters: parameters, Safety: safety, Priority: 20, ActorUserID: sql.NullInt32{Int32: uid, Valid: true}})
			if e != nil {
				return nil, e
			}
			if _, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: "device.command.dispatch:" + command.String(), EventType: "device.command.dispatch", Payload: gin.H{"commandId": command.String()}}); e != nil {
				return nil, e
			}
		}
		event := "live_stream.started"
		if status == "requested" {
			event = "live_stream.requested"
		}
		if _, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: uuid.NewString(), EventType: event, Payload: gin.H{"streamId": sid, "deviceId": did, "streamKey": key, "status": status}, NoEnqueue: true}); e != nil {
			return nil, e
		}
		row, e := w.Queries.LockLiveControlSession(ctx, sqlcgen.LockLiveControlSessionParams{ProjectID: pid, ID: sid})
		return gin.H{"session": publicLiveControlSession(pid, row), "replayed": false}, e
	})
	if err != nil {
		fail(err)
		return
	}
	c.JSON(200, result)
}
