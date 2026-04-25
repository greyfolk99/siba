// Package types tests verify type checking logic including assignment compatibility,
// comparison operators, iterability checks, value comparison, and truthiness evaluation.
package types

import (
	"testing"

	"github.com/greyfolk99/siba/internal/ast"
)

// TestCheckAssignment_Match verifies that assigning a value whose type matches the variable's declared type produces no diagnostic.
func TestCheckAssignment_Match(t *testing.T) {
	v := ast.Variable{
		Name: "port",
		Type: &ast.TypeExpr{Kind: ast.TypeNumber},
	}
	val := ast.Value{Kind: ast.TypeNumber, Num: 8080, Raw: "8080"}

	if d := CheckAssignment(v, val); d != nil {
		t.Fatalf("expected no diagnostic, got %v", d)
	}
}

// TestCheckAssignment_Mismatch verifies that assigning a string value to a number-typed variable produces an E030 diagnostic.
func TestCheckAssignment_Mismatch(t *testing.T) {
	v := ast.Variable{
		Name: "port",
		Type: &ast.TypeExpr{Kind: ast.TypeNumber},
	}
	val := ast.Value{Kind: ast.TypeString, Str: "hello", Raw: `"hello"`}

	d := CheckAssignment(v, val)
	if d == nil {
		t.Fatal("expected E030 diagnostic")
	}
	if d.Code != "E030" {
		t.Fatalf("expected E030, got %s", d.Code)
	}
}

// TestCheckAssignment_NilType verifies that a variable with no type annotation (nil Type) accepts any value without error.
func TestCheckAssignment_NilType(t *testing.T) {
	v := ast.Variable{Name: "x", Type: nil}
	val := ast.Value{Kind: ast.TypeString, Str: "anything"}

	if d := CheckAssignment(v, val); d != nil {
		t.Fatalf("nil type should accept anything, got %v", d)
	}
}

// TestCheckAssignment_AnyType verifies that a variable typed as "any" accepts values of any kind without error.
func TestCheckAssignment_AnyType(t *testing.T) {
	v := ast.Variable{
		Name: "x",
		Type: &ast.TypeExpr{Kind: ast.TypeAny},
	}
	val := ast.Value{Kind: ast.TypeNumber, Num: 42}

	if d := CheckAssignment(v, val); d != nil {
		t.Fatalf("any type should accept anything, got %v", d)
	}
}

// TestCheckAssignment_ArrayType verifies that array assignment checks element types, accepting matching element types and rejecting mismatched ones.
func TestCheckAssignment_ArrayType(t *testing.T) {
	v := ast.Variable{
		Name: "tags",
		Type: &ast.TypeExpr{Kind: ast.TypeArray, ElementType: &ast.TypeExpr{Kind: ast.TypeString}},
	}

	// correct: string[]
	val := ast.Value{
		Kind:  ast.TypeArray,
		Array: []ast.Value{{Kind: ast.TypeString, Str: "a"}, {Kind: ast.TypeString, Str: "b"}},
	}
	if d := CheckAssignment(v, val); d != nil {
		t.Fatalf("expected no diagnostic for matching array, got %v", d)
	}

	// mismatch: number[]
	val2 := ast.Value{
		Kind:  ast.TypeArray,
		Array: []ast.Value{{Kind: ast.TypeNumber, Num: 1}},
	}
	if d := CheckAssignment(v, val2); d == nil {
		t.Fatal("expected diagnostic for number[] assigned to string[]")
	}
}

// TestCheckAssignment_UnionType verifies that union types accept values matching any member type and reject values that match none.
func TestCheckAssignment_UnionType(t *testing.T) {
	v := ast.Variable{
		Name: "x",
		Type: &ast.TypeExpr{
			Kind: ast.TypeUnion,
			UnionMembers: []*ast.TypeExpr{
				{Kind: ast.TypeString},
				{Kind: ast.TypeNumber},
			},
		},
	}

	// string matches string | number
	if d := CheckAssignment(v, ast.Value{Kind: ast.TypeString, Str: "hi"}); d != nil {
		t.Fatalf("string should match string|number, got %v", d)
	}

	// boolean does not match
	if d := CheckAssignment(v, ast.Value{Kind: ast.TypeBoolean, Bool: true}); d == nil {
		t.Fatal("boolean should not match string|number")
	}
}

