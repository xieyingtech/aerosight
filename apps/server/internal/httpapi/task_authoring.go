package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/orchestration"
	"aerosight/server/internal/taskdefinition"
	"github.com/gin-gonic/gin"
)

func (s *Server) validateTaskPublication(ctx context.Context, w *database.WriteTx, uid, pid int32, definition map[string]any) error {
	if definition["apiVersion"] != "aerosight/v2" {
		return nil
	}
	resourceQueries := map[string]string{
		"assetId":                      "select exists(select 1 from assets where project_id=$1 and id=$2 and status='available')",
		"assetIds":                     "select exists(select 1 from assets where project_id=$1 and id=$2 and status='available')",
		"deviceId":                     "select exists(select 1 from devices where project_id=$1 and id=$2)",
		"connectorId":                  "select exists(select 1 from device_adapters where project_id=$1 and id=$2)",
		"issueId":                      "select exists(select 1 from issues where project_id=$1 and id=$2)",
		"algorithmDefinitionVersionId": "select exists(select 1 from algorithm_definition_versions where project_id=$1 and id=$2 and status='published')",
		"definitionVersionId":          "select exists(select 1 from algorithm_definition_versions where project_id=$1 and id=$2 and status='published')",
	}
	defaults := map[string]any{}
	for key, value := range fhObject(fhObject(definition["inputSchema"])["properties"]) {
		if v, ok := fhObject(value)["default"]; ok {
			defaults[key] = v
		}
	}
	for key, value := range fhObject(fhObject(definition["trigger"])["inputs"]) {
		defaults[key] = value
	}
	for _, raw := range definition["steps"].([]any) {
		step := fhObject(raw)
		uses := fhString(step["uses"])
		permission := "mission:operate"
		if uses == "copilot.run" {
			permission = "agent:use"
		}
		if uses == "issue.create-or-update" {
			permission = "issue:handle"
		}
		if _, err := s.projectAccess(ctx, w.Queries, uid, pid, permission); err != nil {
			return err
		}
		for key, value := range fhObject(step["with"]) {
			query, ok := resourceQueries[key]
			if uses == "inspection.detect" && fhObject(step["with"])["source"] == "external" && key == "algorithmDefinitionVersionId" {
				query = `select exists(select 1 from algorithm_definition_versions v join algorithm_definitions d on d.id=v.algorithm_definition_id and d.project_id=v.project_id join algorithm_providers p on p.id=d.provider_id and p.project_id=d.project_id where v.project_id=$1 and v.id=$2 and v.status='published' and d.capability_code='detection' and p.status='active' and p.provider_type='http-json')`
			}
			if !ok {
				continue
			}
			if ref, ok := value.(string); ok {
				if strings.HasPrefix(ref, "steps.") {
					continue
				}
				if strings.HasPrefix(ref, "inputs.") {
					var exists bool
					value, exists = defaults[strings.TrimPrefix(ref, "inputs.")]
					if !exists {
						continue
					}
				}
			}
			values := []any{value}
			if list, ok := value.([]any); ok {
				values = list
			}
			for _, v := range values {
				id, ok := fhSafePositive(v)
				if !ok {
					return errors.New("TASK_RESOURCE_REFERENCE_INVALID")
				}
				var found bool
				if err := w.Tx.QueryRowContext(ctx, query, pid, id).Scan(&found); err != nil {
					return err
				}
				if !found {
					return errors.New("TASK_RESOURCE_SCOPE_INVALID")
				}
			}
		}
	}
	// Enable each v2 handler only after its runtime/contract acceptance. The
	// report, observation, and native evidence handlers have verified execution paths.
	for _, raw := range definition["steps"].([]any) {
		step := fhObject(raw)
		parameters := fhObject(step["with"])
		// A custom schema may refine the contract, but cannot waive it. Check
		// known literal fields even when another parameter is a runtime reference.
		var contract map[string]any
		switch step["uses"] {
		case "inspection.observe":
			switch parameters["mode"] {
			case "assets":
				contract = inspectionAssetsInputSchema()
			case "existing-flight":
				contract = inspectionExistingFlightInputSchema()
			}
		case "inspection.detect":
			switch parameters["source"] {
			case "external":
				contract = externalDetectInputSchema()
			case "flighthub-ai":
				contract = inspectionNativeDetectInputSchema()
			}
		case "copilot.run":
			if parameters["mode"] == "assessment" {
				contract = assessmentInputSchema()
			}
		case "issue.create-or-update":
			if parameters["assessmentId"] != nil {
				contract = inspectionIssueInputSchema()
			}
		case "report.generate":
			contract = inspectionReportInputSchema()
		}
		if contract != nil {
			if err := validateStaticTaskInputs(contract, parameters); err != nil {
				return &taskStepInputError{key: fhString(step["key"]), cause: err}
			}
		}

		if step["uses"] == "inspection.observe" && parameters["mode"] == "flighthub-flight" {
			// Flight selections are frozen literal publication inputs. They
			// cannot be replaced by later step outputs or trigger overrides.
			if err := validateInspectionFlightSelection(ctx, w.Tx, pid, parameters); err != nil {
				return err
			}
		}
		if step["uses"] == "copilot.run" && parameters["mode"] == "assessment" {
			var ready bool
			if err := w.Tx.QueryRowContext(ctx, `select exists(select 1 from agents where project_id=$1 and status='active' and config_json->>'kind'='copilot') and exists(select 1 from ai_providers where enabled and is_default and provider_type='openai')`, pid).Scan(&ready); err != nil {
				return err
			}
			if !ready {
				return errors.New("TASK_ASSESSMENT_PROVIDER_UNAVAILABLE")
			}
		}
		if step["uses"] != "report.generate" && !(step["uses"] == "issue.create-or-update" && parameters["assessmentId"] != nil) && !(step["uses"] == "copilot.run" && parameters["mode"] == "assessment") && !(step["uses"] == "inspection.observe" && (parameters["mode"] == "assets" || parameters["mode"] == "existing-flight")) && !(step["uses"] == "inspection.detect" && (parameters["source"] == "flighthub-ai" || parameters["source"] == "external")) {
			return errors.New("TASK_CAPABILITY_NOT_DEPLOYED:" + fhString(step["uses"]))
		}
		if resolved, err := orchestration.ResolveReferences(parameters, orchestration.Context{Inputs: defaults, Steps: map[string]map[string]any{}}); err == nil {
			if _, err = taskdefinition.MergeInputs(taskJSON(step["inputSchema"]), nil, fhObject(resolved)); err != nil {
				return &taskStepInputError{key: fhString(step["key"]), cause: err}
			}
			values := fhObject(resolved)
			if step["uses"] == "inspection.observe" && values["mode"] == "existing-flight" {
				connectorID, ok := fhSafePositive(values["connectorId"])
				if !ok {
					return errors.New("TASK_RESOURCE_REFERENCE_INVALID")
				}
				var found bool
				err = w.Tx.QueryRowContext(ctx, `select exists(select 1 from connector_remote_resources r join device_adapters a on a.id=r.connector_instance_id and a.project_id=r.project_id where r.project_id=$1 and r.connector_instance_id=$2 and r.resource_kind='flight-task' and r.remote_id=$3 and r.status='active' and a.adapter_type='dji-flighthub2')`, pid, connectorID, fhString(values["flightUuid"])).Scan(&found)
				if err != nil {
					return err
				}
				if !found {
					return errors.New("TASK_RESOURCE_SCOPE_INVALID")
				}
			}
		}
	}
	return nil
}

