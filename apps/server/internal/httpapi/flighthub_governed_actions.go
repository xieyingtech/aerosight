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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed flighthub_device_admin_input.json
var fhDeviceAdminSchema []byte

//go:embed flighthub_flight_input.json
var fhFlightSchema []byte
var fhAdminPolicies = map[string]fhActionPolicy{
	"rtk-calibrate":         {"device.rtk.calibrate", "flighthub.rtk.calibrate", true},
	"relay-pair":            {"device.relay.pair", "flighthub.relay.pair", true},
	"active-project-update": {"device.active-project.update", "flighthub.device-migration", true},
	"sn-decrypt":            {"security.sn.decrypt", "flighthub.sn-decrypt", true},
}

func authorizeFHAdmin(pid int32, cid int64, input map[string]any, row gin.H) error {
	fail := func(code string) error { return errors.New("FLIGHTHUB_DEVICE_ADMIN_" + code) }
	if row["role"] != "owner" && row["role"] != "admin" {
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
	did := fhOptionalNumber(input, "deviceId")
	rtype, rid := "connector", strconv.FormatInt(cid, 10)
	if did > 0 {
		rtype, rid = "device", strconv.FormatInt(did, 10)
		if row["deviceProjectId"] != float64(pid) || row["identityPresent"] != true || row["deviceOnline"] != true || row["stateFresh"] != true {
			return fail("DEVICE_UNAVAILABLE")
		}
	}
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != row["teamId"] || row["approvalResourceType"] != rtype || row["approvalResourceId"] != rid || row["approvalAction"] != "flighthub.admin."+fhString(input["action"]) || row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true {
		return fail("APPROVAL_REQUIRED")
	}
	return nil
}
func authorizeFHFlight(pid int32, cid int64, input map[string]any, row gin.H) error {
	fail := func(code string) error { return errors.New("FLIGHTHUB_ACTION_" + code) }
	if row["hasPermission"] != true {
		return fail("PERMISSION_DENIED")
	}
	if row["connectorProjectId"] != float64(pid) || row["taskRunProjectId"] != float64(pid) || row["connectorTeamId"] != row["teamId"] || row["taskRunTeamId"] != row["teamId"] {
		return fail("SCOPE_MISMATCH")
	}
	if row["connectorStatus"] != "connecting" && row["connectorStatus"] != "connected" && row["connectorStatus"] != "degraded" {
		return fail("CONNECTOR_DISABLED")
	}
	if row["actionEnabled"] != true || row["capabilityFieldVerified"] != true {
		return fail("DISABLED")
	}
	if row["selectedDeviceId"] == nil || row["deviceIdentityPresent"] != true {
		return fail("DEVICE_SCOPE_MISMATCH")
	}
	if row["safetyPolicyVersionId"] == nil || row["preflightAllowed"] != true || row["approvalPreflightAllowed"] != true {
		return fail("PREFLIGHT_FAILED")
	}
	if row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true {
		return fail("APPROVAL_REQUIRED")
	}
	action := fhString(input["action"])
	approvalAction := "flighthub.flight-task.resume"
	if action == "flight-task-create" {
		approvalAction = "flighthub.flight-task.create"
	} else if action == "flight-task-status" {
		approvalAction = "flighthub.flight-task.status"
	}
	rid := strconv.FormatInt(fhOptionalNumber(input, "taskRunId"), 10)
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != row["teamId"] || row["approvalResourceType"] != "task_run" || row["approvalResourceId"] != rid || row["approvalAction"] != approvalAction {
		return fail("APPROVAL_SCOPE_MISMATCH")
	}
	if action == "flight-task-create" {
		if row["taskRunStatus"] != "ready" && row["taskRunStatus"] != "dispatching" {
			return fail("TASK_STATE_INVALID")
		}
		if row["waylineProjectId"] != float64(pid) || row["waylineConnectorId"] != float64(cid) || row["waylineKind"] != "wayline" {
			return fail("WAYLINE_SCOPE_MISMATCH")
		}
	} else {
		if row["targetProjectId"] != float64(pid) || row["targetConnectorId"] != float64(cid) || row["targetKind"] != "flight-task" || row["targetTaskRunId"] != rid {
			return fail("REMOTE_TASK_SCOPE_MISMATCH")
		}
		status := row["taskRunStatus"]
		allowed := status == "paused" || status == "blocked"
		if action == "flight-task-status" {
			allowed = status == "running" || status == "paused" || status == "dispatching"
		}
		if !allowed {
			return fail("TASK_STATE_INVALID")
		}
	}
	return nil
}
func (s *Server) fhGovernedAction(kind string) gin.HandlerFunc {
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
		ctx := c.Request.Context()
		uid := currentUser(c).ID
		if c.Request.Method == "GET" {
			job, err := uuid.Parse(c.Query("jobId"))
			if err != nil {
				fhActionFailure(c, errFHInput)
				return
			}
			s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
				var raw []json.RawMessage
				var err error
				if kind == "flight" {
					raw, err = q.FHFlightRead(ctx, sqlcgen.FHFlightReadParams{P1: job, P2: pid, P3: cid})
				} else {
					if a.Role != "owner" && a.Role != "admin" {
						return nil, sql.ErrNoRows
					}
					raw, err = q.FHDeviceAdminRead(ctx, sqlcgen.FHDeviceAdminReadParams{P1: job, P2: pid, P3: cid})
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
				row := rows[0]
				if kind != "flight" {
					var sensitive any
					if row["action"] == "sn-decrypt" && row["status"] == "succeeded" && row["resultEnvelope"] != nil {
						raw, _ := json.Marshal(row["resultEnvelope"])
						var envelope credentials.Envelope
						if err := json.Unmarshal(raw, &envelope); err != nil {
							return nil, err
						}
						if err := credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("flighthub-device-admin-result", job.String(), pid), &sensitive); err != nil {
							return nil, err
						}
					}
					delete(row, "resultEnvelope")
					row["sensitiveResult"] = sensitive
				}
				return row, nil
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
		schema, permission, resourceType := fhFlightSchema, "mission:operate", "task_run"
		if kind != "flight" {
			schema, permission, resourceType = fhDeviceAdminSchema, "device:configure", "device"
		}
		input, err := parseFHInput(raw, schema)
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		did, rid, target, wayline := fhOptionalNumber(input, "deviceId"), fhOptionalNumber(input, "taskRunId"), fhOptionalNumber(input, "targetResourceId"), fhOptionalNumber(input, "waylineResourceId")
		if did > 2147483647 || rid > 2147483647 {
			fhActionFailure(c, errFHInput)
			return
		}
		approval, err := uuid.Parse(fhString(input["approvalRequestId"]))
		if err != nil {
			fhActionFailure(c, errFHInput)
			return
		}
		a, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		action, key := fhString(input["action"]), fhString(input["idempotencyKey"])
		jobID := uuid.New()
		policy := fhAdminPolicies[action]
		digestInput := gin.H{"action": action, "connectorInstanceId": float64(cid), "deviceId": input["deviceId"], "request": input["request"]}
		resourceID := strconv.FormatInt(did, 10)
		aadKind := "flighthub-device-admin-action"
		if kind == "flight" {
			digestInput = gin.H{"action": action, "taskRunId": input["taskRunId"], "connectorInstanceId": float64(cid), "waylineResourceId": input["waylineResourceId"], "targetResourceId": input["targetResourceId"], "request": input["request"]}
			resourceID = strconv.FormatInt(rid, 10)
			aadKind = "flighthub-flight-action"
		} else if did == 0 {
			resourceType = "connector"
			resourceID = strconv.FormatInt(cid, 10)
		}
		digest, err := database.AuditHash(digestInput)
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		envelope, err := credentials.EncryptJSON(input["request"], s.credentialSecret, credentials.AAD(aadKind, jobID.String(), pid))
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
		policyResult := map[string]any{"permission": "project:admin", "capability": policy.capability, "featureFlag": policy.flag, "evidence": "field-write", "approval": input["approvalRequestId"]}
		if kind == "flight" {
			auditInput = gin.H{}
			for k, v := range input {
				auditInput[k] = v
			}
			auditInput["request"] = gin.H{"digest": digest}
			policyResult = map[string]any{"permission": "mission:operate", "capability": "flight.execute", "approval": input["approvalRequestId"], "completion": "await-remote-reconciliation"}
		}
		audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "connector." + action, ResourceType: resourceType, ResourceID: resourceID, Input: auditInput, PolicyResult: policyResult}
		result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, permission, false), func(w *database.WriteTx) (gin.H, error) {
			q := w.Queries
			var raw []json.RawMessage
			var err error
			if kind == "flight" {
				raw, err = q.FHFlightAuthorize(ctx, sqlcgen.FHFlightAuthorizeParams{P1: pid, P2: int32(rid), P3: a.TeamID, P4: cid, P5: approval, P6: wayline, P7: target, P8: uid})
			} else {
				raw, err = q.FHDeviceAdminAuthorize(ctx, sqlcgen.FHDeviceAdminAuthorizeParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: sql.NullInt32{Int32: int32(did), Valid: did > 0}, P6: policy.capability, P7: policy.flag, P8: approval})
			}
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows(raw)
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				if kind != "flight" {
					return nil, errors.New("FLIGHTHUB_DEVICE_ADMIN_SCOPE_MISMATCH")
				}
				return nil, errors.New("FLIGHTHUB_ACTION_SCOPE_MISMATCH")
			}
			row := rows[0]
			if kind == "flight" {
				if err := authorizeFHFlight(pid, cid, input, row); err != nil {
					return nil, err
				}
				did = int64(row["selectedDeviceId"].(float64))
				request := input["request"].(map[string]any)
				if landing := fhOptionalNumber(request, "landingDeviceId"); landing > 0 {
					if landing > 2147483647 {
						return nil, errFHInput
					}
					found, err := q.FHFlightLanding(ctx, sqlcgen.FHFlightLandingParams{P1: pid, P2: cid, P3: sql.NullInt32{Int32: int32(landing), Valid: true}})
					if err != nil {
						return nil, err
					}
					if len(found) == 0 {
						return nil, errors.New("FLIGHTHUB_ACTION_LANDING_DEVICE_SCOPE_MISMATCH")
					}
				}
			} else {
				if err := authorizeFHAdmin(pid, cid, input, row); err != nil {
					return nil, err
				}
			}
			id, status := "", ""
			reused := false
			if kind == "flight" {
				r, e := q.FHFlightInsert(ctx, sqlcgen.FHFlightInsertParams{P1: jobID, P2: pid, P3: a.TeamID, P4: cid, P5: int32(rid), P6: int32(did), P7: sql.NullInt64{Int64: wayline, Valid: wayline > 0}, P8: sql.NullInt64{Int64: target, Valid: target > 0}, P9: approval, P10: uid, P11: action, P12: key, P13: digest, P14: envelopeJSON})
				id, status, err = r.ID, r.Status, e
			} else {
				r, e := q.FHDeviceAdminInsert(ctx, sqlcgen.FHDeviceAdminInsertParams{P1: jobID, P2: pid, P3: a.TeamID, P4: cid, P5: sql.NullInt32{Int32: int32(did), Valid: did > 0}, P6: uid, P7: approval, P8: action, P9: policy.capability, P10: policy.flag, P11: key, P12: digest, P13: envelopeJSON})
				id, status, err = r.ID, r.Status, e
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if errors.Is(err, sql.ErrNoRows) {
				if kind == "flight" {
					raw, err = q.FHFlightExisting(ctx, sqlcgen.FHFlightExistingParams{P1: pid, P2: cid, P3: action, P4: key})
				} else {
					raw, err = q.FHDeviceAdminExisting(ctx, sqlcgen.FHDeviceAdminExistingParams{P1: pid, P2: cid, P3: action, P4: key})
				}
				if err != nil {
					return nil, err
				}
				rows, err = decodeSnapshotRows(raw)
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
				}
				old := rows[0]
				match := old["requestDigest"] == digest && old["requestedByUserId"] == float64(uid)
				if kind == "flight" {
					match = match && old["taskRunId"] == float64(rid) && old["deviceId"] == float64(did) && old["approvalRequestId"] == input["approvalRequestId"] && reflect.DeepEqual(old["waylineResourceId"], input["waylineResourceId"]) && reflect.DeepEqual(old["targetResourceId"], input["targetResourceId"])
				}
				if !match {
					return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
				}
				id, status, reused = fhString(old["id"]), fhString(old["status"]), true
			}
			payload, _ := json.Marshal(gin.H{"jobId": id})
			if kind == "flight" {
				err = q.FHFlightEnqueue(ctx, sqlcgen.FHFlightEnqueueParams{P1: pid, P2: a.TeamID, P3: "flighthub-flight-action:" + id, P4: sql.NullString{String: id, Valid: true}, P5: payload})
			} else {
				err = q.FHDeviceAdminEnqueue(ctx, sqlcgen.FHDeviceAdminEnqueueParams{P1: pid, P2: a.TeamID, P3: "flighthub-device-admin:" + id, P4: sql.NullString{String: id, Valid: true}, P5: payload})
			}
			result := gin.H{"id": id, "status": status, "reused": reused}
			if kind == "flight" {
				result["completion"] = "await-remote-reconciliation"
			}
			return result, err
		})
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		c.JSON(202, result)
	}
}
