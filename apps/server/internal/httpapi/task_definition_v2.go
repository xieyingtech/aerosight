package httpapi

import (
	"aerosight/server/internal/tasktrigger"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aerosight/server/internal/taskdefinition"
)

func taskObjectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": []any{}, "additionalProperties": false}
}

// The registry describes known contracts; availability is checked separately at
// publish time so incomplete implementations cannot produce executable versions.
func taskV2Capability(uses string) (map[string]any, bool) {
	outputs := map[string][]string{
		"inspection.observe": {"observationId"}, "inspection.detect": {"evidenceSetId"},
		"copilot.run":            {"assessmentId", "sessionId", "jobId"},
		"issue.create-or-update": {"issueIds", "issueId"}, "report.generate": {"reportId", "reportVersionId", "version", "completeness", "dataGaps"},
		"algorithm.run": {"algorithmRunId", "detectionIds"}, "device.collect": {"assetId"}, "device.command": {"commandId"},
	}
	names, ok := outputs[uses]
	if !ok {
		return nil, false
	}
	props := map[string]any{}
	for _, name := range names {
		props[name] = map[string]any{}
	}
	schema := taskObjectSchema(props)
	if uses == "report.generate" {
		for _, name := range []string{"reportId", "reportVersionId", "completeness"} {
			props[name] = map[string]any{"type": "string", "minLength": float64(1)}
		}
		props["version"] = map[string]any{"type": "integer", "minimum": float64(1)}
		props["dataGaps"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		schema["required"] = []any{"reportId", "reportVersionId", "version", "completeness", "dataGaps"}
	}
	if uses == "copilot.run" {
		props["assessmentId"] = map[string]any{"type": "string", "minLength": float64(1)}
		props["sessionId"] = map[string]any{"type": "integer", "minimum": float64(1)}
		props["jobId"] = map[string]any{"type": "string", "minLength": float64(1)}
	}
	if uses == "inspection.detect" {
		props["evidenceSetId"] = map[string]any{"type": "string", "minLength": float64(1)}
		schema["required"] = []any{"evidenceSetId"}
	}
	if uses == "inspection.observe" {
		props["observationId"] = map[string]any{"type": "string", "minLength": float64(1)}
		schema["required"] = []any{"observationId"}
	}
	return schema, true
}

func inspectionIssueInputSchema() map[string]any {
	schema := taskObjectSchema(map[string]any{"assessmentId": map[string]any{"type": "string", "minLength": float64(1)}, "title": map[string]any{"type": "string", "minLength": float64(1), "maxLength": float64(300)}, "priority": map[string]any{"type": "string", "enum": []any{"low", "medium", "high", "critical"}}})
	schema["required"] = []any{"assessmentId"}
	return schema
}

func assessmentInputSchema() map[string]any {
	schema := taskObjectSchema(map[string]any{"mode": map[string]any{"type": "string", "const": "assessment"}, "evidenceSetId": map[string]any{"type": "string", "minLength": float64(1)}, "temperature": map[string]any{"type": "number", "minimum": float64(0), "maximum": float64(2), "default": float64(0.2)}})
	schema["required"] = []any{"mode", "evidenceSetId"}
	return schema
}

func externalDetectInputSchema() map[string]any {
	schema := taskObjectSchema(map[string]any{
		"source":                       map[string]any{"type": "string", "const": "external"},
		"observationId":                map[string]any{"type": "string", "minLength": float64(1)},
		"algorithmDefinitionVersionId": map[string]any{"type": "integer", "minimum": float64(1)},
		"maxImages":                    map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(1000), "default": float64(64)},
		"parameters":                   map[string]any{"type": "object", "default": map[string]any{}},
	})
	schema["required"] = []any{"source", "observationId", "algorithmDefinitionVersionId"}
	return schema
}

