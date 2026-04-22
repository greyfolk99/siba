// eval_test.go tests the core control-flow evaluation logic:
// condition parsing (parseCondition), conditional evaluation (EvaluateIf),
// loop evaluation (EvaluateFor), and end-to-end control block processing
// (ProcessControlBlocks) for @if/@for directives.
package control

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
)

func makeScope(vars map[string]ast.Variable) *scope.Scope {
	s := scope.NewScope("test", scope.ScopeHeading, nil)
	for name, v := range vars {
		s.Declare(name, v)
	}
	return s
}

func strVal(s string) *ast.Value {
	return &ast.Value{Kind: ast.TypeString, Str: s, Raw: `"` + s + `"`}
}

func numVal(n float64) *ast.Value {
	return &ast.Value{Kind: ast.TypeNumber, Num: n, Raw: ""}
}

func boolVal(b bool) *ast.Value {
	v := &ast.Value{Kind: ast.TypeBoolean, Bool: b}
	if b {
		v.Raw = "true"
	} else {
		v.Raw = "false"
	}
	return v
}

// --- EvaluateIf tests ---

// TestEvaluateIf_StringEquality verifies that @if with == operator correctly matches two equal string values.
func TestEvaluateIf_StringEquality(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"env": {Name: "env", Mutability: ast.MutConst, Value: strVal("production")},
	})

	result, diag := EvaluateIf(`env == "production"`, s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_StringInequality verifies that == returns false when the variable value does not match the literal.
func TestEvaluateIf_StringInequality(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"env": {Name: "env", Mutability: ast.MutConst, Value: strVal("staging")},
	})

	result, diag := EvaluateIf(`env == "production"`, s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result {
		t.Fatal("expected false")
	}
}

// TestEvaluateIf_NumberComparison verifies all numeric comparison operators (>, <, >=, <=) against a number variable.
func TestEvaluateIf_NumberComparison(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"port": {Name: "port", Mutability: ast.MutConst, Value: numVal(8080)},
	})

	tests := []struct {
		cond   string
		expect bool
	}{
		{"port > 80", true},
		{"port < 9000", true},
		{"port >= 8080", true},
		{"port <= 8080", true},
		{"port > 9000", false},
	}

	for _, tt := range tests {
		result, diag := EvaluateIf(tt.cond, s)
		if diag != nil {
			t.Fatalf("cond %q: unexpected diagnostic: %v", tt.cond, diag)
		}
		if result != tt.expect {
			t.Errorf("cond %q: got %v, want %v", tt.cond, result, tt.expect)
		}
	}
}

// TestEvaluateIf_NotEqual verifies that the != operator returns true when the variable differs from the literal.
func TestEvaluateIf_NotEqual(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"env": {Name: "env", Mutability: ast.MutConst, Value: strVal("staging")},
	})

	result, diag := EvaluateIf(`env != "production"`, s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_TruthyCheck verifies truthiness evaluation for all value types (bool, string, number) without an operator.
func TestEvaluateIf_TruthyCheck(t *testing.T) {
	tests := []struct {
		name   string
		val    *ast.Value
		expect bool
	}{
		{"true bool", boolVal(true), true},
		{"false bool", boolVal(false), false},
		{"non-empty string", strVal("hello"), true},
		{"empty string", strVal(""), false},
		{"non-zero number", numVal(42), true},
		{"zero", numVal(0), false},
	}

	for _, tt := range tests {
		s := makeScope(map[string]ast.Variable{
			"x": {Name: "x", Mutability: ast.MutConst, Value: tt.val},
		})

		result, diag := EvaluateIf("x", s)
		if diag != nil {
			t.Fatalf("%s: unexpected diagnostic: %v", tt.name, diag)
		}
		if result != tt.expect {
			t.Errorf("%s: got %v, want %v", tt.name, result, tt.expect)
		}
	}
}

// TestEvaluateIf_UndefinedVariable verifies that referencing an undeclared variable produces diagnostic E044.
func TestEvaluateIf_UndefinedVariable(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})

	_, diag := EvaluateIf(`x == "hello"`, s)
	if diag == nil {
		t.Fatal("expected diagnostic for undefined variable")
	}
	if diag.Code != "E044" {
		t.Fatalf("expected E044, got %s", diag.Code)
	}
}

