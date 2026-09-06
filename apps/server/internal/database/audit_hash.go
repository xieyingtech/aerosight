package database

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AuditHash uses a fixed locale for new writes. Legacy localeCompare depended on
// the Node host locale; matching existing idempotency records also checks those collations.
func AuditHash(value any) (string, error) {
	return auditHashLocale(value, language.English)
}

func auditHashLocale(value any, locale language.Tag) (string, error) {
	raw, err := json.Marshal(auditValue(reflect.ValueOf(value)))
	if err != nil {
		return "", err
	}
	var normalized any
	if err = json.Unmarshal(raw, &normalized); err != nil {
		return "", err
	}
	var out bytes.Buffer
	comparator := collate.New(locale)
	var encode func(any) error
	scalar := func(v any) error {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return err
		}
		text := strings.TrimSuffix(b.String(), "\n")
		var unescaped strings.Builder
		for i := 0; i < len(text); i++ {
			if text[i] == '\\' && i+1 < len(text) {
				if strings.HasPrefix(text[i:], `\u2028`) {
					unescaped.WriteRune('\u2028')
					i += 5
					continue
				}
				if strings.HasPrefix(text[i:], `\u2029`) {
					unescaped.WriteRune('\u2029')
					i += 5
					continue
				}
				unescaped.WriteByte(text[i])
				i++
				unescaped.WriteByte(text[i])
				continue
			}
			unescaped.WriteByte(text[i])
		}
		text = unescaped.String()
		out.WriteString(text)
		return nil
	}
	encode = func(v any) error {
		switch item := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(item))
			for key := range item {
				keys = append(keys, key)
			}
			sort.SliceStable(keys, func(i, j int) bool {
				a, aok := arrayKey(keys[i])
				b, bok := arrayKey(keys[j])
				if aok != bok {
					return aok
				}
				if aok {
					return a < b
				}
				return comparator.CompareString(keys[i], keys[j]) < 0
			})
			out.WriteByte('{')
			for i, key := range keys {
				if i > 0 {
					out.WriteByte(',')
				}
				if err := scalar(key); err != nil {
					return err
				}
				out.WriteByte(':')
				if err := encode(item[key]); err != nil {
					return err
				}
			}
			out.WriteByte('}')
		case []any:
			out.WriteByte('[')
			for i, value := range item {
				if i > 0 {
					out.WriteByte(',')
				}
				if err := encode(value); err != nil {
					return err
				}
			}
			out.WriteByte(']')
		default:
			return scalar(v)
		}
		return nil
	}
	if err = encode(normalized); err != nil {
		return "", err
	}
	sum := sha256.Sum256(out.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func matchesAuditHash(value any, expected string) (bool, error) {
	for _, locale := range collate.Supported() {
		hash, err := auditHashLocale(value, locale)
		if err != nil {
			return false, err
		}
		if hash == expected {
			return true, nil
		}
	}
	return false, nil
}
func arrayKey(key string) (uint64, bool) {
	n, err := strconv.ParseUint(key, 10, 32)
	return n, err == nil && n < 4294967295 && strconv.FormatUint(n, 10) == key
}
func auditValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return auditValue(v.Elem())
	}
	if _, ok := v.Interface().(time.Time); ok {
		return map[string]any{}
	}
	if raw, ok := v.Interface().(json.RawMessage); ok {
		return raw
	}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		if v.Type().Key().Kind() != reflect.String {
			return v.Interface()
		}
		result := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			result[iter.Key().String()] = auditValue(iter.Value())
		}
		return result
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Interface()
		}
		result := make([]any, v.Len())
		for i := range result {
			result[i] = auditValue(v.Index(i))
		}
		return result
	default:
		return v.Interface()
	}
}
