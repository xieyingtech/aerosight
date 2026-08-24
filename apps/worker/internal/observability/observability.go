package observability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	safeCorrelationID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
	sensitiveKey      = regexp.MustCompile(`(?i)authorization|cookie|credential|password|secret|token|api[-_]?key`)
	bearerValue       = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	querySecret       = regexp.MustCompile(`(?i)([?&](?:token|api_key|key|secret)=)[^&\s]+`)
)

func CorrelationID(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if safeCorrelationID.MatchString(candidate) {
		return candidate
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "correlation-id-unavailable"
	}
	return hex.EncodeToString(bytes)
}

func Redact(value any) any {
	switch typed := value.(type) {
	case string:
		return redactText(typed)
	case error:
		return redactText(typed.Error())
	case map[string]any:
		clean := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey.MatchString(key) {
				clean[key] = "[REDACTED]"
			} else {
				clean[key] = Redact(item)
			}
		}
		return clean
	case []any:
		clean := make([]any, len(typed))
		for index, item := range typed {
			clean[index] = Redact(item)
		}
		return clean
	default:
		return typed
	}
}

func redactText(value string) string {
	value = bearerValue.ReplaceAllString(value, "Bearer [REDACTED]")
	return querySecret.ReplaceAllString(value, `${1}[REDACTED]`)
}

func EventFields(requestID, eventID string) map[string]any {
	fields := map[string]any{"request_id": CorrelationID(requestID)}
	if eventID != "" {
		fields["event_id"] = CorrelationID(eventID)
	}
	return fields
}

func String(value any) string {
	return fmt.Sprint(Redact(value))
}