// TestEvaluateIf_PropertyAccess verifies that dot-notation property access (e.g., config.env) works in conditions.
func TestEvaluateIf_PropertyAccess(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"config": {
			Name:       "config",
			Mutability: ast.MutConst,
			Value: &ast.Value{
				Kind: ast.TypeObject,
				Object: map[string]ast.Value{
					"env": {Kind: ast.TypeString, Str: "production", Raw: `"production"`},
				},
			},
		},
	})

	result, diag := EvaluateIf(`config.env == "production"`, s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_BoolLiteral verifies that comparing a boolean variable against the literal true works correctly.
func TestEvaluateIf_BoolLiteral(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"enabled": {Name: "enabled", Mutability: ast.MutConst, Value: boolVal(true)},
	})

	result, diag := EvaluateIf("enabled == true", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_NullComparison verifies that comparing a null-typed variable against the null literal returns true.
func TestEvaluateIf_NullComparison(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"x": {Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNull, IsNull: true, Raw: "null"}},
	})

	result, diag := EvaluateIf("x == null", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// TestEvaluateIf_TwoVariables verifies that a condition comparing two scope variables (a < b) evaluates correctly.
func TestEvaluateIf_TwoVariables(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"a": {Name: "a", Mutability: ast.MutConst, Value: numVal(10)},
		"b": {Name: "b", Mutability: ast.MutConst, Value: numVal(20)},
	})

	result, diag := EvaluateIf("a < b", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if !result {
		t.Fatal("expected true")
	}
}

// --- EvaluateFor tests ---

// TestEvaluateFor_StringArray verifies that iterating over a string array produces one scope per element with the correct value.
func TestEvaluateFor_StringArray(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"items": {
			Name:       "items",
			Mutability: ast.MutConst,
			Value: &ast.Value{
				Kind: ast.TypeArray,
				Array: []ast.Value{
					{Kind: ast.TypeString, Str: "a", Raw: `"a"`},
					{Kind: ast.TypeString, Str: "b", Raw: `"b"`},
					{Kind: ast.TypeString, Str: "c", Raw: `"c"`},
				},
			},
		},
	})

	iters, diag := EvaluateFor("item", "items", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if len(iters) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(iters))
	}

	// each iteration should have the iterator variable in scope
	for i, iter := range iters {
		v, ok := iter.Scope.Resolve("item")
		if !ok {
			t.Fatalf("iteration %d: expected 'item' in scope", i)
		}
		expected := []string{"a", "b", "c"}
		if v.Value.Str != expected[i] {
			t.Errorf("iteration %d: expected %q, got %q", i, expected[i], v.Value.Str)
		}
	}
}

// TestEvaluateFor_ObjectArray verifies that iterating over an array of objects exposes object properties on the iterator variable.
func TestEvaluateFor_ObjectArray(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"endpoints": {
			Name:       "endpoints",
			Mutability: ast.MutConst,
			Value: &ast.Value{
				Kind: ast.TypeArray,
				Array: []ast.Value{
					{
						Kind: ast.TypeObject,
						Object: map[string]ast.Value{
							"name": {Kind: ast.TypeString, Str: "login"},
							"path": {Kind: ast.TypeString, Str: "/login"},
						},
					},
					{
						Kind: ast.TypeObject,
						Object: map[string]ast.Value{
							"name": {Kind: ast.TypeString, Str: "logout"},
							"path": {Kind: ast.TypeString, Str: "/logout"},
						},
					},
				},
			},
		},
	})

	iters, diag := EvaluateFor("ep", "endpoints", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if len(iters) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(iters))
	}

	// check property access in each iteration
	v, _ := iters[0].Scope.Resolve("ep")
	if v.Value.Object["name"].Str != "login" {
		t.Errorf("expected name 'login', got %q", v.Value.Object["name"].Str)
	}
}

// TestEvaluateFor_UndefinedCollection verifies that referencing an undeclared collection produces diagnostic E041.
func TestEvaluateFor_UndefinedCollection(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})

	_, diag := EvaluateFor("item", "nonexistent", s)
	if diag == nil {
		t.Fatal("expected diagnostic for undefined collection")
	}
	if diag.Code != "E041" {
		t.Fatalf("expected E041, got %s", diag.Code)
	}
}

