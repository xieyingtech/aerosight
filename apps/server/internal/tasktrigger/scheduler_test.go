package tasktrigger

import (
	"testing"
	"time"
)

func TestCronMatchesStandardFields(t *testing.T) {
	moment := time.Date(2026, time.September, 1, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		expression string
		want       bool
	}{
		{"*/5 10 * * *", true},
		{"31 10 * * *", false},
		{"30 9-11 * * 2", true},
		{"30 9-11 * * 1", false},
		{"0,30 * 1 9 *", true},
	}
	for _, item := range cases {
		got, err := CronMatches(item.expression, moment)
		if err != nil || got != item.want {
			t.Fatalf("CronMatches(%q)=%v,%v want %v", item.expression, got, err, item.want)
		}
	}
}

func TestCronRejectsMalformedOrOutOfRangeFields(t *testing.T) {
	for _, expression := range []string{"* * *", "60 * * * *", "*/0 * * * *", "* 25 * * *"} {
		if _, err := CronMatches(expression, time.Now()); err == nil {
			t.Fatalf("CronMatches accepted %q", expression)
		}
	}
}

func TestValidateInputsRejectsMissingAndUnknownValues(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"issueId":{"type":"integer"}},"required":["issueId"],"additionalProperties":false}`)
	if err := validateInputs(schema, map[string]any{}); err == nil || err.Error() != "TASK_TRIGGER_INPUT_REQUIRED:issueId" {
		t.Fatalf("expected missing input rejection, got %v", err)
	}
	if err := validateInputs(schema, map[string]any{"issueId": 7, "projectId": 3}); err == nil || err.Error() != "TASK_TRIGGER_INPUT_UNKNOWN:projectId" {
		t.Fatalf("expected unknown input rejection, got %v", err)
	}
	if err := validateInputs(schema, map[string]any{"issueId": 7}); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}
