package agent

import (
	"encoding/json"
	"regexp"
)

const chatSpace = `\s\x{000B}\p{Z}\x{FEFF}`

var chatTemporaryURL = regexp.MustCompile(`(?i)https?://[^` + chatSpace + `)\]}]+(?:signature|sig|token|expires)=[^` + chatSpace + `)\]}]+`)
var chatCredential = regexp.MustCompile(`\b(?:sk|pk)-[A-Za-z0-9_-]{12,}\b`)
var chatAuthorization = regexp.MustCompile(`(?i)\b(?:authorization[` + chatSpace + `]*:[` + chatSpace + `]*|bearer[` + chatSpace + `]+)[^` + chatSpace + `,;]+`)

func redactChatText(text string) string {
	text = chatTemporaryURL.ReplaceAllString(text, "[temporary-url-redacted]")
	text = chatCredential.ReplaceAllString(text, "[credential-redacted]")
	return chatAuthorization.ReplaceAllString(text, "[authorization-redacted]")
}

func chatTextLimit(text string, limit int) string {
	units := 0
	for index, r := range text {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > limit {
			return text[:index]
		}
		units += width
	}
	return text
}

// Store only the same minimal tool metadata as the web implementation. Raw
// arguments and results, credentials and temporary asset locators are omitted.
func SanitizeChatMessage(content string, toolCalls any) (string, []map[string]any) {
	result := []map[string]any{}
	encoded, err := json.Marshal(toolCalls)
	if err != nil {
		return chatTextLimit(redactChatText(content), 20000), result
	}
	var calls []any
	_ = json.Unmarshal(encoded, &calls)
	if len(calls) > 50 {
		calls = calls[:50]
	}
	for _, raw := range calls {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, named := item["name"].(string)
		status, stated := item["status"].(string)
		if !named || !stated {
			continue
		}
		switch status {
		case "queued", "running", "confirmation_required", "succeeded", "failed":
		default:
			continue
		}
		stored := map[string]any{"name": chatTextLimit(name, 100), "status": status}
		if summary, ok := item["summary"].(string); ok {
			summary = chatTextLimit(redactChatText(summary), 2000)
			if summary != "" {
				stored["summary"] = summary
			}
		}
		if item["truncated"] == true {
			stored["truncated"] = true
		}
		if refs, ok := item["evidenceRefs"].([]any); ok {
			if len(refs) > 100 {
				refs = refs[:100]
			}
			evidence := []map[string]any{}
			for _, rawRef := range refs {
				ref, ok := rawRef.(map[string]any)
				if !ok {
					continue
				}
				kind, k := ref["type"].(string)
				id, i := ref["id"].(string)
				version, v := ref["version"].(string)
				if !k || !i || !v {
					continue
				}
				safe := map[string]any{"type": kind, "id": id, "version": version}
				if href, ok := ref["href"].(string); ok && href != "" && !chatTemporaryURL.MatchString(href) {
					safe["href"] = href
				}
				evidence = append(evidence, safe)
			}
			if len(evidence) > 0 {
				stored["evidenceRefs"] = evidence
			}
		}
		result = append(result, stored)
	}
	return chatTextLimit(redactChatText(content), 20000), result
}