func inspectionFlightInputSchema() map[string]any {
	version := taskObjectSchema(map[string]any{
		"waylineId":     map[string]any{"type": "string", "minLength": float64(1)},
		"updatedAt":     map[string]any{"type": "integer", "minimum": float64(1)},
		"sizeBytes":     map[string]any{"type": "integer", "minimum": float64(1)},
		"remoteVersion": map[string]any{"type": "string", "minLength": float64(1)},
	})
	version["required"] = []any{"waylineId", "updatedAt", "sizeBytes", "remoteVersion"}
	schema := taskObjectSchema(map[string]any{
		"mode":              map[string]any{"type": "string", "const": "flighthub-flight"},
		"connectorId":       map[string]any{"type": "integer", "minimum": float64(1)},
		"deviceId":          map[string]any{"type": "integer", "minimum": float64(1)},
		"waylineResourceId": map[string]any{"type": "integer", "minimum": float64(1)},
		"waylineVersion":    version,
		"schedulerOwner":    map[string]any{"type": "string", "const": "aerosight"},
		"taskType":          map[string]any{"type": "string", "const": "immediate"},
	})
	schema["required"] = []any{"mode", "connectorId", "deviceId", "waylineResourceId", "waylineVersion", "schedulerOwner", "taskType"}
	return schema
}

func inspectionAssetsInputSchema() map[string]any {
	inputSchema := taskObjectSchema(map[string]any{
		"mode":             map[string]any{"type": "string", "const": "assets"},
		"assetIds":         map[string]any{"type": "array", "minItems": float64(1), "maxItems": float64(1000), "items": map[string]any{"type": "integer", "minimum": float64(1)}},
		"maxImages":        map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(1000), "default": float64(64)},
		"scopeDescription": map[string]any{"type": "string", "minLength": float64(1)},
	})
	inputSchema["required"] = []any{"mode", "assetIds"}
	return inputSchema
}

func inspectionExistingFlightInputSchema() map[string]any {
	schema := taskObjectSchema(map[string]any{
		"mode":                map[string]any{"type": "string", "const": "existing-flight"},
		"connectorId":         map[string]any{"type": "integer", "minimum": float64(1)},
		"flightUuid":          map[string]any{"type": "string", "minLength": float64(1), "maxLength": float64(256)},
		"assetIds":            map[string]any{"type": "array", "minItems": float64(1), "maxItems": float64(1000), "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": float64(1)}},
		"maxImages":           map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(1000), "default": float64(64)},
		"scopeDescription":    map[string]any{"type": "string", "minLength": float64(1), "pattern": "\\S"},
		"confirmLimitedScope": map[string]any{"type": "boolean", "default": false},
	})
	schema["required"] = []any{"mode", "connectorId", "flightUuid"}
	schema["if"] = map[string]any{"properties": map[string]any{"confirmLimitedScope": map[string]any{"const": true}}, "required": []any{"confirmLimitedScope"}}
	schema["then"] = map[string]any{"required": []any{"assetIds", "scopeDescription"}}
	return schema
}

func inspectionNativeDetectInputSchema() map[string]any {
	schema := taskObjectSchema(map[string]any{"source": map[string]any{"type": "string", "const": "flighthub-ai"}, "observationId": map[string]any{"type": "string", "minLength": float64(1)}})
	schema["required"] = []any{"source", "observationId"}
	return schema
}

func inspectionReportInputSchema() map[string]any {
	return taskObjectSchema(map[string]any{"scope": map[string]any{"type": "string", "const": "current-run"}})
}

