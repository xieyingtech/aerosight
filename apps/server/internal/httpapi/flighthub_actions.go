package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed flighthub_live_input.json
var fhLiveSchema []byte

//go:embed flighthub_geospatial_input.json
var fhGeoSchema []byte

type fhActionPolicy struct {
	capability, flag string
	ownerOnly        bool
}

var fhActionPolicies = map[string]fhActionPolicy{
	"live-quality-set":      {"live.quality.set", "flighthub.live.quality", false},
	"live-converter-create": {"live.converter.create", "flighthub.live.converter.create", false},
	"live-converter-toggle": {"live.converter.toggle", "flighthub.live.converter.toggle", false},
	"live-converter-delete": {"live.converter.delete", "flighthub.live.converter.delete", true},
	"map-element-create":    {"geospatial.write", "flighthub.actions", false},
	"map-element-update":    {"geospatial.write", "flighthub.actions", false},
	"map-element-delete":    {"geospatial.element.delete", "flighthub.geospatial.delete", true},
}

func fhActionFailure(c *gin.Context, err error) {
	code := err.Error()
	status := 403
	if code == "invalid_request" {
		status = 400
	} else if code == "action_not_found" {
		status = 404
	} else if strings.Contains(code, "PREVIEW_CONFLICT") {
		code = "preview_conflict"
		status = 409
	} else if strings.Contains(code, "VERSION_CONFLICT") {
		code = "version_conflict"
		status = 409
	} else if code == "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST" {
		code = "idempotency_conflict"
		status = 409
	} else if strings.HasPrefix(code, "FLIGHTHUB_") {
		code = strings.ToLower(code)
	} else {
		code = "access_denied"
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}
func fhOptionalNumber(input map[string]any, key string) int64 {
	n, _ := input[key].(float64)
	return int64(n)
}
func fhAuthorizeResourceAction(kind string, pid int32, cid int64, input map[string]any, row gin.H, policy fhActionPolicy) error {
	prefix := "FLIGHTHUB_" + strings.ToUpper(kind) + "_ACTION_"
	if row["hasOperatePermission"] != true || policy.ownerOnly && row["role"] != "owner" && row["role"] != "admin" {
		return errors.New(prefix + "PERMISSION_DENIED")
	}
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != row["teamId"] {
		return errors.New(prefix + "SCOPE_MISMATCH")
	}
	if row["connectorStatus"] != "connecting" && row["connectorStatus"] != "connected" && row["connectorStatus"] != "degraded" {
		return errors.New(prefix + "CONNECTOR_DISABLED")
	}
	if row["actionEnabled"] != true || row["capabilityFieldVerified"] != true {
		return errors.New(prefix + "DISABLED")
	}
	if kind == "live" && input["deviceId"] != nil {
		if row["deviceProjectId"] != float64(pid) || row["deviceConnectorIdentityPresent"] != true {
			return errors.New(prefix + "DEVICE_SCOPE_MISMATCH")
		}
		return nil
	}
	if kind == "geospatial" && input["action"] == "map-element-create" {
		return nil
	}
	targetKind := "stream-converter"
	if kind == "geospatial" {
		targetKind = "map-element"
	}
	if row["targetProjectId"] != float64(pid) || row["targetConnectorId"] != float64(cid) || row["targetKind"] != targetKind || row["targetStatus"] != "active" {
		return errors.New(prefix + "TARGET_SCOPE_MISMATCH")
	}
	if kind == "geospatial" && row["targetRemoteVersion"] != input["expectedRemoteVersion"] {
		return errors.New(prefix + "VERSION_CONFLICT")
	}
	return nil
}
func (s *Server) fhResourceAction(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid, err := projectID(c)
		if err != nil {
			fhActionFailure(c, errFHInput)
			return
		}
		cid, err := connectorID(c)
		if err != nil {
			fhActionFailure(c, errFHInput)
			return
		}
		if c.Request.Method == "GET" {
			job, err := uuid.Parse(c.Query("jobId"))
			if err != nil {
				fhActionFailure(c, errFHInput)
				return
			}
			s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
				var raw []json.RawMessage
				var err error
				if kind == "live" {
					raw, err = q.FHLiveRead(c.Request.Context(), sqlcgen.FHLiveReadParams{P1: job, P2: pid, P3: cid})
				} else {
					raw, err = q.FHGeospatialRead(c.Request.Context(), sqlcgen.FHGeospatialReadParams{P1: job, P2: pid, P3: cid})
				}
				if err != nil {
					return nil, err
				}
				rows, err := decodeSnapshotRows(raw)
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					return nil, sql.ErrNoRows
				}
				return rows[0], nil
			})
			return
		}
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			fhActionFailure(c, errFHInput)
			return
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil || body == nil {
			fhActionFailure(c, errFHInput)
			return
		}
		body["connectorInstanceId"] = float64(cid)
		raw, _ = json.Marshal(body)
		schema := fhLiveSchema
		if kind == "geospatial" {
			schema = fhGeoSchema
		}
		input, err := parseFHInput(raw, schema)
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		uid := currentUser(c).ID
		ctx := c.Request.Context()
		a, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		action := fhString(input["action"])
		policy := fhActionPolicies[action]
		key := fhString(input["idempotencyKey"])
		did := fhOptionalNumber(input, "deviceId")
		if did > 2147483647 {
			fhActionFailure(c, errFHInput)
			return
		}
		target := fhOptionalNumber(input, "targetResourceId")
		expected := fhString(input["expectedRemoteVersion"])
		digestInput := gin.H{"action": action, "connectorInstanceId": float64(cid), "targetResourceId": input["targetResourceId"], "request": input["request"]}
		if kind == "live" {
			digestInput["deviceId"] = input["deviceId"]
		} else {
			digestInput["expectedRemoteVersion"] = input["expectedRemoteVersion"]
		}
		digest, err := database.AuditHash(digestInput)
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		jobID := uuid.New()
		envelope, err := credentials.EncryptJSON(input["request"], s.credentialSecret, credentials.AAD("flighthub-"+kind+"-action", jobID.String(), pid))
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		envelopeJSON, _ := json.Marshal(envelope)
		auditInput := gin.H{}
		for k, v := range digestInput {
			auditInput[k] = v
		}
		auditInput["request"] = gin.H{"digest": digest}
		resourceType, resourceID := "connector_remote_resource", ""
		if target > 0 {
			resourceID = strconv.FormatInt(target, 10)
		}
		if did > 0 {
			resourceType = "device"
			resourceID = strconv.FormatInt(did, 10)
		}
		permission := "mission:operate"
		if policy.ownerOnly {
			permission = "project:admin"
		}
		audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "connector." + action, ResourceType: resourceType, ResourceID: resourceID, Input: auditInput, PolicyResult: map[string]any{"permission": permission, "capability": policy.capability, "featureFlag": policy.flag, "evidence": "field-write", "completion": "worker-final"}}
		if kind == "geospatial" {
			audit.PolicyResult["concurrency"] = "remote-version"
		}
		result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
			q := w.Queries
			var rowsJSON []json.RawMessage
			var err error
			if kind == "live" {
				rowsJSON, err = q.FHLiveAuthorize(ctx, sqlcgen.FHLiveAuthorizeParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: int32(did), P6: policy.capability, P7: policy.flag, P8: target, P9: action})
			} else {
				rowsJSON, err = q.FHGeospatialAuthorize(ctx, sqlcgen.FHGeospatialAuthorizeParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: target, P6: policy.capability, P7: policy.flag})
			}
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows(rowsJSON)
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return nil, errors.New("FLIGHTHUB_" + strings.ToUpper(kind) + "_ACTION_SCOPE_MISMATCH")
			}
			if err := fhAuthorizeResourceAction(kind, pid, cid, input, rows[0], policy); err != nil {
				return nil, err
			}
			id, status := "", ""
			reused := false
			if kind == "live" {
				r, e := q.FHLiveInsert(ctx, sqlcgen.FHLiveInsertParams{P1: jobID, P2: pid, P3: a.TeamID, P4: cid, P5: sql.NullInt32{Int32: int32(did), Valid: did > 0}, P6: sql.NullInt64{Int64: target, Valid: target > 0}, P7: uid, P8: action, P9: policy.capability, P10: policy.flag, P11: key, P12: digest, P13: envelopeJSON})
				id, status, err = r.ID, r.Status, e
			} else {
				r, e := q.FHGeospatialInsert(ctx, sqlcgen.FHGeospatialInsertParams{P1: jobID, P2: pid, P3: a.TeamID, P4: cid, P5: sql.NullInt64{Int64: target, Valid: target > 0}, P6: uid, P7: action, P8: policy.capability, P9: policy.flag, P10: key, P11: sql.NullString{String: expected, Valid: expected != ""}, P12: digest, P13: envelopeJSON})
				id, status, err = r.ID, r.Status, e
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if errors.Is(err, sql.ErrNoRows) {
				if kind == "live" {
					rowsJSON, err = q.FHLiveExisting(ctx, sqlcgen.FHLiveExistingParams{P1: pid, P2: cid, P3: action, P4: key})
				} else {
					rowsJSON, err = q.FHGeospatialExisting(ctx, sqlcgen.FHGeospatialExistingParams{P1: pid, P2: cid, P3: action, P4: key})
				}
				if err != nil {
					return nil, err
				}
				rows, err = decodeSnapshotRows(rowsJSON)
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					if kind == "geospatial" {
						return nil, errors.New("FLIGHTHUB_GEOSPATIAL_ACTION_VERSION_CONFLICT")
					}
					return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
				}
				old := rows[0]
				match := old["requestDigest"] == digest && old["requestedByUserId"] == float64(uid) && reflect.DeepEqual(old["targetResourceId"], input["targetResourceId"])
				if kind == "live" {
					match = match && reflect.DeepEqual(old["deviceId"], input["deviceId"])
				} else {
					match = match && reflect.DeepEqual(old["expectedRemoteVersion"], input["expectedRemoteVersion"])
				}
				if !match {
					return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
				}
				id, status, reused = fhString(old["id"]), fhString(old["status"]), true
			}
			payload, _ := json.Marshal(gin.H{"jobId": id})
			eventID := "flighthub-" + kind + "-action:" + id
			if kind == "live" {
				err = q.FHLiveEnqueue(ctx, sqlcgen.FHLiveEnqueueParams{P1: pid, P2: a.TeamID, P3: eventID, P4: sql.NullString{String: id, Valid: true}, P5: payload})
			} else {
				err = q.FHGeospatialEnqueue(ctx, sqlcgen.FHGeospatialEnqueueParams{P1: pid, P2: a.TeamID, P3: eventID, P4: sql.NullString{String: id, Valid: true}, P5: payload})
			}
			return gin.H{"id": id, "status": status, "reused": reused, "completion": "worker-final"}, err
		})
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		c.JSON(202, result)
	}
}