// TestEvaluateFor_NonArrayCollection verifies that using a non-array value as a @for collection produces diagnostic E033.
func TestEvaluateFor_NonArrayCollection(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"x": {Name: "x", Mutability: ast.MutConst, Value: strVal("not an array")},
	})

	_, diag := EvaluateFor("item", "x", s)
	if diag == nil {
		t.Fatal("expected diagnostic for non-array collection")
	}
	if diag.Code != "E033" {
		t.Fatalf("expected E033, got %s", diag.Code)
	}
}

// TestEvaluateFor_EmptyArray verifies that iterating over an empty array yields zero iterations and no error.
func TestEvaluateFor_EmptyArray(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"items": {
			Name:       "items",
			Mutability: ast.MutConst,
			Value:      &ast.Value{Kind: ast.TypeArray, Array: []ast.Value{}},
		},
	})

	iters, diag := EvaluateFor("item", "items", s)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if len(iters) != 0 {
		t.Fatalf("expected 0 iterations, got %d", len(iters))
	}
}

// TestEvaluateFor_ParentScopeVisible verifies that variables from the parent scope remain accessible inside @for iteration scopes.
func TestEvaluateFor_ParentScopeVisible(t *testing.T) {
	parent := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Mutability: ast.MutConst, Value: strVal("My Doc")},
		"items": {
			Name:       "items",
			Mutability: ast.MutConst,
			Value:      &ast.Value{Kind: ast.TypeArray, Array: []ast.Value{{Kind: ast.TypeString, Str: "a"}}},
		},
	})

	iters, _ := EvaluateFor("item", "items", parent)
	if len(iters) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(iters))
	}

	// parent variable should be visible from iteration scope
	v, ok := iters[0].Scope.Resolve("title")
	if !ok || v.Value.Str != "My Doc" {
		t.Fatal("expected parent variable 'title' visible in for scope")
	}
}

// TestEvaluateFor_NilValue verifies that a collection variable with a nil value produces diagnostic E042.
func TestEvaluateFor_NilValue(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"items": {Name: "items", Mutability: ast.MutConst, Value: nil},
	})

	_, diag := EvaluateFor("item", "items", s)
	if diag == nil {
		t.Fatal("expected diagnostic for nil value")
	}
	if diag.Code != "E042" {
		t.Fatalf("expected E042, got %s", diag.Code)
	}
}

// --- parseCondition tests ---

// TestParseCondition verifies that parseCondition correctly splits conditions into left operand, operator, and right operand across all supported operators and whitespace variants.
func TestParseCondition(t *testing.T) {
	tests := []struct {
		input         string
		left, op, right string
	}{
		{`env == "production"`, "env", "==", `"production"`},
		{`env != "staging"`, "env", "!=", `"staging"`},
		{"port > 80", "port", ">", "80"},
		{"port < 9000", "port", "<", "9000"},
		{"count >= 10", "count", ">=", "10"},
		{"count <= 100", "count", "<=", "100"},
		{"enabled", "enabled", "", ""},          // truthy check
		{"  x  ==  y  ", "x", "==", "y"},        // whitespace handling
	}

	for _, tt := range tests {
		left, op, right := parseCondition(tt.input)
		if left != tt.left || op != tt.op || right != tt.right {
			t.Errorf("parseCondition(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, left, op, right, tt.left, tt.op, tt.right)
		}
	}
}

// --- ProcessControlBlocks tests ---

// TestProcessControlBlocks_IfTrue verifies that @if blocks with a true condition include their body content in the output.
func TestProcessControlBlocks_IfTrue(t *testing.T) {
	content := "before\n<!-- @if env == \"production\" -->\nproduction content\n<!-- @endif -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:      ast.DirectiveIf,
			Condition: `env == "production"`,
			Start:     ast.Position{Line: 2},
			End:       ast.Position{Line: 4},
		},
	}

	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("env", ast.Variable{
		Name:  "env",
		Value: strVal("production"),
	})

	result, _ := ProcessControlBlocks(content, blocks, root)
	if result != "before\nproduction content\nafter" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

