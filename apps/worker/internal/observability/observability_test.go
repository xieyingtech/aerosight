package observability

import (
	"encoding/json"
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

func TestFlightHubRedactorCoversStructuredSnapshots(t *testing.T) {
	type temporaryCredentials struct {
		AccessKeyID     string `json:"access_key_id"`
		AccessKeySecret string `json:"access_key_secret"`
		SecurityToken   string `json:"security_token"`
	}
	secrets := []string{
		"organization-token-plaintext",
		"1581FABCDEFGHIJKL",
		"plain-serial-from-decrypt",
		"temporary-access-id",
		"temporary-access-secret",
		"temporary-security-token",
		"signed-value-plaintext",
		"live-playback-token",
		"raw-upstream-secret",
	}
	payload := map[string]any{
		"x_user_token": "organization-token-plaintext",
		"device_sn":    "1581FABCDEFGHIJKL",
		"sn_decrypt_result": map[string]any{
			"mapping": map[string]string{"ciphertext": "plain-serial-from-decrypt"},
		},
		"storage_sts": temporaryCredentials{
			AccessKeyID: "temporary-access-id", AccessKeySecret: "temporary-access-secret", SecurityToken: "temporary-security-token",
		},
		"download_url":   "https://objects.example/item?X-Amz-Signature=signed-value-plaintext",
		"live_token":     "live-playback-token",
		"upstream_error": fmt.Errorf("raw-upstream-secret Bearer organization-token-plaintext sn=1581FABCDEFGHIJKL"),
	}

	for name, snapshot := range map[string]string{
		"log":   String(payload),
		"trace": string(mustJSON(Redact(payload))),
		"audit": string(mustJSON(map[string]any{"input": Redact(payload)})),
		"api":   string(mustJSON(map[string]any{"data": Redact(payload)})),
	} {
		for _, secret := range secrets {
			if strings.Contains(snapshot, secret) {
				t.Fatalf("%s snapshot leaked %q: %s", name, secret, snapshot)
			}
		}
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