// TestCheckComparison_EqualityAnyTypes verifies that equality operators (== and !=) are allowed between values of any types without error.
func TestCheckComparison_EqualityAnyTypes(t *testing.T) {
	// == and != work on any types
	a := ast.Value{Kind: ast.TypeString, Str: "hello"}
	b := ast.Value{Kind: ast.TypeNumber, Num: 42}

	if d := CheckComparison(a, b, "=="); d != nil {
		t.Fatalf("== should work on any types, got %v", d)
	}
	if d := CheckComparison(a, b, "!="); d != nil {
		t.Fatalf("!= should work on any types, got %v", d)
	}
}

// TestCheckComparison_OrderedSameType verifies that ordering operators (>, <, >=, <=) are allowed when both operands have the same type.
func TestCheckComparison_OrderedSameType(t *testing.T) {
	a := ast.Value{Kind: ast.TypeNumber, Num: 1}
	b := ast.Value{Kind: ast.TypeNumber, Num: 2}

	for _, op := range []string{">", "<", ">=", "<="} {
		if d := CheckComparison(a, b, op); d != nil {
			t.Fatalf("operator %s on numbers should work, got %v", op, d)
		}
	}
}

// TestCheckComparison_OrderedDifferentTypes verifies that ordering operators between different types (e.g., string > number) produce an E031 diagnostic.
func TestCheckComparison_OrderedDifferentTypes(t *testing.T) {
	a := ast.Value{Kind: ast.TypeString, Str: "a"}
	b := ast.Value{Kind: ast.TypeNumber, Num: 1}

	d := CheckComparison(a, b, ">")
	if d == nil {
		t.Fatal("expected E031 for comparing string > number")
	}
	if d.Code != "E031" {
		t.Fatalf("expected E031, got %s", d.Code)
	}
}

// TestCheckComparison_OrderedOnBoolean verifies that ordering operators on boolean values produce an E032 diagnostic, since booleans are not orderable.
func TestCheckComparison_OrderedOnBoolean(t *testing.T) {
	a := ast.Value{Kind: ast.TypeBoolean, Bool: true}
	b := ast.Value{Kind: ast.TypeBoolean, Bool: false}

	d := CheckComparison(a, b, ">")
	if d == nil {
		t.Fatal("expected E032 for > on booleans")
	}
	if d.Code != "E032" {
		t.Fatalf("expected E032, got %s", d.Code)
	}
}

// TestCheckIterable_Array verifies that array values pass the iterability check without error.
func TestCheckIterable_Array(t *testing.T) {
	val := ast.Value{Kind: ast.TypeArray, Array: []ast.Value{{Kind: ast.TypeString, Str: "a"}}}
	if d := CheckIterable(val); d != nil {
		t.Fatalf("array should be iterable, got %v", d)
	}
}

// TestCheckIterable_NonArray verifies that non-array types (string, number, object, boolean, null) produce an E033 diagnostic when checked for iterability.
func TestCheckIterable_NonArray(t *testing.T) {
	for _, val := range []ast.Value{
		{Kind: ast.TypeString, Str: "not array"},
		{Kind: ast.TypeNumber, Num: 42},
		{Kind: ast.TypeObject, Object: map[string]ast.Value{}},
		{Kind: ast.TypeBoolean, Bool: true},
		{Kind: ast.TypeNull, IsNull: true},
	} {
		d := CheckIterable(val)
		if d == nil {
			t.Fatalf("expected E033 for non-array type %v", val.Kind)
		}
		if d.Code != "E033" {
			t.Fatalf("expected E033, got %s", d.Code)
		}
	}
}

