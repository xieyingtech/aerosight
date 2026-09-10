package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func validateUserTaskInvocation(input map[string]any) error {
	kind, _ := input["type"].(string)
	allowed := map[string]bool{"type": true, "idempotencyKey": true, "occurredAt": true, "inputs": true}
	switch kind {
	case "manual":
	case "api":
		allowed["key"] = true
	case "webhook":
		allowed["source"] = true
		allowed["deliveryId"] = true
	default:
		return errors.New("TASK_TRIGGER_INTERNAL_SOURCE_REQUIRED")
	}
	for key := range input {
		if !allowed[key] {
			return errors.New("TASK_TRIGGER_INPUT_INVALID")
		}
	}
	key, ok := input["idempotencyKey"].(string)
	if !ok || len(utf16.Encode([]rune(key))) < 1 || len(utf16.Encode([]rune(key))) > 400 {
		return errors.New("TASK_TRIGGER_INPUT_INVALID")
	}
	occurred, ok := input["occurredAt"].(string)
	if _, err := time.Parse(time.RFC3339Nano, occurred); !ok || err != nil || !strings.HasSuffix(occurred, "Z") {
		return errors.New("TASK_TRIGGER_INPUT_INVALID")
	}
	for _, field := range []string{"key", "source", "deliveryId"} {
		if allowed[field] {
			if v, ok := input[field].(string); !ok || v == "" {
				return errors.New("TASK_TRIGGER_INPUT_INVALID")
			}
		}
	}
	if _, ok := input["inputs"]; !ok {
		input["inputs"] = map[string]any{}
	}
	if _, ok := input["inputs"].(map[string]any); !ok {
		return errors.New("TASK_TRIGGER_INPUT_INVALID")
	}
	return nil
}

func planUserTaskTrigger(version sqlcgen.ReadTaskTriggerVersionRow, input map[string]any, active int64, userID int32) ([]byte, error) {
	if version.TaskVersionStatus != "published" {
		return nil, errors.New("TASK_TRIGGER_VERSION_NOT_PUBLISHED")
	}
	if version.TaskStatus != "active" {
		return nil, errors.New("TASK_TRIGGER_TASK_DISABLED")
	}
	var trigger, schema map[string]any
	if err := json.Unmarshal(version.TriggerJson, &trigger); err != nil {
		return nil, err
	}
	if trigger["type"] != input["type"] {
		return nil, errors.New("TASK_TRIGGER_TYPE_MISMATCH")
	}
	if trigger["type"] == "api" && trigger["key"] != input["key"] {
		return nil, errors.New("TASK_TRIGGER_API_KEY_MISMATCH")
	}
	if trigger["type"] == "webhook" && trigger["source"] != input["source"] {
		return nil, errors.New("TASK_TRIGGER_WEBHOOK_SOURCE_MISMATCH")
	}
	if active >= int64(version.ConcurrencyLimit) {
		return nil, errors.New("TASK_TRIGGER_CONCURRENCY_LIMIT")
	}
	if err := json.Unmarshal(version.InputSchemaJson, &schema); err != nil || schema == nil {
		return nil, errors.New("TASK_TRIGGER_INPUT_SCHEMA_INVALID")
	}
	inputs := input["inputs"].(map[string]any)
	required, _ := schema["required"].([]any)
	for _, value := range required {
		if key, ok := value.(string); ok {
			if _, found := inputs[key]; !found {
				return nil, errors.New("TASK_TRIGGER_INPUT_REQUIRED:" + key)
			}
		}
	}
	if schema["additionalProperties"] == false {
		properties, _ := schema["properties"].(map[string]any)
		for key := range inputs {
			if _, ok := properties[key]; !ok {
				return nil, errors.New("TASK_TRIGGER_INPUT_UNKNOWN:" + key)
			}
		}
	}
	details := map[string]any{}
	for key, value := range input {
		if key != "inputs" && !(input["type"] == "api" && key == "key") {
			details[key] = value
		}
	}
	details["actor"] = gin.H{"type": "user", "id": strconv.FormatInt(int64(userID), 10)}
	return json.Marshal(gin.H{"trigger": details, "inputs": inputs})
}

