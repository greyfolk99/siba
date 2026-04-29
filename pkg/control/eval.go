package control

import (
	"fmt"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/parser"
	"github.com/greyfolk99/siba/pkg/scope"
	"github.com/greyfolk99/siba/pkg/types"
)

// EvaluateIf evaluates an @if condition against the given scope
func EvaluateIf(condition string, s *scope.Scope) (bool, *ast.Diagnostic) {
	left, op, right := parseCondition(condition)

	// single variable (truthy check)
	if op == "" {
		leftVal, diag := resolveOperand(left, s)
		if diag != nil {
			return false, diag
		}
		return types.TruthyValue(*leftVal), nil
	}

	leftVal, diag := resolveOperand(left, s)
	if diag != nil {
		return false, diag
	}
	rightVal, diag := resolveOperand(right, s)
	if diag != nil {
		return false, diag
	}

	// type check
	if d := types.CheckComparison(*leftVal, *rightVal, op); d != nil {
		return false, d
	}

	result, err := types.CompareValues(*leftVal, *rightVal, op)
	if err != nil {
		return false, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E040",
			Message:  fmt.Sprintf("comparison error: %v", err),
		}
	}

	return result, nil
}

// ForIteration represents one iteration of a @for loop
type ForIteration struct {
	Scope *scope.Scope
	Value ast.Value
}

// EvaluateFor evaluates a @for loop, creating a scope for each iteration
func EvaluateFor(iterator string, collection string, parentScope *scope.Scope) ([]ForIteration, *ast.Diagnostic) {
	// resolve collection variable
	collVar, ok := parentScope.Resolve(collection)
	if !ok {
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E041",
			Message:  fmt.Sprintf("undefined variable in @for: %s", collection),
		}
	}

	if collVar.Value == nil {
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E042",
			Message:  fmt.Sprintf("@for collection %q has no value", collection),
		}
	}

	// type check
	if d := types.CheckIterable(*collVar.Value); d != nil {
		return nil, d
	}

	var iterations []ForIteration
	for i, elem := range collVar.Value.Array {
		iterScope := scope.NewScope(
			fmt.Sprintf("__for_%s_%d__", iterator, i),
			scope.ScopeControlBlock,
			parentScope,
		)

		// bind iterator variable (let, so it can shadow parent variables)
		iterVar := ast.Variable{
			Name:       iterator,
			Mutability: ast.MutLet,
			Value:      &elem,
			Type:       parser.InferType(elem),
		}
		iterScope.Declare(iterator, iterVar)

		iterations = append(iterations, ForIteration{
			Scope: iterScope,
			Value: elem,
		})
	}

	return iterations, nil
}

func resolveOperand(operand string, s *scope.Scope) (*ast.Value, *ast.Diagnostic) {
	operand = strings.TrimSpace(operand)

	// try as literal value first (quoted string, number, bool, null)
	if isLiteral(operand) {
		if val, err := parser.ParseValue(operand); err == nil {
			return &val, nil
		}
	}

	// try as variable reference
	// handle property access: obj.prop
	if dotIdx := strings.LastIndex(operand, "."); dotIdx >= 0 {
		objName := operand[:dotIdx]
		propName := operand[dotIdx+1:]
		if v, ok := s.Resolve(objName); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
			if prop, ok := v.Value.Object[propName]; ok {
				return &prop, nil
			}
		}
	}

	// simple variable
	if v, ok := s.Resolve(operand); ok {
		if v.Value != nil {
			return v.Value, nil
		}
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E043",
			Message:  fmt.Sprintf("variable %q has no value", operand),
		}
	}

	return nil, &ast.Diagnostic{
		Severity: ast.SeverityError,
		Code:     "E044",
		Message:  fmt.Sprintf("undefined variable or invalid literal: %s", operand),
	}
}

func isLiteral(s string) bool {
	s = strings.TrimSpace(s)
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return true
	}
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-') {
		return true
	}
	return false
}

// parseCondition parses "@if expr" into left, operator, right
// Supports: ==, !=, >=, <=, >, <
// Single operand returns op="" (truthy check)
// Skips operators inside quoted strings.
func parseCondition(expr string) (left, op, right string) {
	expr = strings.TrimSpace(expr)

	// scan for operators outside of quoted strings
	inString := false
	for i := 0; i < len(expr); i++ {
		if expr[i] == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		// check two-char operators first
		if i+1 < len(expr) {
			twoChar := expr[i : i+2]
			switch twoChar {
			case "==", "!=", ">=", "<=":
				return strings.TrimSpace(expr[:i]), twoChar, strings.TrimSpace(expr[i+2:])
			}
		}

		// single-char operators
		switch expr[i] {
		case '>':
			return strings.TrimSpace(expr[:i]), ">", strings.TrimSpace(expr[i+1:])
		case '<':
			return strings.TrimSpace(expr[:i]), "<", strings.TrimSpace(expr[i+1:])
		}
	}

	// no operator found — truthy check
	return expr, "", ""
}

