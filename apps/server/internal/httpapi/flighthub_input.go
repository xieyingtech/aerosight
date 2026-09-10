package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf16"
)

var errFHInput = errors.New("invalid_request")

// normalizeFHInput handles the JSON Schema subset emitted by the frozen Zod
// request contracts. It normalizes only strings explicitly marked x-trim.
func normalizeFHInput(value any, schema map[string]any) (any, error) {
	return normalizeContractInput(value, schema, schema, 0)
}
func normalizeContractInput(value any, schema, root map[string]any, depth int) (any, error) {
	if depth > 128 {
		return nil, errFHInput
	}
	if ref, ok := schema["$ref"].(string); ok {
		var resolved any = root
		if !strings.HasPrefix(ref, "#/") {
			return nil, errFHInput
		}
		for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, ok := resolved.(map[string]any)
			if !ok {
				return nil, errFHInput
			}
			resolved = m[segment]
		}
		target, ok := resolved.(map[string]any)
		if !ok {
			return nil, errFHInput
		}
		return normalizeContractInput(value, target, root, depth+1)
	}
	for _, union := range []string{"oneOf", "anyOf"} {
		if choices, ok := schema[union].([]any); ok {
			var result any
			count := 0
			for _, choice := range choices {
				candidate, err := normalizeContractInput(value, choice.(map[string]any), root, depth+1)
				if err == nil {
					result = candidate
					count++
				}
			}
			if count == 0 || union == "oneOf" && count != 1 {
				return nil, errFHInput
			}
			return result, nil
		}
	}
	if schema["x-trim"] == true {
		if s, ok := value.(string); ok {
			value = strings.TrimSpace(s)
		}
	}
	if constant, ok := schema["const"]; ok && !reflect.DeepEqual(value, constant) {
		return nil, errFHInput
	}
	if values, ok := schema["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			if reflect.DeepEqual(value, candidate) {
				found = true
			}
		}
		if !found {
			return nil, errFHInput
		}
	}
	switch schema["type"] {
	case "object":
		source, ok := value.(map[string]any)
		if !ok {
			return nil, errFHInput
		}
		properties, _ := schema["properties"].(map[string]any)
		copySource := map[string]any{}
		for k, v := range source {
			copySource[k] = v
		}
		source = copySource
		for key, p := range properties {
			prop := p.(map[string]any)
			if _, exists := source[key]; !exists {
				if d, ok := prop["default"]; ok {
					source[key] = d
				}
			}
		}
		required, _ := schema["required"].([]any)
		for _, r := range required {
			if _, ok := source[r.(string)]; !ok {
				return nil, errFHInput
			}
		}
		result := map[string]any{}
		for key, v := range source {
			if property, ok := properties[key]; ok {
				item, err := normalizeContractInput(v, property.(map[string]any), root, depth+1)
				if err != nil {
					return nil, err
				}
				result[key] = item
			} else {
				if schema["additionalProperties"] == false {
					return nil, errFHInput
				}
				result[key] = v
			}
		}
		return result, nil
	case "array":
		source, ok := value.([]any)
		if !ok {
			return nil, errFHInput
		}
		if minimum, ok := schema["minItems"].(float64); ok && float64(len(source)) < minimum {
			return nil, errFHInput
		}
		if maximum, ok := schema["maxItems"].(float64); ok && float64(len(source)) > maximum {
			return nil, errFHInput
		}
		result := []any{}
		prefix, _ := schema["prefixItems"].([]any)
		for i, v := range source {
			itemSchema := schema["items"]
			if i < len(prefix) {
				itemSchema = prefix[i]
			}
			if itemSchema == false {
				return nil, errFHInput
			}
			if items, ok := itemSchema.(map[string]any); ok {
				item, err := normalizeContractInput(v, items, root, depth+1)
				if err != nil {
					return nil, err
				}
				result = append(result, item)
			} else {
				result = append(result, v)
			}
		}
		return result, nil
	case "string":
		s, ok := value.(string)
		if !ok {
			return nil, errFHInput
		}
		length := float64(len(utf16.Encode([]rune(s))))
		if min, ok := schema["minLength"].(float64); ok && length < min {
			return nil, errFHInput
		}
		if max, ok := schema["maxLength"].(float64); ok && length > max {
			return nil, errFHInput
		}
		if pattern, ok := schema["pattern"].(string); ok {
			rx, err := regexp.Compile(pattern)
			if err != nil || !rx.MatchString(s) {
				return nil, errFHInput
			}
		}
	case "number", "integer":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || schema["type"] == "integer" && math.Trunc(n) != n {
			return nil, errFHInput
		}
		for _, limit := range []string{"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"} {
			if bound, ok := schema[limit].(float64); ok {
				if limit == "minimum" && n < bound || limit == "maximum" && n > bound || limit == "exclusiveMinimum" && n <= bound || limit == "exclusiveMaximum" && n >= bound {
					return nil, errFHInput
				}
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return nil, errFHInput
		}
	}
	return value, nil
}
func parseFHInput(raw []byte, schemaJSON []byte) (map[string]any, error) {
	var source any
	var schema map[string]any
	if json.Unmarshal(raw, &source) != nil || json.Unmarshal(schemaJSON, &schema) != nil {
		return nil, errFHInput
	}
	result, err := normalizeFHInput(source, schema)
	if err != nil {
		return nil, err
	}
	obj, ok := result.(map[string]any)
	if !ok {
		return nil, errFHInput
	}
	if obj["action"] == "map-element-update" {
		if request, ok := obj["request"].(map[string]any); !ok || len(request) == 0 {
			return nil, errFHInput
		}
	}
	return obj, nil
}
