package taskdefinition

import (
	"strings"
	"testing"
)

func TestMergeInputsPrecedenceAndRecursiveValidation(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"count":{"type":"integer","default":1},"ids":{"type":"array","minItems":1,"items":{"type":"integer","minimum":1}},"settings":{"type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"],"additionalProperties":false}},"required":["ids","settings"],"additionalProperties":false}`)
	trigger := map[string]any{"count": 2, "ids": []any{1}, "settings": map[string]any{"enabled": true}}
	out, err := MergeInputs(schema, trigger, map[string]any{"count": 3})
	if err != nil || out["count"] != float64(3) {
		t.Fatalf("merge: %+v %v", out, err)
	}
	out["settings"].(map[string]any)["enabled"] = false
	if trigger["settings"].(map[string]any)["enabled"] != true {
		t.Fatal("mutated trigger")
	}
	for name, input := range map[string]map[string]any{
		"wrong nested type": {"settings": map[string]any{"enabled": "true"}},
		"wrong item type":   {"ids": []any{"1"}}, "empty array": {"ids": []any{}},
		"null": {"ids": nil}, "unknown": {"extra": 1}, "fraction": {"count": 1.5},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := MergeInputs(schema, trigger, input); err == nil {
				t.Fatal("invalid inputs accepted")
			}
		})
	}
	if _, err := MergeInputs(schema, nil, nil); err == nil {
		t.Fatal("missing required inputs accepted")
	}
}
func TestSchemaCompilationDoesNotLoadRemoteResources(t *testing.T) {
	for _, s := range []map[string]any{{"$ref": "https://127.0.0.1/private"}, {"type": "object", "properties": map[string]any{"broken": 42}}} {
		if _, err := CompileSchema(s); err == nil || !strings.Contains(err.Error(), "SCHEMA_INVALID") {
			t.Fatalf("schema accepted: %v", err)
		}
	}
}