// TestProcessControlBlocks_IfFalse verifies that @if blocks with a false condition strip their body content from the output.
func TestProcessControlBlocks_IfFalse(t *testing.T) {
	content := "before\n<!-- @if env == \"production\" -->\nproduction content\n<!-- @endif -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:      ast.DirectiveIf,
			Condition: `env == "production"`,
			Start:     ast.Position{Line: 2},
			End:       ast.Position{Line: 4},
		},
	}

	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("env", ast.Variable{
		Name:  "env",
		Value: strVal("staging"),
	})

	result, _ := ProcessControlBlocks(content, blocks, root)
	if result != "before\nafter" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

// TestProcessControlBlocks_ForLoop verifies that @for expands the body template once per array element with variable substitution.
func TestProcessControlBlocks_ForLoop(t *testing.T) {
	content := "before\n<!-- @for item in items -->\n- {{item}}\n<!-- @endfor -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:       ast.DirectiveFor,
			Iterator:   "item",
			Collection: "items",
			Start:      ast.Position{Line: 2},
			End:        ast.Position{Line: 4},
		},
	}

	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("items", ast.Variable{
		Name: "items",
		Value: &ast.Value{
			Kind: ast.TypeArray,
			Array: []ast.Value{
				{Kind: ast.TypeString, Str: "alpha", Raw: `"alpha"`},
				{Kind: ast.TypeString, Str: "beta", Raw: `"beta"`},
			},
		},
	})

	result, _ := ProcessControlBlocks(content, blocks, root)
	expected := "before\n- alpha\n- beta\nafter"
	if result != expected {
		t.Fatalf("unexpected result:\ngot:  %q\nwant: %q", result, expected)
	}
}

// TestProcessControlBlocks_ForObjectArray verifies that @for over an object array correctly substitutes dot-notation property references in the body.
func TestProcessControlBlocks_ForObjectArray(t *testing.T) {
	content := "before\n<!-- @for ep in endpoints -->\n### {{ep.name}}\npath: {{ep.path}}\n<!-- @endfor -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:       ast.DirectiveFor,
			Iterator:   "ep",
			Collection: "endpoints",
			Start:      ast.Position{Line: 2},
			End:        ast.Position{Line: 5},
		},
	}

	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 6
	root.Declare("endpoints", ast.Variable{
		Name: "endpoints",
		Value: &ast.Value{
			Kind: ast.TypeArray,
			Array: []ast.Value{
				{Kind: ast.TypeObject, Object: map[string]ast.Value{
					"name": {Kind: ast.TypeString, Str: "login"},
					"path": {Kind: ast.TypeString, Str: "/login"},
				}},
				{Kind: ast.TypeObject, Object: map[string]ast.Value{
					"name": {Kind: ast.TypeString, Str: "logout"},
					"path": {Kind: ast.TypeString, Str: "/logout"},
				}},
			},
		},
	})

	result, _ := ProcessControlBlocks(content, blocks, root)
	expected := "before\n### login\npath: /login\n### logout\npath: /logout\nafter"
	if result != expected {
		t.Fatalf("unexpected result:\ngot:  %q\nwant: %q", result, expected)
	}
}

// TestProcessControlBlocks_NoBlocks verifies that content is returned unchanged when no control blocks are present.
func TestProcessControlBlocks_NoBlocks(t *testing.T) {
	content := "hello\nworld"
	result, _ := ProcessControlBlocks(content, nil, nil)
	if result != content {
		t.Fatalf("expected unchanged content, got %q", result)
	}
}

// TestProcessControlBlocks_EmptyForLoop verifies that @for over an empty array removes the block entirely, producing no body output.
func TestProcessControlBlocks_EmptyForLoop(t *testing.T) {
	content := "before\n<!-- @for item in items -->\n- {{item}}\n<!-- @endfor -->\nafter"
	blocks := []ast.ControlBlock{
		{
			Kind:       ast.DirectiveFor,
			Iterator:   "item",
			Collection: "items",
			Start:      ast.Position{Line: 2},
			End:        ast.Position{Line: 4},
		},
	}

	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5
	root.Declare("items", ast.Variable{
		Name:  "items",
		Value: &ast.Value{Kind: ast.TypeArray, Array: []ast.Value{}},
	})

	result, _ := ProcessControlBlocks(content, blocks, root)
	if result != "before\nafter" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}
