// Package taskdefinition handles the author format independently of execution.
package taskdefinition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

const MaxSourceBytes = 256 * 1024

type InputError struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

func (e *InputError) Error() string   { return e.Code + ":" + e.Path }
func invalid(code, path string) error { return &InputError{Code: code, Path: path} }

// Parse accepts JSON-compatible YAML only. Walking the syntax tree before any
// decoding prevents aliases, tags and duplicate keys from being normalized away.
func Parse(format, source string) (map[string]any, error) {
	if len(source) == 0 || len(source) > MaxSourceBytes {
		return nil, invalid("TASK_SOURCE_SIZE_INVALID", "$")
	}
	if format != "yaml" && format != "json" {
		return nil, invalid("TASK_SOURCE_FORMAT_INVALID", "$")
	}
	if format == "json" && !json.Valid([]byte(source)) {
		return nil, invalid("TASK_SOURCE_SYNTAX_INVALID", "$")
	}
	file, err := parser.ParseBytes([]byte(source), 0)
	if err != nil {
		return nil, invalid("TASK_SOURCE_SYNTAX_INVALID", "$")
	}
	if len(file.Docs) != 1 {
		return nil, invalid("TASK_SOURCE_DOCUMENT_COUNT", "$")
	}
	value, err := convert(file.Docs[0].Body, "$", 0)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid("TASK_SOURCE_OBJECT_REQUIRED", "$")
	}
	// Use the same numeric representation as existing JSON author APIs.
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, invalid("TASK_SOURCE_VALUE_INVALID", "$")
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	return object, nil
}

func convert(node ast.Node, path string, depth int) (any, error) {
	if depth > 64 {
		return nil, invalid("TASK_SOURCE_DEPTH_LIMIT", path)
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		object := map[string]any{}
		for _, entry := range n.Values {
			key, ok := entry.Key.(*ast.StringNode)
			if !ok {
				return nil, invalid("TASK_SOURCE_STRING_KEY_REQUIRED", path)
			}
			name := key.Value
			if _, exists := object[name]; exists {
				return nil, invalid("TASK_SOURCE_DUPLICATE_KEY", path+"."+name)
			}
			value, err := convert(entry.Value, path+"."+name, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		return object, nil
	case *ast.MappingValueNode:
		return convert(&ast.MappingNode{Values: []*ast.MappingValueNode{n}}, path, depth)
	case *ast.SequenceNode:
		values := make([]any, 0, len(n.Values))
		for i, child := range n.Values {
			v, err := convert(child, fmt.Sprintf("%s[%d]", path, i), depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		return values, nil
	case *ast.StringNode:
		return n.Value, nil
	case *ast.LiteralNode:
		return n.Value.Value, nil
	case *ast.NullNode:
		return nil, nil
	case *ast.BoolNode:
		return n.GetValue(), nil
	case *ast.IntegerNode, *ast.FloatNode:
		value := node.(ast.ScalarNode).GetValue()
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, invalid("TASK_SOURCE_NUMBER_INVALID", path)
		}
		var number float64
		if json.Unmarshal(raw, &number) != nil || math.IsInf(number, 0) || math.IsNaN(number) || math.Abs(number) > 9007199254740991 {
			return nil, invalid("TASK_SOURCE_NUMBER_INVALID", path)
		}
		return number, nil
	default:
		return nil, invalid("TASK_SOURCE_NODE_FORBIDDEN", path)
	}
}

func Hash(definition map[string]any) (string, error) {
	raw, err := json.Marshal(definition)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
