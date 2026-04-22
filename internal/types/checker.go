package types

import (
	"fmt"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/parser"
)

// CheckAssignment validates that a value matches the variable's declared type
func CheckAssignment(v ast.Variable, val ast.Value) *ast.Diagnostic {
	if v.Type == nil {
		return nil
	}

	valType := parser.InferType(val)
	if !ast.IsAssignable(v.Type, valType) {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E030",
			Message: fmt.Sprintf("type mismatch: variable %q expects %s, got %s",
				v.Name, ast.TypeToString(v.Type), ast.TypeToString(valType)),
			Range: ast.Range{Start: v.Position, End: v.Position},
		}
	}
	return nil
}

// CheckComparison validates that two values can be compared with the given operator
func CheckComparison(left, right ast.Value, op string) *ast.Diagnostic {
	// == and != work on any types
	if op == "==" || op == "!=" {
		return nil
	}

	// ordered operators require same numeric/string types
	if left.Kind != right.Kind {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E031",
			Message: fmt.Sprintf("cannot compare %s %s %s: incompatible types",
				ast.TypeToString(parser.InferType(left)), op, ast.TypeToString(parser.InferType(right))),
		}
	}

	if left.Kind != ast.TypeNumber && left.Kind != ast.TypeString {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E032",
			Message: fmt.Sprintf("operator %s requires number or string operands, got %s",
				op, ast.TypeToString(parser.InferType(left))),
		}
	}

	return nil
}

// CheckIterable validates that a value can be iterated with @for
func CheckIterable(val ast.Value) *ast.Diagnostic {
	if val.Kind != ast.TypeArray {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E033",
			Message: fmt.Sprintf("@for requires an array, got %s",
				ast.TypeToString(parser.InferType(val))),
		}
	}
	return nil
}

// CompareValues compares two values with the given operator
func CompareValues(left, right ast.Value, op string) (bool, error) {
	switch op {
	case "==":
		return valuesEqual(left, right), nil
	case "!=":
		return !valuesEqual(left, right), nil
	case ">":
		return compareOrdered(left, right, op)
	case "<":
		return compareOrdered(left, right, op)
	case ">=":
		return compareOrdered(left, right, op)
	case "<=":
		return compareOrdered(left, right, op)
	default:
		return false, fmt.Errorf("unknown operator: %s", op)
	}
}

func valuesEqual(a, b ast.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case ast.TypeString:
		return a.Str == b.Str
	case ast.TypeNumber:
		return a.Num == b.Num
	case ast.TypeBoolean:
		return a.Bool == b.Bool
	case ast.TypeNull:
		return true
	default:
		return a.Raw == b.Raw
	}
}

func compareOrdered(a, b ast.Value, op string) (bool, error) {
	if a.Kind != b.Kind {
		return false, fmt.Errorf("cannot compare %v with %v", a.Kind, b.Kind)
	}

	switch a.Kind {
	case ast.TypeNumber:
		switch op {
		case ">":
			return a.Num > b.Num, nil
		case "<":
			return a.Num < b.Num, nil
		case ">=":
			return a.Num >= b.Num, nil
		case "<=":
			return a.Num <= b.Num, nil
		}
	case ast.TypeString:
		switch op {
		case ">":
			return a.Str > b.Str, nil
		case "<":
			return a.Str < b.Str, nil
		case ">=":
			return a.Str >= b.Str, nil
		case "<=":
			return a.Str <= b.Str, nil
		}
	}

	return false, fmt.Errorf("operator %s not supported for type %v", op, a.Kind)
}

// TruthyValue returns whether a value is truthy
func TruthyValue(v ast.Value) bool {
	switch v.Kind {
	case ast.TypeBoolean:
		return v.Bool
	case ast.TypeString:
		return v.Str != ""
	case ast.TypeNumber:
		return v.Num != 0
	case ast.TypeNull:
		return false
	case ast.TypeArray:
		return len(v.Array) > 0
	case ast.TypeObject:
		return len(v.Object) > 0
	default:
		return false
	}
}
