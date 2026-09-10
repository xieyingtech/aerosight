package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed flighthub_member_input.json
var fhMemberSchema []byte

//go:embed flighthub_member_preview_input.json
var fhMemberPreviewSchema []byte

const fhMemberCapability = "organization.project-member.write"
const fhMemberFlag = "flighthub.organization.project-member"
const fhMemberApproval = "flighthub.organization.project-member-upsert"

func fhMemberKeys(input map[string]any) ([]string, error) {
	seen := map[string]bool{}
	keys := []string{}
	for _, v := range input["members"].([]any) {
		m := v.(map[string]any)
		id := fhString(m["userId"])
		if seen[id] || strings.ContainsAny(id, "\x00\r\n") || strings.ContainsAny(fhString(m["nickname"]), "\x00\r\n") {
			return nil, errFHInput
		}
		seen[id] = true
		hash := sha256.Sum256([]byte(id))
		keys = append(keys, hex.EncodeToString(hash[:])[:32])
	}
	return keys, nil
}
func fhSliceText(v any, n int) string {
	u := utf16.Encode([]rune(fhString(v)))
	if len(u) > n {
		u = u[:n]
	}
	return string(utf16.Decode(u))
}
func fhMemberPreview(pid int32, cid int64, input map[string]any, keys []string, row gin.H) (gin.H, error) {
	if row["managementGranted"] != true {
		return nil, errors.New("FLIGHTHUB_MANAGEMENT_WRITE_PERMISSION_DENIED")
	}
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != row["teamId"] || row["connectorStatus"] != "connected" || row["targetCount"] != float64(len(keys)) || fhString(row["organizationName"]) == "" {
		return nil, errors.New("FLIGHTHUB_MANAGEMENT_WRITE_TARGET_MISMATCH")
	}
	members := []any{}
	for i, v := range input["members"].([]any) {
		m := v.(map[string]any)
		members = append(members, gin.H{"reference": keys[i][:12], "role": m["role"], "nickname": m["nickname"]})
	}
	return gin.H{"projectId": pid, "connectorInstanceId": cid, "projectName": fhSliceText(row["projectName"], 256), "organizationName": fhSliceText(row["organizationName"], 256), "members": members, "impact": "add-or-update-project-members"}, nil
}
func authorizeFHMember(pid int32, cid int64, input map[string]any, row gin.H, digest string) error {
	fail := func(s string) error { return errors.New("FLIGHTHUB_MANAGEMENT_WRITE_" + s) }
	if row["managementGranted"] != true {
		return fail("PERMISSION_DENIED")
	}
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != row["teamId"] {
		return fail("SCOPE_MISMATCH")
	}
	if row["connectorStatus"] != "connected" {
		return fail("CONNECTOR_OFFLINE")
	}
	if row["featureEnabled"] != true || row["capabilityVerified"] != true {
		return fail("DISABLED")
	}
	if row["targetCount"] != float64(len(input["members"].([]any))) || digest != input["previewDigest"] {
		return fail("TARGET_MISMATCH")
	}
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != row["teamId"] || row["approvalResourceType"] != "connector" || row["approvalResourceId"] != strconv.FormatInt(cid, 10) || row["approvalAction"] != fhMemberApproval || row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true || row["approvalPreviewDigest"] != input["previewDigest"] {
		return fail("APPROVAL_REQUIRED")
	}
	return nil
}
func (s *Server) fhMemberAction(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	fail := func(err error) {
		status := 409
		if c.Request.Method == "GET" {
			status = 404
		}
		code := err.Error()
		if !strings.HasPrefix(code, "FLIGHTHUB_MANAGEMENT_WRITE_") && code != "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST" && code != "device control is forbidden in replay mode" {
			code = "FLIGHTHUB_MANAGEMENT_WRITE_FAILED"
		}
		c.JSON(status, gin.H{"error": code})
	}
	pid, err := projectID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "INPUT_INVALID"})
		return
	}
	cid, err := connectorID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "INPUT_INVALID"})
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		fail(err)
		return
	}
	if c.Request.Method == "GET" {
		job, e := uuid.Parse(c.Query("jobId"))
		if e != nil || job.Version() < 1 || job.Version() > 5 || job.Variant() != uuid.RFC4122 {
			c.JSON(400, gin.H{"error": "INPUT_INVALID"})
			return
		}
		if a.Role != "owner" {
			rows, e := s.queries.FHMemberGrant(ctx, sqlcgen.FHMemberGrantParams{P1: pid, P2: a.TeamID, P3: uid})
			if e != nil {
				fail(e)
				return
			}
			if len(rows) == 0 {
				fail(errors.New("FLIGHTHUB_MANAGEMENT_WRITE_PERMISSION_DENIED"))
				return
			}
		}
		raw, e := s.queries.FHMemberRead(ctx, sqlcgen.FHMemberReadParams{P1: job, P2: pid, P3: cid})
		row, e := fhFirstRow(raw, e, "FLIGHTHUB_MANAGEMENT_WRITE_NOT_FOUND")
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, row)
		return
	}
	if c.Request.Method == "POST" && (c.Query("mode") == "replay" || c.GetHeader("X-AeroSight-Mode") == "replay") {
		fail(errors.New("device control is forbidden in replay mode"))
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	var body map[string]any
	if err != nil || json.Unmarshal(raw, &body) != nil || body == nil {
		fail(errFHInput)
		return
	}
	schema := fhMemberPreviewSchema
	if c.Request.Method == "POST" {
		schema = fhMemberSchema
		body["connectorInstanceId"] = float64(cid)
	}
	raw, _ = json.Marshal(body)
	input, err := parseFHInput(raw, schema)
	if err != nil {
		fail(err)
		return
	}
	keys, err := fhMemberKeys(input)
	if err != nil {
		fail(err)
		return
	}
	params := sqlcgen.FHMemberTargetParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: keys, P6: fhMemberFlag, P7: fhMemberCapability}
	if c.Request.Method == "PUT" {
		raw, e := s.queries.FHMemberTarget(ctx, params)
		row, e := fhFirstRow(raw, e, "FLIGHTHUB_MANAGEMENT_WRITE_SCOPE_MISMATCH")
		if e != nil {
			fail(e)
			return
		}
		preview, e := fhMemberPreview(pid, cid, input, keys, row)
		if e != nil {
			fail(e)
			return
		}
		digest, e := database.AuditHash(preview)
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, gin.H{"preview": preview, "previewDigest": digest, "confirmation": "ADD PROJECT MEMBER", "approval": gin.H{"resourceType": "connector", "resourceId": strconv.FormatInt(cid, 10), "action": fhMemberApproval, "context": gin.H{"previewDigest": digest}}})
		return
	}
	approval, err := uuid.Parse(fhString(input["approvalRequestId"]))
	if err != nil {
		fail(errFHInput)
		return
	}
	params.P8 = uuid.NullUUID{UUID: approval, Valid: true}
	job := uuid.New()
	key := fhString(input["idempotencyKey"])
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "connector.project-member-upsert", ResourceType: "connector", ResourceID: strconv.FormatInt(cid, 10), Input: gin.H{"connectorInstanceId": cid, "previewDigest": input["previewDigest"], "confirmed": true}, PolicyResult: map[string]any{"permission": "organization:manage", "capability": fhMemberCapability, "featureFlag": fhMemberFlag, "evidence": "field-write", "approval": "approved", "completion": "worker-readback"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "project:view", false), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		raw, e := q.FHMemberTarget(ctx, params)
		row, e := fhFirstRow(raw, e, "FLIGHTHUB_MANAGEMENT_WRITE_SCOPE_MISMATCH")
		if e != nil {
			return nil, e
		}
		preview, e := fhMemberPreview(pid, cid, input, keys, row)
		if e != nil {
			return nil, e
		}
		pd, e := database.AuditHash(preview)
		if e != nil {
			return nil, e
		}
		if e = authorizeFHMember(pid, cid, input, row, pd); e != nil {
			return nil, e
		}
		users := []any{}
		for _, v := range input["members"].([]any) {
			m := v.(map[string]any)
			users = append(users, gin.H{"user_id": m["userId"], "role": m["role"], "nickname": m["nickname"]})
		}
		request := gin.H{"add_users": users}
		digest, e := database.AuditHash(request)
		if e != nil {
			return nil, e
		}
		envelope, e := credentials.EncryptJSON(request, s.credentialSecret, credentials.AAD("flighthub-management-write", job.String(), pid))
		if e != nil {
			return nil, e
		}
		envelopeJSON, _ := json.Marshal(envelope)
		previewJSON, _ := json.Marshal(preview)
		inserted, e := q.FHMemberInsert(ctx, sqlcgen.FHMemberInsertParams{P1: job, P2: pid, P3: a.TeamID, P4: cid, P5: uid, P6: approval, P7: fhMemberCapability, P8: fhMemberFlag, P9: key, P10: digest, P11: envelopeJSON, P12: pd, P13: previewJSON})
		id, status, reused := inserted.ID, inserted.Status, false
		if errors.Is(e, sql.ErrNoRows) {
			raw, e = q.FHMemberExisting(ctx, sqlcgen.FHMemberExistingParams{P1: pid, P2: cid, P3: key})
			old, e := fhFirstRow(raw, e, "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			if e != nil {
				return nil, e
			}
			if old["requestDigest"] != digest || old["requestedByUserId"] != float64(uid) {
				return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			}
			id, status, reused = fhString(old["id"]), fhString(old["status"]), true
		} else if e != nil {
			return nil, e
		}
		payload, _ := json.Marshal(gin.H{"jobId": id})
		e = q.FHMemberEnqueue(ctx, sqlcgen.FHMemberEnqueueParams{P1: pid, P2: a.TeamID, P3: "flighthub-management-write:" + id, P4: sql.NullString{String: id, Valid: true}, P5: payload})
		return gin.H{"id": id, "status": status, "reused": reused, "previewDigest": pd, "completion": "worker-readback"}, e
	})
	if err != nil {
		fail(err)
		return
	}
	c.JSON(202, result)
}
