package control

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/types"
)

// --- Edge case tests from review ---

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

func TestProcessControlBlocks_IfDiagnosticPropagated(t *testing.T) {
	content := "before\n<!-- @if undefined_var == \"x\" -->\ncontent\n<!-- @endif -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:      ast.DirectiveIf,
			Condition: `undefined_var == "x"`,
			Start:     ast.Position{Line: 2},
			End:       ast.Position{Line: 4},
		},
	}
	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	_, diags := ProcessControlBlocks(content, blocks, root)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for undefined variable in @if")
	}
}

func TestProcessControlBlocks_ForDiagnosticPropagated(t *testing.T) {
	content := "before\n<!-- @for x in y -->\ncontent\n<!-- @endfor -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:       ast.DirectiveFor,
			Iterator:   "x",
			Collection: "y",
			Start:      ast.Position{Line: 2},
			End:        ast.Position{Line: 4},
		},
	}
	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("y", ast.Variable{
		Name:  "y",
		Value: strVal("not an array"),
	})
	_, diags := ProcessControlBlocks(content, blocks, root)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for non-array in @for")
	}
}

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

func TestProcessControlBlocks_ForPropertyAccess(t *testing.T) {
	content := "list:\n<!-- @for x in data -->\n- {{x.key}}: {{x.val}}\n<!-- @endfor -->\nend"
	blocks := []ast.ControlBlock{
		{
			Kind:       ast.DirectiveFor,
			Iterator:   "x",
			Collection: "data",
			Start:      ast.Position{Line: 2},
			End:        ast.Position{Line: 4},
		},
	}
	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("data", ast.Variable{
		Name: "data",
		Value: &ast.Value{
			Kind: ast.TypeArray,
			Array: []ast.Value{
				{Kind: ast.TypeObject, Object: map[string]ast.Value{
					"key": {Kind: ast.TypeString, Str: "a"},
					"val": {Kind: ast.TypeString, Str: "1"},
				}},
			},
		},
	})
	result, _ := ProcessControlBlocks(content, blocks, root)
	expected := "list:\n- a: 1\nend"
	if result != expected {
		t.Fatalf("unexpected result:\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestEvaluateIf_EmptyCondition(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	_, diag := EvaluateIf("", s)
	if diag == nil {
		t.Fatal("expected diagnostic for empty condition")
	}
}

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
