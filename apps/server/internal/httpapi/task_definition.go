package httpapi

import (
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
)

//go:embed task_definition_input.json
var taskDefinitionSchema []byte

//go:embed task_legacy_definition_input.json
var taskLegacyDefinitionSchema []byte
var taskConditionRef = regexp.MustCompile(`^(?:inputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*|steps\.[A-Za-z][A-Za-z0-9_-]*\.outputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*)$`)

func validateTaskConditionRefs(v any) error {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "ref" {
				ref, ok := v.(string)
				if !ok || !taskConditionRef.MatchString(ref) {
					return errFHInput
				}
				for _, part := range strings.Split(ref, ".") {
					if part == "__proto__" || part == "prototype" || part == "constructor" {
						return errFHInput
					}
				}
			} else if e := validateTaskConditionRefs(v); e != nil {
				return e
			}
		}
	case []any:
		for _, v := range x {
			if e := validateTaskConditionRefs(v); e != nil {
				return e
			}
		}
	}
	return nil
}
func parseTaskDefinition(v any) (map[string]any, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	definition, e := parseFHInput(raw, taskDefinitionSchema)
	if e != nil {
		return nil, errors.New("TASK_DEFINITION_INVALID")
	}
	keys := map[string]bool{}
	for _, v := range definition["steps"].([]any) {
		step := v.(map[string]any)
		key := fhString(step["key"])
		if keys[key] {
			return nil, errors.New("TASK_STEP_KEY_DUPLICATE")
		}
		keys[key] = true
		if e := validateTaskConditionRefs(step["condition"]); e != nil {
			return nil, errors.New("TASK_CONDITION_REFERENCE_INVALID")
		}
	}
	for _, v := range definition["steps"].([]any) {
		step := v.(map[string]any)
		for _, d := range step["dependsOn"].([]any) {
			dep := fhString(d)
			if !keys[dep] || dep == step["key"] {
				return nil, errors.New("TASK_STEP_DEPENDENCY_INVALID")
			}
		}
	}
	return definition, nil
}
func validateLegacyTaskDraft(row map[string]any, steps []map[string]any) error {
	if strings.TrimSpace(fhString(row["script"])) == "" {
		return errors.New("TASK_VERSION_SCRIPT_REQUIRED")
	}
	def := fhObject(row["definition"])
	if fhString(def["name"]) == "" {
		return errors.New("TASK_VERSION_NAME_REQUIRED")
	}
	if len(steps) == 0 {
		return errors.New("TASK_VERSION_STEPS_REQUIRED")
	}
	positions := map[int64]bool{}
	keys := map[string]bool{}
	for _, s := range steps {
		pos := fhOptionalNumber(s, "position")
		key := fhString(s["stepKey"])
		if pos <= 0 || positions[pos] {
			return errors.New("TASK_STEP_POSITION_INVALID")
		}
		if strings.TrimSpace(key) == "" || keys[key] || strings.TrimSpace(fhString(s["action"])) == "" {
			return errors.New("TASK_STEP_INVALID")
		}
		positions[pos] = true
		keys[key] = true
	}
	candidate := map[string]any{}
	for k, v := range def {
		candidate[k] = v
	}
	candidate["steps"] = steps
	raw, _ := json.Marshal(candidate)
	parsed, e := parseFHInput(raw, taskLegacyDefinitionSchema)
	if e != nil {
		return errors.New("TASK_DEFINITION_INVALID")
	}
	spatial := fhObject(parsed["spatialScope"])
	if spatial["type"] == "area" {
		for _, v := range spatial["rings"].([]any) {
			ring := v.([]any)
			if !reflect.DeepEqual(ring[0], ring[len(ring)-1]) {
				return errors.New("TASK_SPATIAL_RING_INVALID")
			}
		}
	}
	for _, v := range parsed["steps"].([]any) {
		s := v.(map[string]any)
		failure := fhObject(s["failurePolicy"])
		if failure["idempotency"] == "unsafe" && fhOptionalNumber(failure, "maxRetries") > 0 {
			return errors.New("TASK_UNSAFE_RETRY")
		}
		media := fhObject(s["mediaRequirements"])
		if media["required"] == true && fhOptionalNumber(media, "minimumCount") < 1 {
			return errors.New("TASK_REQUIRED_MEDIA_INVALID")
		}
	}
	return nil
}
func taskJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
