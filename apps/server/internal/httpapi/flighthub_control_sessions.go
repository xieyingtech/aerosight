package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var fhPayloadIndex = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

func normalizeFHControl(value any) (gin.H, error) {
	bad := errors.New("FLIGHTHUB_CONTROL_SELECTION_INVALID")
	m, ok := value.(map[string]any)
	if !ok {
		return nil, bad
	}
	for k := range m {
		if k != "flight" && k != "payloadIndex" {
			return nil, bad
		}
	}
	flight, _ := m["flight"].(bool)
	items := []any{}
	if v, present := m["payloadIndex"]; present {
		items, ok = v.([]any)
		if !ok {
			return nil, bad
		}
	}
	if len(items) > 32 {
		return nil, bad
	}
	payload := []string{}
	seen := map[string]bool{}
	for _, v := range items {
		s, ok := v.(string)
		if !ok || !fhPayloadIndex.MatchString(s) || seen[s] {
			return nil, bad
		}
		seen[s] = true
		payload = append(payload, s)
	}
	if !flight && len(payload) == 0 {
		return nil, bad
	}
	sort.Strings(payload)
	return gin.H{"flight": flight, "payloadIndex": payload}, nil
}
func authorizeFHControl(pid, tid, did int32, policy int64, row gin.H, now time.Time) error {
	fail := func(s string) error { return errors.New("FLIGHTHUB_CONTROL_" + s) }
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != float64(tid) || row["deviceProjectId"] != float64(pid) {
		return fail("SCOPE_MISMATCH")
	}
	if row["connectorStatus"] != "connected" {
		return fail("CONNECTOR_UNAVAILABLE")
	}
	if row["featureEnabled"] != true || row["capabilityFieldVerified"] != true {
		return fail("NOT_ENABLED")
	}
	captured, e := time.Parse(time.RFC3339Nano, fhString(row["stateCapturedAt"]))
	if e != nil || row["deviceOnline"] != true || now.Sub(captured) > 30*time.Second || captured.After(now.Add(time.Second)) {
		return fail("DEVICE_STALE")
	}
	if row["currentSafetyPolicyVersionId"] != strconv.FormatInt(policy, 10) {
		return fail("SAFETY_POLICY_STALE")
	}
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != float64(tid) || row["approvalResourceType"] != "device" || row["approvalResourceId"] != strconv.FormatInt(int64(did), 10) || row["approvalAction"] != "flighthub.control.acquire" || row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true {
		return fail("APPROVAL_REQUIRED")
	}
	if row["conflictingSessionCount"] != float64(0) {
		return fail("SESSION_CONFLICT")
	}
	return nil
}
func fhSafePositive(v any) (int64, bool) {
	n, ok := v.(float64)
	return int64(n), ok && n > 0 && n <= 9007199254740991 && math.Trunc(n) == n
}
func fhControlFailure(c *gin.Context, err error) {
	code := err.Error()
	status := 400
	if c.Request.Method == "PATCH" {
		status = 409
		if code != "FLIGHTHUB_CONTROL_HEARTBEAT_REJECTED" && code != "FLIGHTHUB_CONTROL_SESSION_NOT_FOUND" && code != "FLIGHTHUB_CONTROL_OPERATION_RATE_LIMITED" {
			code = "FLIGHTHUB_CONTROL_SESSION_FAILED"
		}
		if strings.Contains(code, "RATE") {
			status = 429
		}
	} else {
		allowed := map[string]bool{"FLIGHTHUB_CONTROL_SELECTION_INVALID": true, "FLIGHTHUB_CONTROL_SCOPE_MISMATCH": true, "FLIGHTHUB_CONTROL_CONNECTOR_UNAVAILABLE": true, "FLIGHTHUB_CONTROL_NOT_ENABLED": true, "FLIGHTHUB_CONTROL_DEVICE_STALE": true, "FLIGHTHUB_CONTROL_SAFETY_POLICY_STALE": true, "FLIGHTHUB_CONTROL_APPROVAL_REQUIRED": true, "FLIGHTHUB_CONTROL_SESSION_CONFLICT": true, "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST": true}
		if !allowed[code] {
			code = "FLIGHTHUB_CONTROL_SESSION_FAILED"
		}
		if strings.Contains(code, "CONFLICT") {
			status = 409
		} else if strings.Contains(code, "APPROVAL") {
			status = 403
		}
	}
	c.JSON(status, gin.H{"error": code})
}
func (s *Server) fhControlSession(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	invalid := func() { c.JSON(400, gin.H{"error": "FLIGHTHUB_CONTROL_SESSION_INPUT_INVALID"}) }
	pid, err := projectID(c)
	if err != nil {
		invalid()
		return
	}
	did64, err := strconv.ParseInt(c.Param("deviceId"), 10, 32)
	if err != nil || did64 <= 0 {
		invalid()
		return
	}
	did := int32(did64)
	var body map[string]any
	if json.NewDecoder(c.Request.Body).Decode(&body) != nil || body == nil {
		invalid()
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	if c.Request.Method == "PATCH" {
		id, e := uuid.Parse(c.Param("sessionId"))
		if e != nil || id.Version() < 1 || id.Version() > 5 || id.Variant() != uuid.RFC4122 || body["action"] != "heartbeat" && body["action"] != "release" {
			invalid()
			return
		}
		a, e := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
		if e != nil {
			fhControlFailure(c, e)
			return
		}
		if body["action"] == "heartbeat" {
			r, e := s.queries.FHControlHeartbeat(ctx, sqlcgen.FHControlHeartbeatParams{P1: pid, P2: id, P3: time.Now().UTC(), P4: 15000, P5: uid, P6: 500, P7: did})
			if errors.Is(e, sql.ErrNoRows) {
				e = errors.New("FLIGHTHUB_CONTROL_HEARTBEAT_REJECTED")
			}
			if e != nil {
				fhControlFailure(c, e)
				return
			}
			c.JSON(200, r)
			return
		}
		audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: id.String() + ":release", Action: "flighthub.control_session.release", ResourceType: "connector_control_session", ResourceID: id.String(), Input: gin.H{"sessionPresent": true}, PolicyResult: map[string]any{"holderRequired": true, "remoteRelease": "single-attempt"}}
		r, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
			q := w.Queries
			raw, e := q.FHControlLockSession(ctx, sqlcgen.FHControlLockSessionParams{P1: pid, P2: id, P3: did})
			row, e := fhFirstRow(raw, e, "FLIGHTHUB_CONTROL_SESSION_NOT_FOUND")
			if e != nil {
				return nil, e
			}
			if row["holderUserId"] != float64(uid) {
				return nil, errors.New("FLIGHTHUB_CONTROL_SESSION_NOT_FOUND")
			}
			status := fhString(row["status"])
			if status == "released" || status == "failed" || status == "expired" {
				return gin.H{"id": id.String(), "status": status, "reused": true}, nil
			}
			if e = q.FHControlRelease(ctx, sqlcgen.FHControlReleaseParams{P1: pid, P2: id}); e != nil {
				return nil, e
			}
			payload, _ := json.Marshal(gin.H{"sessionId": id.String()})
			e = q.FHControlEnqueueRelease(ctx, sqlcgen.FHControlEnqueueReleaseParams{P1: pid, P2: a.TeamID, P3: "flighthub-control-session:" + id.String() + ":release", P4: sql.NullString{String: id.String(), Valid: true}, P5: payload})
			return gin.H{"id": id.String(), "status": "releasing", "reused": status == "releasing"}, e
		})
		if e != nil {
			fhControlFailure(c, e)
			return
		}
		c.JSON(202, r)
		return
	}
	cid, ok := fhSafePositive(body["connectorInstanceId"])
	policy, pok := fhSafePositive(body["safetyPolicyVersionId"])
	approval, e := uuid.Parse(fhString(body["approvalRequestId"]))
	key, kok := body["idempotencyKey"].(string)
	if !ok || !pok || e != nil || approval.Version() < 1 || approval.Version() > 5 || approval.Variant() != uuid.RFC4122 || !kok || utf16Length(key) < 8 || utf16Length(key) > 200 {
		invalid()
		return
	}
	controls, err := normalizeFHControl(body["controls"])
	if err != nil {
		fhControlFailure(c, err)
		return
	}
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		fhControlFailure(c, err)
		return
	}
	controlsJSON, _ := json.Marshal(controls)
	id := uuid.New()
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "flighthub.control_session.acquire", ResourceType: "device", ResourceID: strconv.FormatInt(did64, 10), Input: gin.H{"connectorInstanceId": cid, "controls": controls, "approvalRequestId": approval.String(), "safetyPolicyVersionId": policy}, PolicyResult: map[string]any{"capability": "device.control", "featureFlag": "device.control", "evidence": "field-write", "leaseSeconds": 15, "maximumSeconds": 300}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		locked, e := q.FHControlLockDevice(ctx, sqlcgen.FHControlLockDeviceParams{P1: pid, P2: did})
		if e != nil {
			return nil, e
		}
		if len(locked) == 0 {
			return nil, errors.New("FLIGHTHUB_CONTROL_SCOPE_MISMATCH")
		}
		raw, e := q.FHControlExisting(ctx, sqlcgen.FHControlExistingParams{P1: pid, P2: did, P3: key, P4: controlsJSON})
		if e != nil {
			return nil, e
		}
		if len(raw) > 0 {
			row, e := fhFirstRow(raw, nil, "")
			if e != nil {
				return nil, e
			}
			if row["holderUserId"] != float64(uid) || row["connectorInstanceId"] != strconv.FormatInt(cid, 10) || row["approvalRequestId"] != approval.String() || row["safetyPolicyVersionId"] != strconv.FormatInt(policy, 10) || row["controlsMatch"] != true {
				return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			}
			return gin.H{"id": row["id"], "status": row["status"], "reused": true}, nil
		}
		raw, e = q.FHControlAuthorize(ctx, sqlcgen.FHControlAuthorizeParams{P1: pid, P2: did, P3: cid, P4: approval})
		row, e := fhFirstRow(raw, e, "FLIGHTHUB_CONTROL_SCOPE_MISMATCH")
		if e != nil {
			return nil, e
		}
		now := time.Now().UTC()
		if e = authorizeFHControl(pid, a.TeamID, did, policy, row, now); e != nil {
			return nil, e
		}
		r, e := q.FHControlInsert(ctx, sqlcgen.FHControlInsertParams{P1: id, P2: pid, P3: a.TeamID, P4: cid, P5: did, P6: uid, P7: approval, P8: policy, P9: key, P10: controlsJSON, P11: now, P12: now.Add(15 * time.Second), P13: now.Add(5 * time.Minute)})
		if e != nil {
			return nil, e
		}
		payload, _ := json.Marshal(gin.H{"sessionId": id.String()})
		e = q.FHControlEnqueueAcquire(ctx, sqlcgen.FHControlEnqueueAcquireParams{P1: pid, P2: a.TeamID, P3: "flighthub-control-session:" + id.String() + ":acquire", P4: sql.NullString{String: id.String(), Valid: true}, P5: payload})
		return gin.H{"id": r.ID, "status": r.Status, "reused": false}, e
	})
	if err != nil {
		fhControlFailure(c, err)
		return
	}
	status := 202
	if result["reused"] == true {
		status = 200
	}
	c.JSON(status, result)
}