func parseTaskV2(value map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var source map[string]any
	if err = json.Unmarshal(raw, &source); err != nil {
		return nil, err
	}
	if source["apiVersion"] != "aerosight/v2" {
		return nil, errors.New("TASK_DSL_VERSION_UNSUPPORTED")
	}
	setDefault := func(m map[string]any, key string, v any) {
		if _, ok := m[key]; !ok {
			m[key] = v
		}
	}
	setDefault(source, "inputSchema", taskObjectSchema(map[string]any{}))
	setDefault(source, "concurrencyLimit", float64(1))
	steps, ok := source["steps"].([]any)
	if !ok || len(steps) == 0 || len(steps) > 64 {
		return nil, errors.New("TASK_DEFINITION_INVALID")
	}
	for _, v := range steps {
		step, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("TASK_DEFINITION_INVALID")
		}
		output, known := taskV2Capability(fhString(step["uses"]))
		if !known {
			return nil, errors.New("TASK_CAPABILITY_UNKNOWN")
		}
		setDefault(step, "name", step["key"])
		setDefault(step, "capabilityVersion", "1")
		if step["uses"] == "inspection.observe" && fhObject(step["with"])["mode"] == "flighthub-flight" {
			setDefault(step, "inputSchema", inspectionFlightInputSchema())
		}
		if step["uses"] == "inspection.observe" && fhObject(step["with"])["mode"] == "assets" {
			setDefault(step, "inputSchema", inspectionAssetsInputSchema())
		}
		if step["uses"] == "inspection.observe" && fhObject(step["with"])["mode"] == "existing-flight" {
			setDefault(step, "inputSchema", inspectionExistingFlightInputSchema())
		}
		if step["uses"] == "issue.create-or-update" && fhObject(step["with"])["assessmentId"] != nil {
			setDefault(step, "inputSchema", inspectionIssueInputSchema())
			output = taskObjectSchema(map[string]any{"issueIds": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": float64(1)}}})
			output["required"] = []any{"issueIds"}
		}
		if step["uses"] == "copilot.run" && fhObject(step["with"])["mode"] == "assessment" {
			setDefault(step, "inputSchema", assessmentInputSchema())
		}
		if step["uses"] == "inspection.detect" && fhObject(step["with"])["source"] == "external" {
			schema := externalDetectInputSchema()
			setDefault(step, "inputSchema", schema)
		}
		if step["uses"] == "inspection.detect" && fhObject(step["with"])["source"] == "flighthub-ai" {
			setDefault(step, "inputSchema", inspectionNativeDetectInputSchema())
		}
		if step["uses"] == "report.generate" {
			setDefault(step, "inputSchema", inspectionReportInputSchema())
		}
		setDefault(step, "inputSchema", taskObjectSchema(map[string]any{}))
		setDefault(step, "outputSchema", output)
		setDefault(step, "timeoutSeconds", float64(300))
		setDefault(step, "retry", map[string]any{"maxAttempts": float64(1), "backoffSeconds": float64(0)})
		setDefault(step, "onFailure", "abort")
		if step["onFailure"] == "continue" {
			return nil, errors.New("TASK_FAILURE_POLICY_UNSUPPORTED")
		}
		if retry, ok := step["retry"].(map[string]any); ok && fhOptionalNumber(retry, "maxAttempts") > 1 &&
			(step["uses"] == "device.command" || step["uses"] == "device.collect" || step["uses"] == "inspection.observe") {
			return nil, errors.New("TASK_UNSAFE_RETRY")
		}
	}
	var schema map[string]any
	if err = json.Unmarshal(taskDefinitionSchema, &schema); err != nil {
		return nil, err
	}
	properties := schema["properties"].(map[string]any)
	properties["apiVersion"] = map[string]any{"type": "string", "const": "aerosight/v2"}
	properties["concurrencyLimit"] = map[string]any{"type": "integer", "const": float64(1)}
	stepProperties := properties["steps"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	stepProperties["uses"] = map[string]any{"type": "string"}
	stepProperties["capabilityVersion"] = map[string]any{"type": "string", "const": "1"}
	for _, branch := range properties["trigger"].(map[string]any)["oneOf"].([]any) {
		trigger := branch.(map[string]any)
		trigger["properties"].(map[string]any)["inputs"] = map[string]any{"type": "object", "default": map[string]any{}}
	}
	normalized, err := normalizeFHInput(source, schema)
	if err != nil {
		return nil, errors.New("TASK_DEFINITION_INVALID")
	}
	definition := normalized.(map[string]any)
	trigger := fhObject(definition["trigger"])
	if trigger["type"] == "schedule" {
		location, err := time.LoadLocation(fhString(trigger["timezone"]))
		if err != nil {
			return nil, errors.New("TASK_SCHEDULE_TIMEZONE_INVALID")
		}
		if _, err = tasktrigger.CronMatches(fhString(trigger["cron"]), time.Now().In(location)); err != nil {
			return nil, errors.New("TASK_SCHEDULE_CRON_INVALID")
		}
	}
	if _, err := taskdefinition.CompileSchema(fhObject(definition["inputSchema"])); err != nil {
		return nil, err
	}
	prior := map[string]map[string]any{}
	for i, v := range definition["steps"].([]any) {
		step := v.(map[string]any)
		key := fhString(step["key"])
		parameters := fhObject(step["with"])
		if step["uses"] == "inspection.observe" && parameters["mode"] == "flighthub-flight" {
			if owner, ok := parameters["schedulerOwner"]; ok && owner != "aerosight" {
				return nil, errors.New("TASK_SCHEDULER_OWNER_INVALID")
			}
			for _, field := range []string{"taskType", "task_type"} {
				if kind, ok := parameters[field]; ok && kind != "immediate" {
					return nil, errors.New("TASK_FLIGHT_SCHEDULE_MODE_INVALID")
				}
			}
			parameters["schedulerOwner"] = "aerosight"
		}
		for _, field := range []string{"inputSchema", "outputSchema"} {
			if _, err := taskdefinition.CompileSchema(fhObject(step[field])); err != nil {
				return nil, fmt.Errorf("TASK_STEP_SCHEMA_INVALID:%s:%s:%w", key, field, err)
			}
		}
		if _, exists := prior[key]; exists {
			return nil, errors.New("TASK_STEP_KEY_DUPLICATE")
		}
		for _, dep := range step["dependsOn"].([]any) {
			if _, exists := prior[fhString(dep)]; !exists {
				return nil, errors.New("TASK_STEP_DEPENDENCY_INVALID")
			}
		}
		if err := validateTaskConditionRefs(step["condition"]); err != nil {
			return nil, errors.New("TASK_CONDITION_REFERENCE_INVALID")
		}
		if err := validateV2References(step["condition"], true, prior, fhObject(definition["inputSchema"])); err != nil {
			return nil, err
		}
		if err := validateV2References(step["with"], false, prior, fhObject(definition["inputSchema"])); err != nil {
			return nil, fmt.Errorf("TASK_STEP_REFERENCE_INVALID:%d:%w", i, err)
		}
		prior[key] = step
	}
	return definition, nil
}

func validateV2References(value any, condition bool, prior map[string]map[string]any, inputs map[string]any) error {
	validate := func(ref string) error {
		if !taskConditionRef.MatchString(ref) {
			return errors.New("TASK_REFERENCE_INVALID")
		}
		parts := strings.Split(ref, ".")
		var schema map[string]any
		var path []string
		if parts[0] == "inputs" {
			schema = inputs
			path = parts[1:]
		} else {
			step, exists := prior[parts[1]]
			if !exists {
				return errors.New("TASK_REFERENCE_NOT_PREVIOUS")
			}
			schema = fhObject(step["outputSchema"])
			path = parts[3:]
			canonical, _ := taskV2Capability(fhString(step["uses"]))
			if step["uses"] == "issue.create-or-update" && fhObject(step["with"])["assessmentId"] != nil {
				canonical = taskObjectSchema(map[string]any{"issueIds": map[string]any{}})
			}
			for _, segment := range path {
				child, known := fhObject(canonical["properties"])[segment]
				if !known {
					return errors.New("TASK_REFERENCE_CAPABILITY_FIELD_MISSING")
				}
				canonical = fhObject(child)
			}

		}
		for _, segment := range path {
			if segment == "__proto__" || segment == "constructor" || segment == "prototype" {
				return errors.New("TASK_REFERENCE_INVALID")
			}
			child, exists := fhObject(schema["properties"])[segment]
			if !exists {
				return errors.New("TASK_REFERENCE_FIELD_MISSING")
			}
			schema = fhObject(child)
		}
		return nil
	}
	switch value := value.(type) {
	case string:
		if !condition && taskConditionRef.MatchString(value) {
			return validate(value)
		}
	case map[string]any:
		for key, child := range value {
			if condition && key == "value" {
				continue
			}
			if condition && key == "ref" {
				ref, ok := child.(string)
				if !ok {
					return errors.New("TASK_REFERENCE_INVALID")
				}
				if err := validate(ref); err != nil {
					return err
				}
				continue
			}
			if err := validateV2References(child, condition, prior, inputs); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateV2References(child, condition, prior, inputs); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseTaskAuthorInput(body map[string]any) (map[string]any, string, string, error) {
	if source, exists := body["source"]; exists {
		text, ok := source.(string)
		format := fhString(body["sourceFormat"])
		if !ok || body["definition"] != nil {
			return nil, "", "", errors.New("TASK_SOURCE_INPUT_INVALID")
		}
		value, err := taskdefinition.Parse(format, text)
		if err != nil {
			return nil, "", "", err
		}
		definition, err := parseTaskDefinition(value)
		return definition, format, text, err
	}
	definition, err := parseTaskDefinition(body["definition"])
	raw, _ := json.Marshal(body["definition"])
	return definition, "json", string(raw), err
}
