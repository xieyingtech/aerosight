package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"strconv"
	"strings"
)

func (s *Server) taskDraft(c *gin.Context) {
	fail := func(e error) {
		code := e.Error()
		status := 400
		if strings.Contains(code, "ACCESS") || strings.Contains(code, "PERMISSION") {
			status = 403
		}
		if !strings.HasPrefix(code, "TASK_") && code != "PROJECT_ACCESS_DENIED" {
			code = "TASK_VERSION_UPDATE_FAILED"
		}
		s.failure(c, status, code)
	}
	pid, e := projectID(c)
	if e != nil {
		fail(errors.New("TASK_NOT_FOUND"))
		return
	}
	tid64, e := strconv.ParseInt(c.Param("taskId"), 10, 32)
	if e != nil || tid64 <= 0 {
		fail(errors.New("TASK_NOT_FOUND"))
		return
	}
	tid := int32(tid64)
	var body map[string]any
	if strictJSON(c, &body) != nil {
		fail(errors.New("TASK_VERSION_ACTION_INVALID"))
		return
	}
	action := fhString(body["action"])
	if action != "create" && action != "save" && action != "publish" {
		fail(errors.New("TASK_VERSION_ACTION_INVALID"))
		return
	}
	vid := int64(0)
	if action != "create" {
		var ok bool
		vid, ok = fhSafePositive(body["versionId"])
		if !ok {
			fail(errors.New("TASK_VERSION_NOT_FOUND"))
			return
		}
	}
	var definition map[string]any
	if action == "save" {
		definition, e = parseTaskDefinition(body["definition"])
		if e != nil {
			fail(e)
			return
		}
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, e := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if e != nil {
		fail(e)
		return
	}
	rtype, rid, aaction := "task_version", strconv.FormatInt(vid, 10), "task_version.publish"
	input := gin.H{}
	if action == "create" {
		rtype, rid, aaction = "task", strconv.FormatInt(tid64, 10), "task_version.create_draft"
	} else if action == "save" {
		aaction = "task_version.update_draft"
		keys := []any{}
		for _, v := range definition["steps"].([]any) {
			keys = append(keys, v.(map[string]any)["key"])
		}
		input = gin.H{"taskId": tid, "name": definition["name"], "triggerType": fhObject(definition["trigger"])["type"], "stepKeys": keys}
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: aaction, ResourceType: rtype, ResourceID: rid, Input: input, PolicyResult: map[string]any{"permission": "mission:operate"}}
	result, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (any, error) {
		q := w.Queries
		if action == "create" {
			return createTaskDraft(ctx, q, pid, a.TeamID, tid, uid)
		}
		if action == "save" {
			raw, e := q.TaskDraftLockDraft(ctx, sqlcgen.TaskDraftLockDraftParams{P1: pid, P2: tid, P3: vid})
			if e != nil {
				return nil, e
			}
			if len(raw) == 0 {
				return nil, errors.New("TASK_VERSION_DRAFT_NOT_FOUND")
			}
			if e = q.TaskDraftSave(ctx, sqlcgen.TaskDraftSaveParams{P1: pid, P2: tid, P3: vid, P4: taskJSON(definition), P5: taskJSON(definition["inputSchema"]), P6: taskJSON(definition["trigger"]), P7: int32(fhOptionalNumber(definition, "concurrencyLimit"))}); e != nil {
				return nil, e
			}
			if e = q.TaskDraftDeleteSteps(ctx, sqlcgen.TaskDraftDeleteStepsParams{P1: pid, P2: vid}); e != nil {
				return nil, e
			}
			for i, v := range definition["steps"].([]any) {
				step := v.(map[string]any)
				uses := fhString(step["uses"])
				action := fhString(fhObject(step["with"])["action"])
				if strings.TrimSpace(action) == "" {
					action = uses
				}
				capability := uses
				if requires := step["requires"].([]any); len(requires) > 0 {
					capability = fhString(requires[0])
				}
				retry := fhObject(step["retry"])
				idempotency := "unsafe"
				if fhOptionalNumber(retry, "maxAttempts") > 1 {
					idempotency = "safe"
				}
				failure := gin.H{"onFailure": step["onFailure"], "maxRetries": fhOptionalNumber(retry, "maxAttempts") - 1, "retryBackoffSeconds": retry["backoffSeconds"], "idempotency": idempotency}
				e = q.TaskDraftInsertStep(ctx, sqlcgen.TaskDraftInsertStepParams{P1: pid, P2: a.TeamID, P3: vid, P4: int32(i + 1), P5: fhString(step["key"]), P6: fhString(step["name"]), P7: sql.NullString{String: capability, Valid: true}, P8: action, P9: taskJSON(step["with"]), P10: taskJSON(failure), P11: taskJSON(gin.H{"required": uses == "device.collect"}), P12: uses, P13: taskJSON(step["inputSchema"]), P14: taskJSON(step["outputSchema"]), P15: pqtype.NullRawMessage{RawMessage: taskJSON(step["condition"]), Valid: step["condition"] != nil}, P16: taskJSON(step["dependsOn"]), P17: int32(fhOptionalNumber(step, "timeoutSeconds")), P18: taskJSON(retry)})
				if e != nil {
					return nil, e
				}
			}
			return gin.H{"versionId": vid, "definition": definition, "stepCount": len(definition["steps"].([]any))}, nil
		}
		raw, e := q.TaskDraftLockVersion(ctx, sqlcgen.TaskDraftLockVersionParams{P1: pid, P2: vid})
		row, e := fhFirstRow(raw, e, "TASK_VERSION_NOT_FOUND")
		if e != nil {
			return nil, e
		}
		if row["taskId"] != float64(tid) {
			return nil, errors.New("TASK_VERSION_NOT_FOUND")
		}
		if row["status"] != "draft" {
			return nil, errors.New("TASK_VERSION_NOT_DRAFT")
		}
		raw, e = q.TaskDraftSteps(ctx, sqlcgen.TaskDraftStepsParams{P1: pid, P2: vid})
		steps, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		typed, typedErr := parseTaskDefinition(row["definition"])
		if typedErr == nil {
			defs := typed["steps"].([]any)
			if len(defs) != len(steps) {
				return nil, errors.New("TASK_VERSION_DEFINITION_STEPS_MISMATCH")
			}
			for i, v := range defs {
				if v.(map[string]any)["key"] != steps[i]["stepKey"] {
					return nil, errors.New("TASK_VERSION_DEFINITION_STEPS_MISMATCH")
				}
			}
		} else {
			legacySteps := []map[string]any{}
			for _, step := range steps {
				legacySteps = append(legacySteps, step)
			}
			if e = validateLegacyTaskDraft(row, legacySteps); e != nil {
				return nil, e
			}
		}
		published, e := q.TaskDraftPublish(ctx, sqlcgen.TaskDraftPublishParams{P1: pid, P2: vid, P3: sql.NullInt32{Int32: uid, Valid: true}})
		if e != nil {
			return nil, e
		}
		def := fhObject(row["definition"])
		name, description, trigger := fhString(def["name"]), def["description"], fhString(fhObject(row["trigger"])["type"])
		if typedErr == nil {
			name, description, trigger = fhString(typed["name"]), typed["description"], fhString(fhObject(typed["trigger"])["type"])
		}
		if trigger == "" {
			trigger = "manual"
		}
		if e = q.TaskDraftUpdateTask(ctx, sqlcgen.TaskDraftUpdateTaskParams{P1: pid, P2: tid, P3: sql.NullInt64{Int64: vid, Valid: true}, P4: name, P5: sql.NullString{String: fhString(description), Valid: description != nil}, P6: trigger}); e != nil {
			return nil, e
		}
		_, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: a.TeamID, EventID: uuid.NewString(), EventType: "task_version.published", Payload: gin.H{"taskId": tid, "taskVersionId": vid, "version": row["version"]}, NoEnqueue: true})
		return published, e
	})
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, result)
}
func createTaskDraft(ctx context.Context, q *sqlcgen.Queries, pid, team, tid, uid int32) (any, error) {
	// Serialize before checking for a draft, so simultaneous create requests reuse it.
	raw, e := q.TaskDraftTask(ctx, sqlcgen.TaskDraftTaskParams{P1: pid, P2: tid})
	task, e := fhFirstRow(raw, e, "TASK_NOT_FOUND")
	if e != nil {
		return nil, e
	}
	raw, e = q.TaskDraftExisting(ctx, sqlcgen.TaskDraftExistingParams{P1: pid, P2: tid})
	if e != nil {
		return nil, e
	}
	if len(raw) > 0 {
		row, e := fhFirstRow(raw, nil, "")
		return gin.H{"draft": row, "replayed": true}, e
	}
	next, e := q.TaskDraftNextVersion(ctx, tid)
	if e != nil {
		return nil, e
	}
	def := fhObject(task["definition"])
	trigger := gin.H{"type": "manual"}
	if def["triggerType"] == "event" {
		trigger = gin.H{"type": "webhook", "source": "legacy-event"}
	} else if def["triggerType"] == "schedule" {
		cron := fhString(def["schedule"])
		if cron == "" {
			cron = "0 0 * * *"
		}
		trigger = gin.H{"type": "schedule", "cron": cron, "timezone": "UTC", "enabled": true}
	}
	source := gin.H{"definition": task["definition"], "script": task["script"], "inputSchema": gin.H{"type": "object", "properties": gin.H{}, "additionalProperties": false}, "trigger": trigger, "concurrencyLimit": float64(1)}
	current := fhOptionalNumber(task, "currentVersionId")
	if current > 0 {
		raw, e = q.TaskDraftSource(ctx, sqlcgen.TaskDraftSourceParams{P1: pid, P2: current})
		if e != nil {
			return nil, e
		}
		if len(raw) > 0 {
			source, e = fhFirstRow(raw, nil, "")
			if e != nil {
				return nil, e
			}
		}
	}
	draft, e := q.TaskDraftCreate(ctx, sqlcgen.TaskDraftCreateParams{P1: pid, P2: team, P3: tid, P4: next, P5: taskJSON(source["definition"]), P6: fhString(source["script"]), P7: taskJSON(source["inputSchema"]), P8: taskJSON(source["trigger"]), P9: int32(fhOptionalNumber(source, "concurrencyLimit")), P10: sql.NullInt32{Int32: uid, Valid: true}})
	if e != nil {
		return nil, e
	}
	if current > 0 {
		if e = q.TaskDraftCopySteps(ctx, sqlcgen.TaskDraftCopyStepsParams{P1: pid, P2: current, P3: draft.ID}); e != nil {
			return nil, e
		}
	}
	return gin.H{"draft": draft, "replayed": false}, nil
}
