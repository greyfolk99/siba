package types

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
)

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

func TestCheckAssignment_NilType(t *testing.T) {
	v := ast.Variable{Name: "x", Type: nil}
	val := ast.Value{Kind: ast.TypeString, Str: "anything"}

	if d := CheckAssignment(v, val); d != nil {
		t.Fatalf("nil type should accept anything, got %v", d)
	}
}

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

func TestCheckComparison_OrderedSameType(t *testing.T) {
	a := ast.Value{Kind: ast.TypeNumber, Num: 1}
	b := ast.Value{Kind: ast.TypeNumber, Num: 2}

	for _, op := range []string{">", "<", ">=", "<="} {
		if d := CheckComparison(a, b, op); d != nil {
			t.Fatalf("operator %s on numbers should work, got %v", op, d)
		}
	}
}

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

func TestCheckIterable_Array(t *testing.T) {
	val := ast.Value{Kind: ast.TypeArray, Array: []ast.Value{{Kind: ast.TypeString, Str: "a"}}}
	if d := CheckIterable(val); d != nil {
		t.Fatalf("array should be iterable, got %v", d)
	}
}

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

func TestCompareValues_UnknownOperator(t *testing.T) {
	a := ast.Value{Kind: ast.TypeNumber, Num: 1}
	_, err := CompareValues(a, a, "??")
	if err == nil {
		t.Fatal("expected error for unknown operator")
	}
}

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