// TestCompareValues_Equality verifies that CompareValues correctly evaluates == and != for same-type and cross-type value pairs.
func TestCompareValues_Equality(t *testing.T) {
	tests := []struct {
		a, b   ast.Value
		op     string
		expect bool
	}{
		{ast.Value{Kind: ast.TypeString, Str: "a"}, ast.Value{Kind: ast.TypeString, Str: "a"}, "==", true},
		{ast.Value{Kind: ast.TypeString, Str: "a"}, ast.Value{Kind: ast.TypeString, Str: "b"}, "==", false},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 1}, "==", true},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 2}, "!=", true},
		{ast.Value{Kind: ast.TypeBoolean, Bool: true}, ast.Value{Kind: ast.TypeBoolean, Bool: true}, "==", true},
		{ast.Value{Kind: ast.TypeBoolean, Bool: true}, ast.Value{Kind: ast.TypeBoolean, Bool: false}, "==", false},
		{ast.Value{Kind: ast.TypeNull, IsNull: true}, ast.Value{Kind: ast.TypeNull, IsNull: true}, "==", true},
		// different types are never equal
		{ast.Value{Kind: ast.TypeString, Str: "1"}, ast.Value{Kind: ast.TypeNumber, Num: 1}, "==", false},
	}

	for _, tt := range tests {
		result, err := CompareValues(tt.a, tt.b, tt.op)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != tt.expect {
			t.Errorf("CompareValues(%v %s %v) = %v, want %v", tt.a, tt.op, tt.b, result, tt.expect)
		}
	}
}

// TestCompareValues_Ordered verifies that CompareValues correctly evaluates ordering operators (>, <, >=, <=) for numbers and strings.
func TestCompareValues_Ordered(t *testing.T) {
	tests := []struct {
		a, b   ast.Value
		op     string
		expect bool
	}{
		{ast.Value{Kind: ast.TypeNumber, Num: 2}, ast.Value{Kind: ast.TypeNumber, Num: 1}, ">", true},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 2}, "<", true},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 1}, ">=", true},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 1}, "<=", true},
		{ast.Value{Kind: ast.TypeNumber, Num: 1}, ast.Value{Kind: ast.TypeNumber, Num: 2}, ">", false},
		{ast.Value{Kind: ast.TypeString, Str: "b"}, ast.Value{Kind: ast.TypeString, Str: "a"}, ">", true},
		{ast.Value{Kind: ast.TypeString, Str: "a"}, ast.Value{Kind: ast.TypeString, Str: "b"}, "<", true},
	}

	for _, tt := range tests {
		result, err := CompareValues(tt.a, tt.b, tt.op)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != tt.expect {
			t.Errorf("CompareValues(%v %s %v) = %v, want %v", tt.a, tt.op, tt.b, result, tt.expect)
		}
	}
}

// TestCompareValues_UnknownOperator verifies that CompareValues returns an error when given an unrecognized operator.
func TestCompareValues_UnknownOperator(t *testing.T) {
	a := ast.Value{Kind: ast.TypeNumber, Num: 1}
	_, err := CompareValues(a, a, "??")
	if err == nil {
		t.Fatal("expected error for unknown operator")
	}
}

// TestTruthyValue verifies the truthiness rules: false/empty-string/zero/null/empty-collections are falsy, all others are truthy.
func TestTruthyValue(t *testing.T) {
	tests := []struct {
		val    ast.Value
		expect bool
	}{
		{ast.Value{Kind: ast.TypeBoolean, Bool: true}, true},
		{ast.Value{Kind: ast.TypeBoolean, Bool: false}, false},
		{ast.Value{Kind: ast.TypeString, Str: "hello"}, true},
		{ast.Value{Kind: ast.TypeString, Str: ""}, false},
		{ast.Value{Kind: ast.TypeNumber, Num: 42}, true},
		{ast.Value{Kind: ast.TypeNumber, Num: 0}, false},
		{ast.Value{Kind: ast.TypeNull, IsNull: true}, false},
		{ast.Value{Kind: ast.TypeArray, Array: []ast.Value{{Kind: ast.TypeNumber, Num: 1}}}, true},
		{ast.Value{Kind: ast.TypeArray, Array: []ast.Value{}}, false},
		{ast.Value{Kind: ast.TypeArray}, false}, // nil array
		{ast.Value{Kind: ast.TypeObject, Object: map[string]ast.Value{"k": {}}}, true},
		{ast.Value{Kind: ast.TypeObject, Object: map[string]ast.Value{}}, false},
	}

	for _, tt := range tests {
		result := TruthyValue(tt.val)
		if result != tt.expect {
			t.Errorf("TruthyValue(%v) = %v, want %v", tt.val, result, tt.expect)
		}
	}
}
