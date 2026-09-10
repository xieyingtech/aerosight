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
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed flighthub_model_input.json
var fhModelSchema []byte

type fhModelPolicy struct{ capability, flag, targetKind string }

var fhModelPolicies = map[string]fhModelPolicy{
	"model-delete":          {"model.delete", "flighthub.model.delete", "model"},
	"model-resource-delete": {"model.resource.delete", "flighthub.model-resource.delete", "model-resource"},
}

func fhFirstRow(raw []json.RawMessage, err error, missing string) (gin.H, error) {
	if err != nil {
		return nil, err
	}
	rows, err := decodeSnapshotRows(raw)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New(missing)
	}
	return rows[0], nil
}
func fhModelPreview(target int64, row gin.H) (gin.H, error) {
	if row["targetProjectId"] == nil || row["targetKind"] == nil || row["targetStatus"] != "active" || row["targetRemoteVersion"] == nil {
		return nil, errors.New("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH")
	}
	return gin.H{"targetResourceId": target, "resourceKind": row["targetKind"], "remoteVersion": row["targetRemoteVersion"], "assetId": row["assetId"], "assetStatus": row["assetStatus"], "dependentReferenceCount": row["dependentReferenceCount"], "effect": "remote-delete-and-local-mark-missing"}, nil
}
func authorizeFHModel(pid int32, cid int64, input map[string]any, row gin.H) error {
	fail := func(code string) error { return errors.New("FLIGHTHUB_MODEL_DELETE_" + code) }
	p := fhModelPolicies[fhString(input["action"])]
	if row["role"] != "owner" && row["role"] != "admin" {
		return fail("PERMISSION_DENIED")
	}
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != row["teamId"] {
		return fail("SCOPE_MISMATCH")
	}
	if row["connectorStatus"] != "connecting" && row["connectorStatus"] != "connected" && row["connectorStatus"] != "degraded" {
		return fail("CONNECTOR_DISABLED")
	}
	if row["actionEnabled"] != true || row["capabilityFieldVerified"] != true {
		return fail("DISABLED")
	}
	if row["targetProjectId"] != float64(pid) || row["targetConnectorId"] != float64(cid) || row["targetKind"] != p.targetKind || row["targetStatus"] != "active" {
		return fail("SCOPE_MISMATCH")
	}
	if row["targetRemoteVersion"] != input["expectedRemoteVersion"] || row["currentPreviewDigest"] != input["previewDigest"] {
		return fail("PREVIEW_CONFLICT")
	}
	if row["approvalProjectId"] != float64(pid) || row["approvalTeamId"] != row["teamId"] || row["approvalResourceType"] != "connector_remote_resource" || row["approvalResourceId"] != strconv.FormatInt(fhOptionalNumber(input, "targetResourceId"), 10) || row["approvalAction"] != p.flag || row["approvalStatus"] != "approved" || row["approvalUnexpired"] != true || row["approvalPreviewDigest"] != input["previewDigest"] || row["approvalRemoteVersion"] != input["expectedRemoteVersion"] {
		return fail("APPROVAL_REQUIRED")
	}
	return nil
}
func (s *Server) fhModelAction(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
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
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	if c.Request.Method == "GET" {
		var result any
		if job := c.Query("jobId"); job != "" {
			id, e := uuid.Parse(job)
			if e != nil {
				fhActionFailure(c, errFHInput)
				return
			}
			raw, e := s.queries.FHModelRead(ctx, sqlcgen.FHModelReadParams{P1: id, P2: pid, P3: cid})
			result, err = fhFirstRow(raw, e, "action_not_found")
		} else {
			target, e := strconv.ParseInt(c.Query("targetResourceId"), 10, 64)
			p, ok := fhModelPolicies[c.Query("action")]
			if e != nil || target <= 0 || !ok {
				fhActionFailure(c, errFHInput)
				return
			}
			raw, e := s.queries.FHModelTarget(ctx, sqlcgen.FHModelTargetParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: target})
			row, e := fhFirstRow(raw, e, "FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH")
			if e != nil {
				fhActionFailure(c, e)
				return
			}
			if row["role"] != "owner" && row["role"] != "admin" || row["targetKind"] != p.targetKind {
				fhActionFailure(c, errors.New("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH"))
				return
			}
			preview, e := fhModelPreview(target, row)
			if e != nil {
				fhActionFailure(c, e)
				return
			}
			digest, e := database.AuditHash(preview)
			err = e
			result = gin.H{"preview": preview, "previewDigest": digest, "approval": gin.H{"resourceType": "connector_remote_resource", "resourceId": strconv.FormatInt(target, 10), "action": p.flag, "context": gin.H{"previewDigest": digest, "expectedRemoteVersion": preview["remoteVersion"]}}}
		}
		if err != nil {
			fhActionFailure(c, err)
			return
		}
		c.JSON(200, result)
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	var body map[string]any
	if err != nil || json.Unmarshal(raw, &body) != nil || body == nil {
		fhActionFailure(c, errFHInput)
		return
	}
	body["connectorInstanceId"] = float64(cid)
	raw, _ = json.Marshal(body)
	input, err := parseFHInput(raw, fhModelSchema)
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	approval, err := uuid.Parse(fhString(input["approvalRequestId"]))
	if err != nil {
		fhActionFailure(c, errFHInput)
		return
	}
	action, key := fhString(input["action"]), fhString(input["idempotencyKey"])
	policy := fhModelPolicies[action]
	target := fhOptionalNumber(input, "targetResourceId")
	job := uuid.New()
	digestInput := gin.H{"action": action, "connectorInstanceId": cid, "targetResourceId": target, "approvalRequestId": input["approvalRequestId"], "expectedRemoteVersion": input["expectedRemoteVersion"], "previewDigest": input["previewDigest"], "request": gin.H{"confirmation": true}}
	digest, err := database.AuditHash(digestInput)
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	envelope, err := credentials.EncryptJSON(input["request"], s.credentialSecret, credentials.AAD("flighthub-model-delete", job.String(), pid))
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	envelopeJSON, _ := json.Marshal(envelope)
	digestInput["request"] = gin.H{"confirmed": true}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "connector." + action, ResourceType: "connector_remote_resource", ResourceID: strconv.FormatInt(target, 10), Input: digestInput, PolicyResult: map[string]any{"permission": "project:admin", "capability": policy.capability, "featureFlag": policy.flag, "evidence": "field-write", "approval": "approved", "completion": "worker-final"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "project:view", false), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		raw, e := q.FHModelTarget(ctx, sqlcgen.FHModelTargetParams{P1: pid, P2: cid, P3: a.TeamID, P4: uid, P5: target})
		row, e := fhFirstRow(raw, e, "FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH")
		if e != nil {
			return nil, e
		}
		preview, e := fhModelPreview(target, row)
		if e != nil {
			return nil, e
		}
		pd, e := database.AuditHash(preview)
		if e != nil {
			return nil, e
		}
		raw, e = q.FHModelGate(ctx, sqlcgen.FHModelGateParams{P1: pid, P2: cid, P3: policy.capability, P4: policy.flag, P5: approval})
		gate, e := fhFirstRow(raw, e, "FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH")
		if e != nil {
			return nil, e
		}
		for k, v := range gate {
			row[k] = v
		}
		row["currentPreviewDigest"] = pd
		if e = authorizeFHModel(pid, cid, input, row); e != nil {
			return nil, e
		}
		inserted, e := q.FHModelInsert(ctx, sqlcgen.FHModelInsertParams{P1: job, P2: pid, P3: a.TeamID, P4: cid, P5: target, P6: approval, P7: uid, P8: action, P9: policy.capability, P10: policy.flag, P11: key, P12: fhString(input["expectedRemoteVersion"]), P13: fhString(input["previewDigest"]), P14: digest, P15: envelopeJSON})
		id, status, reused := inserted.ID, inserted.Status, false
		if errors.Is(e, sql.ErrNoRows) {
			raw, e = q.FHModelExisting(ctx, sqlcgen.FHModelExistingParams{P1: pid, P2: cid, P3: action, P4: key})
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
		e = q.FHModelEnqueue(ctx, sqlcgen.FHModelEnqueueParams{P1: pid, P2: a.TeamID, P3: "flighthub-model-delete:" + id, P4: sql.NullString{String: id, Valid: true}, P5: payload})
		return gin.H{"id": id, "status": status, "reused": reused, "completion": "worker-final"}, e
	})
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	c.JSON(202, result)
}
