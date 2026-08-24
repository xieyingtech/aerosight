package observability

import (
	"fmt"
	"strings"
	"testing"
)

func TestCorrelationID(t *testing.T) {
	if got := CorrelationID("request-123"); got != "request-123" {
		t.Fatalf("safe ID changed: %q", got)
	}
	if got := CorrelationID("contains spaces"); got == "contains spaces" || got == "" {
		t.Fatalf("unsafe ID was not replaced: %q", got)
	}
}

func TestRedact(t *testing.T) {
	value := Redact(map[string]any{
		"authorization": "Bearer top-secret",
		"nested": map[string]any{
			"api_key": "key-value",
			"url":     "https://example.test/run?token=token-value",
		},
		"error": fmt.Errorf("request failed with Bearer leaked-token"),
	})
	serialized := fmt.Sprint(value)
	for _, secret := range []string{"top-secret", "key-value", "token-value", "leaked-token"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("redaction leaked %q in %s", secret, serialized)
		}
	}
}