func (s *Server) taskAuthorFailure(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	code := err.Error()
	status := 400
	if strings.Contains(code, "REVISION") || strings.Contains(code, "IDEMPOTENCY") {
		status = 409
	}
	if strings.Contains(code, "ACCESS") || strings.Contains(code, "PERMISSION") {
		status = 403
	}
	if !strings.HasPrefix(code, "TASK_") && !strings.Contains(code, "ACCESS") && !strings.Contains(code, "IDEMPOTENCY") {
		code = "TASK_AUTHORING_FAILED"
	}
	s.failure(c, status, code)
}

func (s *Server) validateTaskSource(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "TASK_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	if _, err = s.projectAccess(c.Request.Context(), s.queries, uid, pid, "mission:operate"); err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		s.failure(c, 400, "TASK_SOURCE_INPUT_INVALID")
		return
	}
	definition, _, _, err := parseTaskAuthorInput(body)
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	tx, err := s.db.BeginTx(c.Request.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	defer tx.Rollback()
	issues := []gin.H{}
	if err := s.validateTaskPublication(c.Request.Context(), &database.WriteTx{Tx: tx, Queries: sqlcgen.New(tx)}, uid, pid, definition); err != nil {
		issues = append(issues, taskValidationIssue(err))
	}
	hash, _ := taskdefinition.Hash(definition)
	c.JSON(200, gin.H{"normalizedDefinition": definition, "definitionHash": hash, "canPublish": len(issues) == 0, "issues": issues})
}

