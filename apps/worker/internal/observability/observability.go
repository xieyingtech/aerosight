package observability

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var (
	safeCorrelationID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
	sensitiveKey      = regexp.MustCompile(`(?i)authorization|cookie|credential|password|secret|token|api[-_]?key|access[-_]?key|(^|[-_])sn($|[-_])|(^|[-_])sts($|[-_])|serial|sn[-_]?decrypt|mapping|object[-_]?key[-_]?prefix|signed[-_]?url|playback[-_]?url|publish[-_]?url|upstream.*(error|message|body)|response[-_]?body|raw[-_]?error`)
	bearerValue       = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	querySecret       = regexp.MustCompile(`(?i)([?&](?:token|api[-_]?key|key|secret|signature|x-amz-credential|x-amz-signature|x-amz-security-token|security-token|credential)=)[^&\s]+`)
	jwtValue          = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	labeledSecret     = regexp.MustCompile(`(?i)("?(?:x-user-token|access_key_id|access_key_secret|security_token|session_token|live_token|device_sn|encrypted_sns|sn|serial_number)"?\s*[:=]\s*"?)[^",\s}&]+`)
	djiSerial         = regexp.MustCompile(`\b(?:7CT|1581F)[A-Z0-9]{8,}\b`)
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
	case nil:
		return nil
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
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(typed, &decoded) == nil {
			return Redact(decoded)
		}
		return redactText(string(typed))
	case map[string]string:
		clean := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey.MatchString(key) {
				clean[key] = "[REDACTED]"
			} else {
				clean[key] = redactText(item)
			}
		}
		return clean
	case []string:
		clean := make([]any, len(typed))
		for index, item := range typed {
			clean[index] = redactText(item)
		}
		return clean
	default:
		kind := reflect.TypeOf(typed).Kind()
		if kind != reflect.Struct && kind != reflect.Map && kind != reflect.Slice && kind != reflect.Array && kind != reflect.Pointer {
			return typed
		}
		encoded, err := json.Marshal(typed)
		if err != nil || string(encoded) == "{}" {
			return typed
		}
		var decoded any
		if json.Unmarshal(encoded, &decoded) != nil {
			return "[REDACTED]"
		}
		return Redact(decoded)
	}
}

func redactText(value string) string {
	value = bearerValue.ReplaceAllString(value, "Bearer [REDACTED]")
	value = querySecret.ReplaceAllString(value, `${1}[REDACTED]`)
	value = jwtValue.ReplaceAllString(value, "[JWT_REDACTED]")
	value = labeledSecret.ReplaceAllString(value, `${1}[REDACTED]`)
	return djiSerial.ReplaceAllString(value, "[SN_REDACTED]")
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
