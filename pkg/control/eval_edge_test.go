// eval_edge_test.go tests edge cases and boundary conditions for the control-flow
// evaluation logic: operator-like characters inside string literals, nil values,
// negative numbers, diagnostic propagation, iterator shadowing, cross-type
// comparison, and ValueToString formatting for all types.
package control

import (
	"testing"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/types"
)

// --- Edge case tests from review ---

// TestParseCondition_OperatorInStringLiteral verifies that operator characters (==, !=, >, <=) inside quoted string literals are not mistaken for condition operators.
func TestParseCondition_OperatorInStringLiteral(t *testing.T) {
	tests := []struct {
		input               string
		left, op, right string
	}{
		{`title == "a==b"`, "title", "==", `"a==b"`},
		{`name != "x!=y"`, "name", "!=", `"x!=y"`},
		{`label == "a>b"`, "label", "==", `"a>b"`},
		{`val == "a<=b"`, "val", "==", `"a<=b"`},
	}

	for _, tt := range tests {
		left, op, right := parseCondition(tt.input)
		if left != tt.left || op != tt.op || right != tt.right {
			t.Errorf("parseCondition(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, left, op, right, tt.left, tt.op, tt.right)
		}
	}
}

// TestEvaluateIf_VariableNoValue verifies that a variable with a nil value in a truthy check produces diagnostic E043.
func TestEvaluateIf_VariableNoValue(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"x": {Name: "x", Mutability: ast.MutConst, Value: nil},
	})
	_, diag := EvaluateIf("x", s)
	if diag == nil {
		t.Fatal("expected diagnostic for nil value")
	}
	if diag.Code != "E043" {
		t.Fatalf("expected E043, got %s", diag.Code)
	}
}

// TestEvaluateIf_NumberLiteralComparison verifies that == correctly compares a numeric variable against an unquoted number literal.
func TestEvaluateIf_NumberLiteralComparison(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"count": {Name: "count", Mutability: ast.MutConst, Value: numVal(5)},
	})
	result, diag := EvaluateIf("count == 5", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_NegativeNumber verifies that conditions involving negative number values evaluate correctly.
func TestEvaluateIf_NegativeNumber(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"temp": {Name: "temp", Mutability: ast.MutConst, Value: numVal(-10)},
	})
	result, diag := EvaluateIf("temp < 0", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}


// TestEvaluateFor_IteratorShadowsParent verifies that the iterator variable shadows a same-named parent variable without mutating the parent scope.
func TestEvaluateFor_IteratorShadowsParent(t *testing.T) {
	parent := makeScope(map[string]ast.Variable{
		"item": {Name: "item", Mutability: ast.MutLet, Value: strVal("parent_val")},
		"items": {
			Name:  "items",
			Value: &ast.Value{Kind: ast.TypeArray, Array: []ast.Value{{Kind: ast.TypeString, Str: "child_val"}}},
		},
	})
	iters, diag := EvaluateFor("item", "items", parent)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	v, _ := iters[0].Scope.Resolve("item")
	if v.Value.Str != "child_val" {
		t.Fatalf("expected 'child_val', got %q", v.Value.Str)
	}
	pv, _ := parent.Resolve("item")
	if pv.Value.Str != "parent_val" {
		t.Fatalf("expected parent 'item' unchanged, got %q", pv.Value.Str)
	}
}


// TestEvaluateIf_EmptyCondition verifies that an empty condition string produces a diagnostic rather than a panic.
func TestEvaluateIf_EmptyCondition(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	_, diag := EvaluateIf("", s)
	if diag == nil {
		t.Fatal("expected diagnostic for empty condition")
	}
}

// TestCompareValues_DifferentTypesEquality verifies that == between different types (string vs number) returns false without error.
func TestCompareValues_DifferentTypesEquality(t *testing.T) {
	a := ast.Value{Kind: ast.TypeString, Str: "42"}
	b := ast.Value{Kind: ast.TypeNumber, Num: 42}
	result, err := types.CompareValues(a, b, "==")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Fatal("expected false: string '42' != number 42")
	}
}

// TestValueToString_EmptyRaw verifies that ValueToString for numbers with an empty Raw field still produces a correct decimal string.
func TestValueToString_EmptyRaw(t *testing.T) {
	// Number with empty Raw should still produce correct string
	v := ast.Value{Kind: ast.TypeNumber, Num: 42}
	s := ast.ValueToString(v)
	if s != "42" {
		t.Fatalf("expected '42', got %q", s)
	}

	// Float with empty Raw
	v2 := ast.Value{Kind: ast.TypeNumber, Num: 3.14}
	s2 := ast.ValueToString(v2)
	if s2 != "3.14" {
		t.Fatalf("expected '3.14', got %q", s2)
	}
}

// TestValueToString_AllTypes verifies that ValueToString produces the expected string representation for every supported value type.
func TestValueToString_AllTypes(t *testing.T) {
	tests := []struct {
		val    ast.Value
		expect string
	}{
		{ast.Value{Kind: ast.TypeString, Str: "hello"}, "hello"},
		{ast.Value{Kind: ast.TypeNumber, Num: 8080, Raw: "8080"}, "8080"},
		{ast.Value{Kind: ast.TypeNumber, Num: 3.14, Raw: "3.14"}, "3.14"},
		{ast.Value{Kind: ast.TypeBoolean, Bool: true}, "true"},
		{ast.Value{Kind: ast.TypeBoolean, Bool: false}, "false"},
		{ast.Value{Kind: ast.TypeNull}, "null"},
		{ast.Value{Kind: ast.TypeArray, Raw: "[1,2,3]"}, "[1,2,3]"},
	}

	for _, tt := range tests {
		result := ast.ValueToString(tt.val)
		if result != tt.expect {
			t.Errorf("ValueToString(%v) = %q, want %q", tt.val, result, tt.expect)
		}
	}
}
