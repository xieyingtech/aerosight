package orchestration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func fixtureContext() Context {
	return Context{Inputs: map[string]any{"minimum": json.Number("0.8")}, Steps: map[string]map[string]any{
		"collect": {"assetId": json.Number("41")},
		"detect":  {"confidence": json.Number("0.91"), "category": "suspected-construction", "spatialRelation": "inside"},
	}}
}

func TestResolveReferencesOnlyUsesInputsAndStepOutputs(t *testing.T) {
	resolved, err := ResolveReferences(map[string]any{"assetId": "steps.collect.outputs.assetId", "literal": "keep"}, fixtureContext())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"assetId": json.Number("41"), "literal": "keep"}
	if !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved = %#v, want %#v", resolved, want)
	}
	if _, err := fixtureContext().Read("process.env.AUTH_SECRET"); err == nil || !strings.Contains(err.Error(), "FORBIDDEN") {
		t.Fatalf("arbitrary reference accepted: %v", err)
	}
}

func TestEvaluateConditionIsDeterministicAndAudited(t *testing.T) {
	raw := []byte(`{"op":"all","conditions":[{"op":"gte","left":{"ref":"steps.detect.outputs.confidence"},"right":{"ref":"inputs.minimum"}},{"op":"eq","left":{"ref":"steps.detect.outputs.spatialRelation"},"right":{"value":"inside"}}]}`)
	first, err := EvaluateCondition(raw, fixtureContext())
	if err != nil || !first.Result {
		t.Fatalf("condition failed: %+v %v", first, err)
	}
	second, err := EvaluateCondition(raw, fixtureContext())
	if err != nil || !reflect.DeepEqual(first, second) || len(first.References) != 3 {
		t.Fatalf("condition not deterministic: %+v %+v %v", first, second, err)
	}
}

func TestEvaluateConditionFailsOnMissingTypeAndCode(t *testing.T) {
	for _, raw := range []string{
		`{"op":"eq","left":{"ref":"steps.detect.outputs.missing"},"right":{"value":"x"}}`,
		`{"op":"eq","left":{"ref":"steps.detect.outputs.category"},"right":{"value":1}}`,
		`{"op":"gte","left":{"ref":"steps.detect.outputs.category"},"right":{"value":1}}`,
		`{"op":"script","source":"return true"}`,
	} {
		if _, err := EvaluateCondition([]byte(raw), fixtureContext()); err == nil {
			t.Fatalf("invalid condition accepted: %s", raw)
		}
	}
}

func TestEvaluateConditionReturnsFalseWithoutSideEffects(t *testing.T) {
	audit, err := EvaluateCondition([]byte(`{"op":"lt","left":{"ref":"steps.detect.outputs.confidence"},"right":{"ref":"inputs.minimum"}}`), fixtureContext())
	if err != nil {
		t.Fatal(err)
	}
	if audit.Result || len(audit.References) != 2 || audit.ConditionHash == "" {
		t.Fatalf("unexpected false condition audit: %+v", audit)
	}
}
