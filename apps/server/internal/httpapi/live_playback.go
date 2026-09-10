package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"aerosight/server/internal/media"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func livePlaybackSession(pid int32, row sqlcgen.LockLivePlaybackRow) gin.H {
	var ref, active, reason any
	if row.PlaybackRef.Valid {
		ref = row.PlaybackRef.String
	}
	if row.LastActiveAt.Valid {
		active = row.LastActiveAt.Time.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	if row.StatusReason.Valid {
		reason = row.StatusReason.String
	}
	return gin.H{"id": row.ID, "projectId": pid, "deviceId": row.DeviceID, "streamKey": row.StreamKey, "sourceType": row.SourceType, "status": row.Status, "playbackRef": ref, "lastActiveAt": active, "statusReason": reason}
}
func (s *Server) getLivePlayback(c *gin.Context) {
	fail := func() { s.failure(c, 403, "Unable to access live stream") }
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
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		fail()
		return
	}
	var result gin.H
	err = database.InTx(ctx, s.db, func(tx *sql.Tx, q *sqlcgen.Queries) error {
		if err := s.authorizeWrite(uid, pid, access.TeamID, "project:view", false)(ctx, &database.WriteTx{Tx: tx, Queries: q}); err != nil {
			return err
		}
		membership, err := q.LockProjectMembership(ctx, sqlcgen.LockProjectMembershipParams{ProjectID: pid, UserID: uid})
		if err != nil {
			return err
		}
		row, err := q.LockLivePlayback(ctx, sqlcgen.LockLivePlaybackParams{ProjectID: pid, ID: sid})
		if err != nil {
			return err
		}
		if strings.HasPrefix(row.CapabilityCode, "stream.") {
			raw, err := q.LockDeviceCommandGrants(ctx, sqlcgen.LockDeviceCommandGrantsParams{ProjectID: pid, TeamID: access.TeamID, UserID: uid, DeviceTypeID: sql.NullInt64{Int64: row.DeviceTypeID, Valid: true}, DeviceID: sql.NullInt32{Int32: row.DeviceID, Valid: true}})
			if err != nil {
				return err
			}
			grants := []device.CommandGrant{}
			for _, g := range raw {
				grants = append(grants, device.CommandGrant{Action: g.ActionPattern, Effect: g.Effect})
			}
			if err = device.AuthorizeCommand(membership.Role, row.CapabilityCode, grants); err != nil {
				return err
			}
		}
		result = gin.H{"session": livePlaybackSession(pid, row), "available": false}
		if row.SourceType == "dji_flighthub" {
			if row.Status != "starting" && row.Status != "live" && row.Status != "degraded" {
				result["reason"] = "stream-" + row.Status
				return nil
			}
			if row.Status == "starting" && !row.StartAcceptedAt.Valid {
				result["reason"] = "stream-starting-unaccepted"
				return nil
			}
			if !row.PlaybackRef.Valid || row.PlaybackRef.String == "" {
				result["reason"] = "playback-unavailable"
				return nil
			}
			if row.LocalAuthorizationRevokedAt.Valid {
				result["reason"] = "playback-authorization-revoked"
				return nil
			}
			if !row.SupplierCredentialExpiresAt.Valid || !row.SupplierCredentialExpiresAt.Time.After(time.Now()) {
				result["reason"] = "playback-credential-expired"
				return nil
			}
			authorized, err := q.AuthorizeFlightHubPlayback(ctx, sqlcgen.AuthorizeFlightHubPlaybackParams{ProjectID: pid, ID: sid})
			if errors.Is(err, sql.ErrNoRows) {
				result["reason"] = "playback-credential-unavailable"
				return nil
			}
			if err != nil {
				return err
			}
			var envelope credentials.Envelope
			if err := json.Unmarshal(authorized.SupplierCredentialEnvelopeJson.RawMessage, &envelope); err != nil {
				return err
			}
			var payload struct {
				Credential string `json:"credential"`
			}
			if err := credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("flighthub-live-session", sid, pid), &payload); err != nil {
				return err
			}
			if payload.Credential == "" {
				result["reason"] = "playback-credential-unavailable"
				return nil
			}
			result["available"] = true
			result["playback"] = gin.H{"supplier": authorized.Supplier.String, "protocol": authorized.SupplierProtocol.String,
				"adapterVersion": authorized.SupplierAdapterVersion.String, "credential": payload.Credential,
				"expiresAt": timestamp(authorized.PlaybackLocatorExpiresAt.Time)}
			return nil
		}
		if row.Status != "live" && row.Status != "degraded" {
			result["reason"] = "stream-" + row.Status
			return nil
		}
		if !row.PlaybackRef.Valid || row.PlaybackRef.String == "" {
			result["reason"] = "playback-unavailable"
			return nil
		}
		now := time.Now()
		var expires string
		switch row.SourceType {
		case "simulator":
			locator, err := media.IssueSimulatorLocator(s.credentialSecret, pid, sid, row.PlaybackRef.String, now)
			if err != nil {
				return err
			}
			result["locator"] = locator
			expires = locator.ExpiresAt
		case "dji":
			protocols := []string{}
			if row.WebrtcBaseUrl != "" {
				protocols = append(protocols, "webrtc")
			}
			if row.HlsBaseUrl != "" {
				protocols = append(protocols, "hls")
			}
			if len(protocols) == 0 {
				result["reason"] = "playback-protocol-unavailable"
				return nil
			}
			issued, err := media.IssuePlaybackToken(s.credentialSecret, media.PlaybackClaims{ProjectID: int64(pid), StreamID: sid, Path: row.PlaybackRef.String, Protocols: protocols}, now, 60)
			if err != nil {
				return err
			}
			result["playback"] = gin.H{"candidates": media.PlaybackCandidates(row.PlaybackRef.String, issued.Token, row.HlsBaseUrl, row.WebrtcBaseUrl), "expiresAt": issued.ExpiresAt}
			expires = issued.ExpiresAt
		default:
			result["reason"] = "playback-adapter-unavailable"
			return nil
		}
		expiry, err := time.Parse(time.RFC3339Nano, expires)
		if err != nil {
			return err
		}
		if err = q.SetLivePlaybackExpiry(ctx, sqlcgen.SetLivePlaybackExpiryParams{ProjectID: pid, ID: sid, PlaybackLocatorExpiresAt: sql.NullTime{Time: expiry, Valid: true}}); err != nil {
			return err
		}
		result["available"] = true
		return nil
	})
	if err != nil {
		fail()
		return
	}
	c.JSON(200, result)
}