func (s *Server) triggerTaskRun(c *gin.Context) {
	fail := func(err error) {
		code := err.Error()
		status := 409
		if strings.Contains(code, "ACCESS_DENIED") || strings.Contains(code, "PERMISSION") {
			status = 403
		} else if strings.Contains(code, "NOT_FOUND") {
			status = 404
		} else if strings.HasPrefix(code, "TASK_TRIGGER_INPUT_") {
			status = 400
		}
		s.failure(c, status, code)
	}
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "TASK_NOT_FOUND")
		return
	}
	tid, err := strconv.ParseInt(c.Param("taskId"), 10, 32)
	if err != nil || tid <= 0 {
		s.failure(c, 404, "TASK_NOT_FOUND")
		return
	}
	var input map[string]any
	if err := c.ShouldBindJSON(&input); err != nil || input == nil {
		s.failure(c, 400, "TASK_TRIGGER_INPUT_INVALID")
		return
	}
	if err := validateUserTaskInvocation(input); err != nil {
		fail(err)
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	key := input["type"].(string) + ":" + input["idempotencyKey"].(string)
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "task_run.trigger", ResourceType: "task", ResourceID: strconv.FormatInt(tid, 10), Input: gin.H{"type": input["type"], "idempotencyKey": input["idempotencyKey"], "inputs": input["inputs"]}, PolicyResult: map[string]any{"permission": "mission:operate", "triggerType": input["type"]}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		q := w.Queries
		if err := q.LockTaskTrigger(ctx, sqlcgen.LockTaskTriggerParams{Column1: pid, Column2: int32(tid)}); err != nil {
			return nil, err
		}
		version, err := q.ReadTaskTriggerVersion(ctx, sqlcgen.ReadTaskTriggerVersionParams{ProjectID: pid, ID: int32(tid)})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("TASK_TRIGGER_VERSION_NOT_FOUND")
		}
		if err != nil {
			return nil, err
		}
		vid := sql.NullInt64{Int64: version.TaskVersionID, Valid: true}
		triggerKey := sql.NullString{String: key, Valid: true}
		existing, err := q.ReadTriggeredRun(ctx, sqlcgen.ReadTriggeredRunParams{ProjectID: pid, TaskVersionID: vid, TriggerKey: triggerKey})
		if err == nil {
			return gin.H{"taskRunId": existing.ID, "status": existing.Status, "replayed": true}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		active, err := q.CountActiveTriggeredRuns(ctx, sqlcgen.CountActiveTriggeredRunsParams{ProjectID: pid, TaskVersionID: vid})
		if err != nil {
			return nil, err
		}
		snapshot, err := planUserTaskTrigger(version, input, active, uid)
		if err != nil {
			return nil, err
		}
		run, err := q.InsertTriggeredRun(ctx, sqlcgen.InsertTriggeredRunParams{ProjectID: pid, TeamID: version.TeamID, TaskID: version.TaskID, TaskVersionID: vid, TriggerSource: input["type"].(string), TriggerKey: triggerKey, InputSnapshotJson: snapshot, CreatedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
		if err != nil {
			return nil, err
		}
		if err := q.InsertTriggeredSteps(ctx, sqlcgen.InsertTriggeredStepsParams{ProjectID: pid, TaskVersionID: version.TaskVersionID, TaskRunID: run.ID, Column4: key}); err != nil {
			return nil, err
		}
		_, err = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: version.TeamID, EventID: uuid.NewString(), EventType: "task_run.triggered", Payload: gin.H{"taskRunId": run.ID, "taskVersionId": version.TaskVersionID, "triggerType": input["type"]}})
		return gin.H{"taskRunId": run.ID, "status": run.Status, "replayed": false}, err
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), "TASK_TRIGGER_") || strings.Contains(err.Error(), "ACCESS_DENIED") {
			fail(err)
		} else {
			s.failure(c, 409, "TASK_TRIGGER_FAILED")
		}
		return
	}
	status := 201
	if result["replayed"] == true {
		status = 200
	}
	c.JSON(status, result)
}
