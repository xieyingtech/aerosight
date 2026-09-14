package taskdefinition

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Author schemas must be self-contained. Never fetch a URL supplied by a Task.
type localSchemaLoader struct{}

func (localSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference forbidden: %s", url)
}

func CompileSchema(schema map[string]any) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	c.UseLoader(localSchemaLoader{})
	const location = "https://aerosight.invalid/task-schema.json"
	if err := c.AddResource(location, schema); err != nil {
		return nil, fmt.Errorf("TASK_TRIGGER_INPUT_SCHEMA_INVALID: %w", err)
	}
	result, err := c.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("TASK_TRIGGER_INPUT_SCHEMA_INVALID: %w", err)
	}
	return result, nil
}

// MergeInputs applies defaults, trigger inputs, then invocation inputs, by key.
// Nested constraints are validated without coercion or mutating the definition.
func MergeInputs(schemaJSON []byte, trigger, invocation map[string]any) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil || schema == nil {
		return nil, fmt.Errorf("TASK_TRIGGER_INPUT_SCHEMA_INVALID")
	}
	compiled, err := CompileSchema(schema)
	if err != nil {
		return nil, err
	}
	merged := map[string]any{}
	props, _ := schema["properties"].(map[string]any)
	for key, raw := range props {
		p, _ := raw.(map[string]any)
		if v, ok := p["default"]; ok {
			merged[key] = v
		}
	}
	for k, v := range trigger {
		merged[k] = v
	}
	for k, v := range invocation {
		merged[k] = v
	}
	// JSON-copy to isolate nested input maps and normalize Go integer types.
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("TASK_TRIGGER_INPUT_INVALID: %w", err)
	}
	if err = json.Unmarshal(encoded, &merged); err != nil {
		return nil, err
	}
	if err = compiled.Validate(merged); err != nil {
		return nil, fmt.Errorf("TASK_TRIGGER_INPUT_INVALID: %w", err)
	}
	return merged, nil
}
