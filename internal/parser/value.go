package parser

import (
	"strconv"
	"strings"

	"github.com/hjseo/siba/internal/ast"
)

// ParseValue parses a raw string into a Value
func ParseValue(raw string) (ast.Value, error) {
	raw = strings.TrimSpace(raw)

	if raw == "null" {
		return ast.Value{Kind: ast.TypeNull, IsNull: true, Raw: raw}, nil
	}
	if raw == "true" {
		return ast.Value{Kind: ast.TypeBoolean, Bool: true, Raw: raw}, nil
	}
	if raw == "false" {
		return ast.Value{Kind: ast.TypeBoolean, Bool: false, Raw: raw}, nil
	}

	// string
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		s := raw[1 : len(raw)-1]
		return ast.Value{Kind: ast.TypeString, Str: s, Raw: raw}, nil
	}

	// number
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return ast.Value{Kind: ast.TypeNumber, Num: n, Raw: raw}, nil
	}

	// array
	if len(raw) >= 2 && raw[0] == '[' && raw[len(raw)-1] == ']' {
		inner := raw[1 : len(raw)-1]
		elements := splitTopLevel(inner, ',')
		var arr []ast.Value
		for _, elem := range elements {
			v, err := ParseValue(strings.TrimSpace(elem))
			if err != nil {
				return ast.Value{}, err
			}
			arr = append(arr, v)
		}
		return ast.Value{Kind: ast.TypeArray, Array: arr, Raw: raw}, nil
	}

	// object
	if len(raw) >= 2 && raw[0] == '{' && raw[len(raw)-1] == '}' {
		inner := raw[1 : len(raw)-1]
		pairs := splitTopLevel(inner, ',')
		obj := make(map[string]ast.Value)
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			colonIdx := strings.Index(pair, ":")
			if colonIdx < 0 {
				continue
			}
			key := strings.TrimSpace(pair[:colonIdx])
			key = strings.Trim(key, "\"")
			valStr := strings.TrimSpace(pair[colonIdx+1:])
			v, err := ParseValue(valStr)
			if err != nil {
				return ast.Value{}, err
			}
			obj[key] = v
		}
		return ast.Value{Kind: ast.TypeObject, Object: obj, Raw: raw}, nil
	}

	// fallback: treat as string
	return ast.Value{Kind: ast.TypeString, Str: raw, Raw: raw}, nil
}

// InferType infers a TypeExpr from a Value
func InferType(val ast.Value) *ast.TypeExpr {
	switch val.Kind {
	case ast.TypeString:
		return &ast.TypeExpr{Kind: ast.TypeString}
	case ast.TypeNumber:
		return &ast.TypeExpr{Kind: ast.TypeNumber}
	case ast.TypeBoolean:
		return &ast.TypeExpr{Kind: ast.TypeBoolean}
	case ast.TypeNull:
		return &ast.TypeExpr{Kind: ast.TypeNull}
	case ast.TypeArray:
		if len(val.Array) > 0 {
			elemType := InferType(val.Array[0])
			return &ast.TypeExpr{Kind: ast.TypeArray, ElementType: elemType}
		}
		return &ast.TypeExpr{Kind: ast.TypeArray, ElementType: &ast.TypeExpr{Kind: ast.TypeAny}}
	case ast.TypeObject:
		fields := make(map[string]*ast.TypeExpr)
		for k, v := range val.Object {
			fields[k] = InferType(v)
		}
		return &ast.TypeExpr{Kind: ast.TypeObject, Fields: fields}
	default:
		return &ast.TypeExpr{Kind: ast.TypeAny}
	}
}

// splitTopLevel splits a string by delimiter, respecting nesting of [], {}, ""
func splitTopLevel(s string, delim byte) []string {
	var result []string
	depth := 0
	inString := false
	start := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '[' || c == '{' {
			depth++
		} else if c == ']' || c == '}' {
			depth--
		} else if c == delim && depth == 0 {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		last := strings.TrimSpace(s[start:])
		if last != "" {
			result = append(result, s[start:])
		}
	}
	return result
}