func (s *Server) createTask(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 404, "TASK_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		s.failure(c, 400, "TASK_SOURCE_INPUT_INVALID")
		return
	}
	definition, format, source, err := parseTaskAuthorInput(body)
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	script := "typed-task-v1"
	if definition["apiVersion"] == "aerosight/v2" {
		script = "typed-task-v2"
	}
	key := fhString(body["idempotencyKey"])
	if len(key) < 1 || len(key) > 200 || strings.TrimSpace(key) != key {
		s.failure(c, 400, "TASK_IDEMPOTENCY_KEY_INVALID")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "task.create", ResourceType: "task", Input: gin.H{"name": definition["name"], "format": format}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (database.IdempotentResult[gin.H], error) {
		return database.Idempotent(ctx, w, database.IdempotencyContext{ProjectID: pid, TeamID: access.TeamID, ActorKey: "user:" + strconv.Itoa(int(uid)), Operation: "task.create", Key: key, Request: gin.H{"definition": definition, "format": format, "source": source}}, func() (gin.H, error) {
			tid, err := w.Queries.TaskAuthorCreateTask(ctx, sqlcgen.TaskAuthorCreateTaskParams{ProjectID: pid, TeamID: access.TeamID, Name: fhString(definition["name"]), Script: script, Description: sql.NullString{String: fhString(definition["description"]), Valid: definition["description"] != nil}, TriggerType: fhString(fhObject(definition["trigger"])["type"]), CreatedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
			if err != nil {
				return nil, err
			}
			draft, err := w.Queries.TaskDraftCreate(ctx, sqlcgen.TaskDraftCreateParams{P1: pid, P2: access.TeamID, P3: tid, P4: 1, P5: taskJSON(definition), P6: script, P7: taskJSON(definition["inputSchema"]), P8: taskJSON(definition["trigger"]), P9: int32(fhOptionalNumber(definition, "concurrencyLimit")), P10: sql.NullInt32{Int32: uid, Valid: true}})
			if err != nil {
				return nil, err
			}
			if err = saveTaskSteps(ctx, w.Queries, pid, access.TeamID, draft.ID, definition); err != nil {
				return nil, err
			}
			if err = saveTaskAuthor(ctx, w.Queries, pid, draft.ID, definition, format, source); err != nil {
				return nil, err
			}
			return gin.H{"taskId": tid, "versionId": draft.ID, "revision": 1, "status": "disabled", "definition": definition}, nil
		})
	})
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	status := 201
	if result.Replayed {
		status = 200
	}
	result.Value["replayed"] = result.Replayed
	c.JSON(status, result.Value)
}

func (s *Server) updateTaskState(c *gin.Context) {
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
	var body struct {
		Status string `json:"status"`
	}
	if strictJSON(c, &body) != nil || body.Status != "active" && body.Status != "disabled" {
		s.failure(c, 400, "TASK_STATE_INVALID")
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "task.update_state", ResourceType: "task", ResourceID: strconv.FormatInt(tid, 10), Input: body}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		if err := w.Queries.LockTaskTrigger(ctx, sqlcgen.LockTaskTriggerParams{Column1: pid, Column2: int32(tid)}); err != nil {
			return nil, err
		}
		rows, err := w.Queries.TaskDraftTask(ctx, sqlcgen.TaskDraftTaskParams{P1: pid, P2: int32(tid)})
		task, err := fhFirstRow(rows, err, "TASK_NOT_FOUND")
		if err != nil {
			return nil, err
		}
		if body.Status == "active" {
			vid := fhOptionalNumber(task, "currentVersionId")
			if vid <= 0 {
				return nil, errors.New("TASK_PUBLISHED_VERSION_REQUIRED")
			}
			rows, err := w.Queries.TaskDraftLockVersion(ctx, sqlcgen.TaskDraftLockVersionParams{P1: pid, P2: vid})
			version, err := fhFirstRow(rows, err, "TASK_VERSION_NOT_FOUND")
			if err != nil {
				return nil, err
			}
			if version["status"] != "published" {
				return nil, errors.New("TASK_PUBLISHED_VERSION_REQUIRED")
			}
			if definition := fhObject(version["definition"]); definition["apiVersion"] == "aerosight/v2" {
				if err := s.validateTaskPublication(ctx, w, uid, pid, definition); err != nil {
					return nil, err
				}
			}
		}
		count, err := w.Queries.TaskAuthorSetState(ctx, sqlcgen.TaskAuthorSetStateParams{ProjectID: pid, ID: int32(tid), Status: body.Status, AuthorizedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
		if err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, errors.New("TASK_NOT_FOUND")
		}
		return gin.H{"taskId": tid, "status": body.Status}, nil
	})
	if err != nil {
		s.taskAuthorFailure(c, err)
		return
	}
	c.JSON(200, result)
}

// References have already been checked against earlier outputs/input properties
// by parseTaskV2. Only their eventual values are deferred; other constraints and
// required keys remain enforceable at publication.
func validateStaticTaskInputs(schema, parameters map[string]any) error {
	properties := fhObject(schema["properties"])
	for key, value := range parameters {
		if ref, ok := value.(string); ok && taskConditionRef.MatchString(ref) {
			if _, known := properties[key]; known {
				properties[key] = map[string]any{}
			}
		}
	}
	_, err := taskdefinition.MergeInputs(taskJSON(schema), nil, parameters)
	return err
}
