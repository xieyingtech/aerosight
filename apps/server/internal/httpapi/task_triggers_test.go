package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"encoding/json"
	"strings"
	"testing"
)

func TestUserTaskInvocationRejectsInternalSources(t *testing.T) {
	for _, kind := range []string{"schedule", "upstream", "copilot"} {
		if err := validateUserTaskInvocation(map[string]any{"type": kind}); err == nil || err.Error() != "TASK_TRIGGER_INTERNAL_SOURCE_REQUIRED" {
			t.Fatalf("accepted internal source %s", kind)
		}
	}
	input := map[string]any{"type": "manual", "idempotencyKey": "run-1", "occurredAt": "2026-09-10T00:00:00Z"}
	if err := validateUserTaskInvocation(input); err != nil {
		t.Fatal(err)
	}
	if _, ok := input["inputs"].(map[string]any); !ok {
		t.Fatal("missing default inputs")
	}
}

func TestUserTaskTriggerPreservesGatesAndRedactsAPIKey(t *testing.T) {
	version := sqlcgen.ReadTaskTriggerVersionRow{TaskVersionStatus: "published", TaskStatus: "active", ConcurrencyLimit: 1,
		TriggerJson: json.RawMessage(`{"type":"api","key":"private-key"}`), InputSchemaJson: json.RawMessage(`{"required":["area"],"properties":{"area":{}},"additionalProperties":false}`)}
	input := map[string]any{"type": "api", "key": "private-key", "idempotencyKey": "run-1", "occurredAt": "2026-09-10T00:00:00Z", "inputs": map[string]any{"area": "north"}}
	if err := validateUserTaskInvocation(input); err != nil {
		t.Fatal(err)
	}
	raw, err := planUserTaskTrigger(version, input, 0, 42)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-key") || !strings.Contains(string(raw), `"id":"42"`) {
		t.Fatalf("unsafe actor snapshot: %s", raw)
	}
	if _, err := planUserTaskTrigger(version, input, 1, 42); err == nil || err.Error() != "TASK_TRIGGER_CONCURRENCY_LIMIT" {
		t.Fatal("missing concurrency gate")
	}
	input["inputs"] = map[string]any{}
	if _, err := planUserTaskTrigger(version, input, 0, 42); err == nil || err.Error() != "TASK_TRIGGER_INPUT_REQUIRED:area" {
		t.Fatal("missing required input gate")
	}
	input["inputs"] = map[string]any{"area": "north", "extra": true}
	if _, err := planUserTaskTrigger(version, input, 0, 42); err == nil || err.Error() != "TASK_TRIGGER_INPUT_UNKNOWN:extra" {
		t.Fatal("missing unknown input gate")
	}
}
