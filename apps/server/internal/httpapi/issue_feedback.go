package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"io"
	"strconv"
	"strings"
)

//go:embed issue_feedback_input.json
var issueFeedbackSchema []byte

func (s *Server) issueFeedback(c *gin.Context) {
	fail := func(e error) {
		code := e.Error()
		status := 400
		if code == "ISSUE_VERSION_CONFLICT" {
			status = 409
		} else if strings.Contains(code, "ACCESS") || strings.Contains(code, "PERMISSION") {
			status = 403
		} else if strings.Contains(code, "NOT_FOUND") {
			status = 404
		}
		if !strings.HasPrefix(code, "ISSUE_") && code != "PROJECT_ACCESS_DENIED" {
			code = "ISSUE_FEEDBACK_FAILED"
		}
		s.failure(c, status, code)
	}
	pid, e := projectID(c)
	if e != nil {
		fail(errFHInput)
		return
	}
	iid, e := strconv.ParseInt(c.Param("issueId"), 10, 32)
	if e != nil || iid <= 0 {
		fail(errFHInput)
		return
	}
	raw, e := io.ReadAll(c.Request.Body)
	if e != nil {
		fail(errFHInput)
		return
	}
	input, e := parseFHInput(raw, issueFeedbackSchema)
	if e != nil {
		fail(errFHInput)
		return
	}
	action := fhString(input["action"])
	if action == "category_correction" && input["correctedLabel"] == nil || action != "category_correction" && input["correctedLabel"] != nil || action == "disposition" && input["disposition"] == nil {
		fail(errors.New("ISSUE_FEEDBACK_INPUT_INVALID"))
		return
	}
	key, e := uuid.Parse(fhString(input["clientKey"]))
	if e != nil {
		fail(errFHInput)
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, e := s.projectAccess(ctx, s.queries, uid, pid, "issue:handle")
	if e != nil {
		fail(e)
		return
	}
	did := fhOptionalNumber(input, "detectionId")
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key.String(), Action: "issue.feedback.record", ResourceType: "issue", ResourceID: strconv.FormatInt(iid, 10), Input: gin.H{"action": action, "detectionId": did, "expectedVersion": input["expectedVersion"]}, PolicyResult: map[string]any{"permission": "issue:handle"}}
	result, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "issue:handle", false), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		raw, e := q.FeedbackReplay(ctx, sqlcgen.FeedbackReplayParams{P1: pid, P2: int32(iid), P3: key})
		if e != nil {
			return nil, e
		}
		if len(raw) > 0 {
			r, e := fhFirstRow(raw, nil, "")
			if e != nil {
				return nil, e
			}
			r["replayed"] = true
			return r, nil
		}
		raw, e = q.FeedbackEvidence(ctx, sqlcgen.FeedbackEvidenceParams{P1: pid, P2: int32(iid), P3: did})
		row, e := fhFirstRow(raw, e, "ISSUE_FEEDBACK_EVIDENCE_NOT_FOUND")
		if e != nil {
			return nil, e
		}
		if row["issueProjectId"] != float64(pid) || row["detectionId"] != float64(did) {
			return nil, errors.New("ISSUE_FEEDBACK_SCOPE_MISMATCH")
		}
		if row["issueVersion"] != input["expectedVersion"] {
			return nil, errors.New("ISSUE_VERSION_CONFLICT")
		}
		evidence := gin.H{}
		for _, k := range []string{"detectionId", "originalLabel", "algorithmDefinitionVersionId", "taskVersionId", "taskRunStepId"} {
			evidence[k] = row[k]
		}
		evJSON, _ := json.Marshal(evidence)
		feedback, e := q.FeedbackInsert(ctx, sqlcgen.FeedbackInsertParams{P1: pid, P2: a.TeamID, P3: int32(iid), P4: did, P5: fhOptionalNumber(row, "algorithmDefinitionVersionId"), P6: sql.NullInt64{Int64: fhOptionalNumber(row, "taskVersionId"), Valid: row["taskVersionId"] != nil}, P7: sql.NullInt64{Int64: fhOptionalNumber(row, "taskRunStepId"), Valid: row["taskRunStepId"] != nil}, P8: action, P9: sql.NullString{String: fhString(input["correctedLabel"]), Valid: input["correctedLabel"] != nil}, P10: sql.NullString{String: fhString(input["disposition"]), Valid: input["disposition"] != nil}, P11: fhString(input["reason"]), P12: key, P13: evJSON, P14: uid})
		if e != nil {
			return nil, e
		}
		evidence["feedbackId"], evidence["correctedLabel"], evidence["disposition"] = feedback.ID, input["correctedLabel"], input["disposition"]
		evJSON, _ = json.Marshal(evidence)
		if e = q.FeedbackEvent(ctx, sqlcgen.FeedbackEventParams{P1: pid, P2: int32(iid), P3: "feedback." + action, P4: sql.NullString{String: fhString(input["reason"]), Valid: true}, P5: evJSON, P6: sql.NullInt32{Int32: uid, Valid: true}, P7: sql.NullString{String: "feedback:" + key.String(), Valid: true}}); e != nil {
			return nil, e
		}
		if e = q.FeedbackUpdate(ctx, sqlcgen.FeedbackUpdateParams{P1: pid, P2: int32(iid)}); e != nil {
			return nil, e
		}
		version := fhOptionalNumber(input, "expectedVersion") + 1
		_, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: a.TeamID, EventID: uuid.NewString(), EventType: "issue.updated", Payload: gin.H{"issueId": iid, "feedbackId": feedback.ID, "feedbackAction": action, "stateVersion": version}, NoEnqueue: true})
		return gin.H{"id": feedback.ID, "action": action, "createdAt": timestamp(feedback.CreatedAt), "stateVersion": version, "replayed": false}, e
	})
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, result)
}
