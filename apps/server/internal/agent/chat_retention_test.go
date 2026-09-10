package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestChatRetention(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u3000", "\v", "\ufeff"} {
		text, _ := SanitizeChatMessage("Bearer"+space+"hidden", nil)
		if text != "[authorization-redacted]" {
			t.Fatalf("unicode whitespace %q", text)
		}
	}
	text, calls := SanitizeChatMessage("查看 https://media.example/a.jpg?expires=99&signature=abc 使用 sk-secretcredential123", []any{map[string]any{
		"name": "query_assets", "status": "succeeded", "summary": "Authorization: secret-token", "rawResult": map[string]any{"bytes": "must-not-persist"}, "apiKey": "must-not-persist",
		"evidenceRefs": []any{map[string]any{"type": "asset", "id": "4", "version": "sha256:abc", "href": "https://media.example/a?token=secret"}, map[string]any{"type": "asset", "id": "5", "version": "1", "href": "/projects/1/assets/5"}},
	}})
	encoded, _ := json.Marshal(calls)
	for _, secret := range []string{"must-not-persist", "secretcredential123", "secret-token", "token=secret"} {
		if strings.Contains(text+string(encoded), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	refs := calls[0]["evidenceRefs"].([]map[string]any)
	if len(refs) != 2 || refs[0]["href"] != nil || refs[1]["href"] != "/projects/1/assets/5" {
		t.Fatalf("refs %+v", refs)
	}
	if !strings.Contains(text, "[temporary-url-redacted]") || !strings.Contains(text, "[credential-redacted]") {
		t.Fatalf("redaction %s", text)
	}
	for _, value := range []any{nil, "invalid", []any{nil, 1, map[string]any{"name": "x", "status": "invalid"}, map[string]any{"name": 1, "status": "succeeded"}}} {
		_, out := SanitizeChatMessage("", value)
		if out == nil || len(out) != 0 {
			t.Fatalf("invalid %+v", out)
		}
	}
	many := make([]any, 60)
	for i := range many {
		many[i] = map[string]any{"name": strings.Repeat("😀", 60), "status": "succeeded", "summary": strings.Repeat("中", 2200)}
	}
	text, calls = SanitizeChatMessage(strings.Repeat("😀", 11000), many)
	if len(utf16.Encode([]rune(text))) != 20000 || !utf8.ValidString(text) || len(calls) != 50 || len(utf16.Encode([]rune(calls[0]["name"].(string)))) != 100 || len([]rune(calls[0]["summary"].(string))) != 2000 {
		t.Fatal("retention limits")
	}
}
