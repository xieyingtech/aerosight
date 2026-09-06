package orchestration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var referencePattern = regexp.MustCompile(`^(inputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*|steps\.[A-Za-z][A-Za-z0-9_-]*\.outputs(?:\.[A-Za-z][A-Za-z0-9_-]*)*)$`)

type Context struct {
	Inputs map[string]any
	Steps  map[string]map[string]any
}

type ConditionAudit struct {
	ConditionHash string   `json:"conditionHash"`
	References    []string `json:"references"`
	Result        bool     `json:"result"`
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func (context Context) Read(reference string) (any, error) {
	if !referencePattern.MatchString(reference) {
		return nil, errors.New("TASK_REFERENCE_FORBIDDEN")
	}
	for _, segment := range strings.Split(reference, ".") {
		if segment == "__proto__" || segment == "prototype" || segment == "constructor" {
			return nil, errors.New("TASK_REFERENCE_FORBIDDEN")
		}
	}
	root := map[string]any{"inputs": context.Inputs, "steps": map[string]any{}}
	steps := root["steps"].(map[string]any)
	for key, outputs := range context.Steps {
		steps[key] = map[string]any{"outputs": outputs}
	}
	var current any = root
	for _, segment := range strings.Split(reference, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("TASK_REFERENCE_MISSING:%s", reference)
		}
		current, ok = object[segment]
		if !ok {
			return nil, fmt.Errorf("TASK_REFERENCE_MISSING:%s", reference)
		}
	}
	return current, nil
}

func ResolveReferences(value any, context Context) (any, error) {
	switch typed := value.(type) {
	case string:
		if referencePattern.MatchString(typed) {
			return context.Read(typed)
		}
		return typed, nil
	case []any:
		resolved := make([]any, len(typed))
		for index, item := range typed {
			value, err := ResolveReferences(item, context)
			if err != nil {
				return nil, err
			}
			resolved[index] = value
		}
		return resolved, nil
	case map[string]any:
		resolved := make(map[string]any, len(typed))
		for key, item := range typed {
			value, err := ResolveReferences(item, context)
			if err != nil {
				return nil, err
			}
			resolved[key] = value
		}
		return resolved, nil
	default:
		return value, nil
	}
}

func EvaluateCondition(raw []byte, context Context) (ConditionAudit, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return ConditionAudit{}, fmt.Errorf("TASK_CONDITION_INVALID: %w", err)
	}
	references := map[string]struct{}{}
	result, err := evaluate(value, context, references)
	if err != nil {
		return ConditionAudit{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ConditionAudit{}, err
	}
	digest := sha256.Sum256(canonical)
	list := make([]string, 0, len(references))
	for reference := range references {
		list = append(list, reference)
	}
	sort.Strings(list)
	return ConditionAudit{ConditionHash: hex.EncodeToString(digest[:]), References: list, Result: result}, nil
}

func evaluate(value any, context Context, references map[string]struct{}) (bool, error) {
	condition, ok := value.(map[string]any)
	if !ok {
		return false, errors.New("TASK_CONDITION_INVALID")
	}
	op, ok := condition["op"].(string)
	if !ok {
		return false, errors.New("TASK_CONDITION_INVALID")
	}
	switch op {
	case "all", "any":
		items, ok := condition["conditions"].([]any)
		if !ok || len(items) == 0 {
			return false, errors.New("TASK_CONDITION_INVALID")
		}
		for _, item := range items {
			result, err := evaluate(item, context, references)
			if err != nil {
				return false, err
			}
			if op == "all" && !result {
				return false, nil
			}
			if op == "any" && result {
				return true, nil
			}
		}
		return op == "all", nil
	case "not":
		result, err := evaluate(condition["condition"], context, references)
		return !result, err
	case "exists":
		_, err := resolveOperand(condition["target"], context, references)
		if err != nil && strings.HasPrefix(err.Error(), "TASK_REFERENCE_MISSING:") {
			return false, nil
		}
		return err == nil, err
	}
	left, err := resolveOperand(condition["left"], context, references)
	if err != nil {
		return false, err
	}
	right, err := resolveOperand(condition["right"], context, references)
	if err != nil {
		return false, err
	}
	switch op {
	case "eq", "ne":
		equal, err := valuesEqual(left, right)
		if err != nil {
			return false, err
		}
		if op == "ne" {
			equal = !equal
		}
		return equal, nil
	case "in":
		items, ok := right.([]any)
		if !ok {
			return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
		}
		for _, item := range items {
			equal, err := valuesEqual(item, left)
			if err != nil {
				continue
			}
			if equal {
				return true, nil
			}
		}
		return false, nil
	case "contains":
		if text, ok := left.(string); ok {
			needle, ok := right.(string)
			if !ok {
				return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
			}
			return strings.Contains(text, needle), nil
		}
		items, ok := left.([]any)
		if !ok {
			return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
		}
		for _, item := range items {
			equal, err := valuesEqual(item, right)
			if err != nil {
				continue
			}
			if equal {
				return true, nil
			}
		}
		return false, nil
	case "gt", "gte", "lt", "lte":
		leftNumber, leftOK := number(left)
		rightNumber, rightOK := number(right)
		if !leftOK || !rightOK {
			return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
		}
		switch op {
		case "gt":
			return leftNumber > rightNumber, nil
		case "gte":
			return leftNumber >= rightNumber, nil
		case "lt":
			return leftNumber < rightNumber, nil
		default:
			return leftNumber <= rightNumber, nil
		}
	default:
		return false, errors.New("TASK_CONDITION_INVALID")
	}
}

func valuesEqual(left, right any) (bool, error) {
	leftNumber, leftNumeric := number(left)
	rightNumber, rightNumeric := number(right)
	if leftNumeric || rightNumeric {
		if !leftNumeric || !rightNumeric {
			return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
		}
		return leftNumber == rightNumber, nil
	}
	if left == nil || right == nil {
		return left == nil && right == nil, nil
	}
	if reflect.TypeOf(left) != reflect.TypeOf(right) {
		return false, errors.New("TASK_CONDITION_TYPE_MISMATCH")
	}
	return reflect.DeepEqual(left, right), nil
}

func resolveOperand(value any, context Context, references map[string]struct{}) (any, error) {
	operand, ok := value.(map[string]any)
	if !ok || len(operand) != 1 {
		return nil, errors.New("TASK_CONDITION_INVALID")
	}
	if literal, ok := operand["value"]; ok {
		return literal, nil
	}
	reference, ok := operand["ref"].(string)
	if !ok {
		return nil, errors.New("TASK_CONDITION_INVALID")
	}
	references[reference] = struct{}{}
	return context.Read(reference)
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}
