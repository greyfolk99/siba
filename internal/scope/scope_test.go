// Package scope tests verify the scope tree construction, variable resolution,
// and declaration rules for siba's heading-based lexical scoping system.
package scope

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
)

// TestBuildScopeTree_RootVariable verifies that a variable declared before any heading is placed in the root scope and is resolvable from child scopes.
func TestBuildScopeTree_RootVariable(t *testing.T) {
	// Variable declared before any heading → root scope, visible everywhere
	doc := &ast.Document{
		Source: "line1\nline2\n# A\nline4\nline5",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 3},
				Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 5}},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "title",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeString, Str: "Hello"},
				Position:   ast.Position{Line: 1},
			},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// root scope should have the variable
	if _, ok := root.Vars["title"]; !ok {
		t.Fatal("expected 'title' in root scope")
	}

	// should be resolvable from heading A's scope
	scopeA := FindScopeForLine(root, 4)
	if v, ok := scopeA.Resolve("title"); !ok || v.Value.Str != "Hello" {
		t.Fatal("expected 'title' resolvable from heading A scope")
	}
}

// TestBuildScopeTree_HeadingScopeIsolation verifies that variables in sibling heading scopes are isolated and not visible to each other.
func TestBuildScopeTree_HeadingScopeIsolation(t *testing.T) {
	// Variable in Section A should NOT be visible in Section B
	doc := &ast.Document{
		Source: "# A\nvar_a\n# B\nvar_b\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 2}},
			},
			{
				Level: 1, Text: "B", Slug: "b",
				Position: ast.Position{Line: 3},
				Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "only_a",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeString, Str: "A val"},
				Position:   ast.Position{Line: 2},
			},
			{
				Name:       "only_b",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeString, Str: "B val"},
				Position:   ast.Position{Line: 4},
			},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	scopeA := FindScopeForLine(root, 2)
	scopeB := FindScopeForLine(root, 4)

	// A can see only_a
	if _, ok := scopeA.Resolve("only_a"); !ok {
		t.Fatal("expected 'only_a' visible in scope A")
	}
	// A cannot see only_b
	if _, ok := scopeA.Resolve("only_b"); ok {
		t.Fatal("expected 'only_b' NOT visible in scope A")
	}
	// B can see only_b
	if _, ok := scopeB.Resolve("only_b"); !ok {
		t.Fatal("expected 'only_b' visible in scope B")
	}
	// B cannot see only_a
	if _, ok := scopeB.Resolve("only_a"); ok {
		t.Fatal("expected 'only_a' NOT visible in scope B")
	}
}

// TestBuildScopeTree_ParentVariableVisibleInChild verifies that a variable in a parent heading scope is resolvable from a nested child scope via the scope chain.
func TestBuildScopeTree_ParentVariableVisibleInChild(t *testing.T) {
	// H1 variable visible in H2 child
	doc := &ast.Document{
		Source: "# A\nvar\n## A1\nuse var here\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "parent_var",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeString, Str: "from parent"},
				Position:   ast.Position{Line: 2},
			},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// parent_var is in scope A
	scopeA := FindScopeForLine(root, 2)
	if _, ok := scopeA.Resolve("parent_var"); !ok {
		t.Fatal("expected 'parent_var' in scope A")
	}

	// parent_var should be resolvable from child A1 via scope chain
	scopeA1 := FindScopeForLine(root, 4)
	if v, ok := scopeA1.Resolve("parent_var"); !ok || v.Value.Str != "from parent" {
		t.Fatal("expected 'parent_var' resolvable from child scope A1")
	}
}

// TestBuildScopeTree_LetShadowing verifies that a @let variable in a child scope correctly shadows a @let variable with the same name in the parent scope.
func TestBuildScopeTree_LetShadowing(t *testing.T) {
	// @let in child scope shadows parent's @let
	doc := &ast.Document{
		Source: "# A\nlet x=1\n## A1\nlet x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "x",
				Mutability: ast.MutLet,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"},
				Position:   ast.Position{Line: 2},
			},
			{
				Name:       "x",
				Mutability: ast.MutLet,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"},
				Position:   ast.Position{Line: 4},
			},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// In scope A (line 2), x=1
	scopeA := FindScopeForLine(root, 2)
	if v, ok := scopeA.Resolve("x"); !ok || v.Value.Num != 1 {
		t.Fatalf("expected x=1 in scope A, got %v", v)
	}

	// In scope A1 (line 4), x=2 (shadows parent)
	scopeA1 := FindScopeForLine(root, 4)
	if v, ok := scopeA1.Resolve("x"); !ok || v.Value.Num != 2 {
		t.Fatalf("expected x=2 in scope A1, got %v", v)
	}
}

// TestBuildScopeTree_ConstCannotBeShadowed verifies that redeclaring a @const variable in a child scope produces an E021 diagnostic and the child resolves to the parent's value.
func TestBuildScopeTree_ConstCannotBeShadowed(t *testing.T) {
	// @const in parent should prevent @const redeclaration in child
	doc := &ast.Document{
		Source: "# A\nconst x=1\n## A1\nconst x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "x",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"},
				Position:   ast.Position{Line: 2},
			},
			{
				Name:       "x",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"},
				Position:   ast.Position{Line: 4},
			},
		},
	}

	root, diags := BuildScopeTree(doc)

	// Should have exactly 1 diagnostic (E021: cannot shadow const)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "E021" {
		t.Fatalf("expected E021 diagnostic, got %s", diags[0].Code)
	}

	// x in scope A should be 1
	scopeA := FindScopeForLine(root, 2)
	if v, ok := scopeA.Resolve("x"); !ok || v.Value.Num != 1 {
		t.Fatal("expected x=1 in scope A")
	}

	// x in scope A1 should still resolve to parent's 1 (child's const was rejected)
	scopeA1 := FindScopeForLine(root, 4)
	if v, ok := scopeA1.Resolve("x"); !ok || v.Value.Num != 1 {
		t.Fatalf("expected x=1 in scope A1 (const shadowing rejected), got %v", v)
	}
}

// TestBuildScopeTree_DuplicateDeclaration verifies that declaring two variables with the same name in the same scope produces an E020 diagnostic.
func TestBuildScopeTree_DuplicateDeclaration(t *testing.T) {
	// Two variables with same name in same scope → E020
	doc := &ast.Document{
		Source: "# A\nconst x=1\nconst x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 3}},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "x",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"},
				Position:   ast.Position{Line: 2},
			},
			{
				Name:       "x",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"},
				Position:   ast.Position{Line: 3},
			},
		},
	}

	_, diags := BuildScopeTree(doc)

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "E020" {
		t.Fatalf("expected E020 diagnostic, got %s", diags[0].Code)
	}
}

// TestBuildScopeTree_EmptyDocument verifies that a document with no headings and no variables produces a valid root scope with no children.
func TestBuildScopeTree_EmptyDocument(t *testing.T) {
	// No headings, no variables
	doc := &ast.Document{
		Source:    "just some text\nno headings here\n",
		Headings:  nil,
		Variables: nil,
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if root.Name != "__root__" {
		t.Fatalf("expected root scope, got %q", root.Name)
	}
	if root.StartLine != 1 {
		t.Fatalf("expected StartLine=1, got %d", root.StartLine)
	}
	if len(root.Children) != 0 {
		t.Fatalf("expected no children, got %d", len(root.Children))
	}

	// Any line resolves to root
	s := FindScopeForLine(root, 1)
	if s != root {
		t.Fatal("expected root scope for line 1")
	}
}

// TestBuildScopeTree_VariableOnHeadingLine verifies that a variable declared on the same line as a heading is placed in that heading's scope.
func TestBuildScopeTree_VariableOnHeadingLine(t *testing.T) {
	// Variable on the same line as a heading → belongs to that heading's scope
	doc := &ast.Document{
		Source: "# A\n## B\ntext\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 3}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "B", Slug: "b",
						Position: ast.Position{Line: 2},
						Content:  ast.Range{Start: ast.Position{Line: 3}, End: ast.Position{Line: 3}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "on_heading",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeString, Str: "val"},
				Position:   ast.Position{Line: 2}, // same line as ## B heading
			},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// The variable on line 2 (heading B's line) should land in B's scope
	// since B.StartLine = h.Position.Line = 2
	scopeB := FindScopeForLine(root, 2)
	if scopeB.Name != "b" {
		t.Fatalf("expected scope 'b' for line 2, got %q", scopeB.Name)
	}
	if _, ok := scopeB.Vars["on_heading"]; !ok {
		t.Fatal("expected 'on_heading' in scope B")
	}
}

// TestBuildScopeTree_LetShadowsConst verifies that a @let in a child scope CAN shadow a @const in the parent (no error).
func TestBuildScopeTree_LetShadowsConst(t *testing.T) {
	// @let in child CAN shadow @const in parent — @let is the shadowable keyword
	doc := &ast.Document{
		Source: "# A\nconst x=1\n## A1\nlet x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{
				Name:       "x",
				Mutability: ast.MutConst,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"},
				Position:   ast.Position{Line: 2},
			},
			{
				Name:       "x",
				Mutability: ast.MutLet,
				Value:      &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"},
				Position:   ast.Position{Line: 4},
			},
		},
	}

	root, diags := BuildScopeTree(doc)

	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}

	// x in child should resolve to @let x=2 (shadowed)
	scopeA1 := FindScopeForLine(root, 4)
	if v, ok := scopeA1.Resolve("x"); !ok || v.Value.Num != 2 {
		t.Fatalf("expected x=2 from child @let, got %v", v)
	}
}

// TestBuildScopeTree_DeeplyNested verifies that in a 4-level deep heading hierarchy, the deepest scope can resolve all ancestor variables while shallower scopes cannot see descendants.
func TestBuildScopeTree_DeeplyNested(t *testing.T) {
	// H1 > H2 > H3 > H4 — variable at each level, deepest resolves all ancestors
	doc := &ast.Document{
		Source: "# L1\nv1\n## L2\nv2\n### L3\nv3\n#### L4\nv4\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "L1", Slug: "l1",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 8}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "L2", Slug: "l2",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 8}},
						Children: []*ast.Heading{
							{
								Level: 3, Text: "L3", Slug: "l3",
								Position: ast.Position{Line: 5},
								Content:  ast.Range{Start: ast.Position{Line: 6}, End: ast.Position{Line: 8}},
								Children: []*ast.Heading{
									{
										Level: 4, Text: "L4", Slug: "l4",
										Position: ast.Position{Line: 7},
										Content:  ast.Range{Start: ast.Position{Line: 8}, End: ast.Position{Line: 8}},
									},
								},
							},
						},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{Name: "a", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "L1"}, Position: ast.Position{Line: 2}},
			{Name: "b", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "L2"}, Position: ast.Position{Line: 4}},
			{Name: "c", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "L3"}, Position: ast.Position{Line: 6}},
			{Name: "d", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "L4"}, Position: ast.Position{Line: 8}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// deepest scope (L4, line 8) should resolve all ancestor variables
	deepest := FindScopeForLine(root, 8)
	for _, name := range []string{"a", "b", "c", "d"} {
		if _, ok := deepest.Resolve(name); !ok {
			t.Errorf("expected variable %q resolvable from deepest scope", name)
		}
	}

	// L2 scope (line 4) should see a, b but NOT c, d
	l2 := FindScopeForLine(root, 4)
	if _, ok := l2.Resolve("a"); !ok {
		t.Error("expected 'a' visible from L2")
	}
	if _, ok := l2.Resolve("b"); !ok {
		t.Error("expected 'b' visible from L2")
	}
	if _, ok := l2.Resolve("c"); ok {
		t.Error("expected 'c' NOT visible from L2")
	}
	if _, ok := l2.Resolve("d"); ok {
		t.Error("expected 'd' NOT visible from L2")
	}
}

// TestBuildScopeTree_SiblingsWithSameVarName verifies that two sibling headings can each declare a variable with the same name without conflict, as they are in independent scopes.
func TestBuildScopeTree_SiblingsWithSameVarName(t *testing.T) {
	// Two sibling headings each declare variable "x" — independent, no conflict
	doc := &ast.Document{
		Source: "# A\nx=1\n# B\nx=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 2}},
			},
			{
				Level: 1, Text: "B", Slug: "b",
				Position: ast.Position{Line: 3},
				Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 4}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	scopeA := FindScopeForLine(root, 2)
	scopeB := FindScopeForLine(root, 4)

	vA, _ := scopeA.Resolve("x")
	vB, _ := scopeB.Resolve("x")

	if vA.Value.Num != 1 {
		t.Errorf("expected x=1 in A, got %v", vA.Value.Num)
	}
	if vB.Value.Num != 2 {
		t.Errorf("expected x=2 in B, got %v", vB.Value.Num)
	}
}

// TestBuildScopeTree_OnlyRootVariables verifies that when a document has variables but no headings, all variables are placed in the root scope and resolvable from any line.
func TestBuildScopeTree_OnlyRootVariables(t *testing.T) {
	// Document with variables but no headings — all in root
	doc := &ast.Document{
		Source: "const a=1\nconst b=2\nconst c=3\n",
		Variables: []ast.Variable{
			{Name: "a", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 1}},
			{Name: "b", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 2}},
			{Name: "c", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 3, Raw: "3"}, Position: ast.Position{Line: 3}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if len(root.Vars) != 3 {
		t.Fatalf("expected 3 vars in root, got %d", len(root.Vars))
	}

	// All resolvable from any line
	for _, line := range []int{1, 2, 3} {
		s := FindScopeForLine(root, line)
		for _, name := range []string{"a", "b", "c"} {
			if _, ok := s.Resolve(name); !ok {
				t.Errorf("line %d: expected %q resolvable", line, name)
			}
		}
	}
}

// TestBuildScopeTree_AdjacentHeadingsNoContent verifies correct scope assignment when adjacent headings have no content between them, including empty content ranges.
func TestBuildScopeTree_AdjacentHeadingsNoContent(t *testing.T) {
	// Adjacent headings with no content between them
	doc := &ast.Document{
		Source: "# A\n## A1\n## A2\ncontent\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 2},
						Content:  ast.Range{Start: ast.Position{Line: 3}, End: ast.Position{Line: 2}}, // empty: end < start
					},
					{
						Level: 2, Text: "A2", Slug: "a2",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{Name: "v", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "val"}, Position: ast.Position{Line: 4}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// Variable on line 4 should be in A2's scope
	s := FindScopeForLine(root, 4)
	if s.Name != "a2" {
		t.Fatalf("expected scope 'a2' for line 4, got %q", s.Name)
	}
	if _, ok := s.Vars["v"]; !ok {
		t.Fatal("expected 'v' in A2 scope")
	}
}

// TestBuildScopeTree_VariableAtLastLine verifies that a variable declared on the very last line of a document is correctly placed and resolvable.
func TestBuildScopeTree_VariableAtLastLine(t *testing.T) {
	// Variable at the very last line of document
	doc := &ast.Document{
		Source: "# A\ntext\nconst x=42",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 3}},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 42, Raw: "42"}, Position: ast.Position{Line: 3}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	s := FindScopeForLine(root, 3)
	if s.Name != "a" {
		t.Fatalf("expected scope 'a' for last line, got %q", s.Name)
	}
	if _, ok := s.Resolve("x"); !ok {
		t.Fatal("expected 'x' resolvable at last line")
	}
}

// TestBuildScopeTree_EmptySource verifies that a completely empty source string produces a valid root scope with StartLine and EndLine both set to 1.
func TestBuildScopeTree_EmptySource(t *testing.T) {
	// Completely empty source
	doc := &ast.Document{
		Source: "",
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if root.StartLine != 1 {
		t.Fatalf("expected StartLine=1, got %d", root.StartLine)
	}
	// strings.Split("", "\n") returns [""], len=1
	if root.EndLine != 1 {
		t.Fatalf("expected EndLine=1, got %d", root.EndLine)
	}
}

// TestBuildScopeTree_MultipleVarsSameScope verifies that multiple distinct variables declared in the same heading scope are all stored and accessible.
func TestBuildScopeTree_MultipleVarsSameScope(t *testing.T) {
	// Multiple different variables in the same heading scope
	doc := &ast.Document{
		Source: "# A\na=1\nb=2\nc=3\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
			},
		},
		Variables: []ast.Variable{
			{Name: "a", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
			{Name: "b", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 3}},
			{Name: "c", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 3, Raw: "3"}, Position: ast.Position{Line: 4}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	s := FindScopeForLine(root, 2)
	if len(s.Vars) != 3 {
		t.Fatalf("expected 3 vars in scope A, got %d", len(s.Vars))
	}
}

// TestBuildScopeTree_UnresolvedVariable verifies that resolving a variable name that does not exist in any scope returns not-found.
func TestBuildScopeTree_UnresolvedVariable(t *testing.T) {
	// Resolve a name that doesn't exist anywhere
	doc := &ast.Document{
		Source: "# A\nconst x=1\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 2}},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
		},
	}

	root, _ := BuildScopeTree(doc)
	s := FindScopeForLine(root, 2)

	if _, ok := s.Resolve("nonexistent"); ok {
		t.Fatal("expected 'nonexistent' to NOT be resolvable")
	}
}

// TestBuildScopeTree_HeadingUsesNameOverSlug verifies that when a heading has a @name attribute, the scope uses that name instead of the slug.
func TestBuildScopeTree_HeadingUsesNameOverSlug(t *testing.T) {
	// Heading with @name should use Name, not Slug
	doc := &ast.Document{
		Source: "# My Heading\ntext\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "My Heading", Slug: "my-heading", Name: "custom_name",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 2}},
			},
		},
	}

	root, _ := BuildScopeTree(doc)

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if root.Children[0].Name != "custom_name" {
		t.Fatalf("expected scope name 'custom_name', got %q", root.Children[0].Name)
	}
}

// TestBuildScopeTree_HeadingFallsBackToSlug verifies that when a heading has no @name attribute, the scope falls back to using the slug.
func TestBuildScopeTree_HeadingFallsBackToSlug(t *testing.T) {
	// Heading without @name should use Slug
	doc := &ast.Document{
		Source: "# My Heading\ntext\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "My Heading", Slug: "my-heading",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 2}},
			},
		},
	}

	root, _ := BuildScopeTree(doc)

	if root.Children[0].Name != "my-heading" {
		t.Fatalf("expected scope name 'my-heading', got %q", root.Children[0].Name)
	}
}

// TestFindScopeForLine_DeepestMatch verifies that FindScopeForLine returns the deepest (most specific) matching scope for each line across nested and sibling scopes.
func TestFindScopeForLine_DeepestMatch(t *testing.T) {
	root := NewScope("root", ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 20

	a := NewScope("a", ScopeHeading, root)
	a.StartLine = 2
	a.EndLine = 10

	a1 := NewScope("a1", ScopeHeading, a)
	a1.StartLine = 4
	a1.EndLine = 8

	b := NewScope("b", ScopeHeading, root)
	b.StartLine = 12
	b.EndLine = 18

	tests := []struct {
		line     int
		expected string
	}{
		{1, "root"},
		{2, "a"},      // exact start boundary
		{3, "a"},
		{4, "a1"},     // exact start of nested
		{5, "a1"},
		{8, "a1"},     // exact end boundary of nested
		{9, "a"},      // back to parent after nested ends
		{10, "a"},     // exact end boundary of parent
		{11, "root"},  // gap between siblings
		{12, "b"},     // exact start of sibling
		{15, "b"},
		{18, "b"},     // exact end of sibling
		{19, "root"},
		{20, "root"},  // last line of document
	}

	for _, tt := range tests {
		s := FindScopeForLine(root, tt.line)
		if s.Name != tt.expected {
			t.Errorf("line %d: expected scope %q, got %q", tt.line, tt.expected, s.Name)
		}
	}
}

// TestFindScopeForLine_OutOfRange verifies that lines outside the document range (before line 1 or beyond EndLine) fall back to the root scope.
func TestFindScopeForLine_OutOfRange(t *testing.T) {
	// Lines outside document range should return root
	root := NewScope("root", ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 10

	child := NewScope("child", ScopeHeading, root)
	child.StartLine = 3
	child.EndLine = 7

	// line 0 is outside all scopes → root (no scope matches, falls through)
	s := FindScopeForLine(root, 0)
	if s.Name != "root" {
		t.Errorf("line 0: expected 'root', got %q", s.Name)
	}

	// line 100 is beyond EndLine → root
	s = FindScopeForLine(root, 100)
	if s.Name != "root" {
		t.Errorf("line 100: expected 'root', got %q", s.Name)
	}
}

// TestFindScopeForLine_SingleLineScope verifies that a scope where StartLine equals EndLine correctly matches only that exact line.
func TestFindScopeForLine_SingleLineScope(t *testing.T) {
	// Scope where StartLine == EndLine
	root := NewScope("root", ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 5

	single := NewScope("single", ScopeHeading, root)
	single.StartLine = 3
	single.EndLine = 3

	s := FindScopeForLine(root, 3)
	if s.Name != "single" {
		t.Errorf("expected 'single', got %q", s.Name)
	}

	s = FindScopeForLine(root, 2)
	if s.Name != "root" {
		t.Errorf("expected 'root' for line 2, got %q", s.Name)
	}

	s = FindScopeForLine(root, 4)
	if s.Name != "root" {
		t.Errorf("expected 'root' for line 4, got %q", s.Name)
	}
}

// TestDeclare_ConstInRootLetInChild verifies that declaring a @let in a child scope CAN shadow a parent @const (no error).
func TestDeclare_ConstInRootLetInChild(t *testing.T) {
	root := NewScope("root", ScopeHeading, nil)
	child := NewScope("child", ScopeHeading, root)

	d := root.Declare("x", ast.Variable{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1}})
	if d != nil {
		t.Fatalf("unexpected diagnostic for root const: %v", d)
	}

	d = child.Declare("x", ast.Variable{Name: "x", Mutability: ast.MutLet, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2}})
	if d != nil {
		t.Fatalf("@let should be able to shadow parent @const, got: %v", d)
	}

	// child resolves to @let x=2
	v, ok := child.Resolve("x")
	if !ok || v.Value.Num != 2 {
		t.Fatalf("expected x=2, got %v", v)
	}
}

// TestDeclare_LetInRootLetInChild verifies that a @let in a child scope can successfully shadow a @let in the parent scope without error.
func TestDeclare_LetInRootLetInChild(t *testing.T) {
	// Direct Declare test: root has let, child can shadow with let
	root := NewScope("root", ScopeHeading, nil)
	child := NewScope("child", ScopeHeading, root)

	d := root.Declare("x", ast.Variable{Name: "x", Mutability: ast.MutLet})
	if d != nil {
		t.Fatalf("unexpected diagnostic: %v", d)
	}

	d = child.Declare("x", ast.Variable{Name: "x", Mutability: ast.MutLet})
	if d != nil {
		t.Fatalf("unexpected diagnostic for let shadow: %v", d)
	}
}

// TestBuildScopeTree_DuplicateLetSameScope_E020 verifies that two @let declarations with the same name in the same scope produce an E020 diagnostic.
func TestBuildScopeTree_DuplicateLetSameScope_E020(t *testing.T) {
	// Two @let with same name in same scope → E020
	doc := &ast.Document{
		Source: "# A\nlet x=1\nlet x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 3}},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutLet, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
			{Name: "x", Mutability: ast.MutLet, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 3}},
		},
	}

	_, diags := BuildScopeTree(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "E020" {
		t.Fatalf("expected E020, got %s", diags[0].Code)
	}
}

// TestBuildScopeTree_ConstShadowsParentLet_E021 verifies that a @const in a child scope cannot shadow a parent's @let, producing an E021 diagnostic.
func TestBuildScopeTree_ConstShadowsParentLet_E021(t *testing.T) {
	// @const in child when parent has @let → E021 (const cannot shadow anything)
	doc := &ast.Document{
		Source: "# A\nlet x=1\n## A1\nconst x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 4}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "A1", Slug: "a1",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutLet, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 4}},
		},
	}

	_, diags := BuildScopeTree(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "E021" {
		t.Fatalf("expected E021, got %s", diags[0].Code)
	}
}

// TestBuildScopeTree_GrandchildConstShadowsGrandparent verifies that E021 is emitted when a @const in a grandchild scope attempts to shadow a @const two levels up.
func TestBuildScopeTree_GrandchildConstShadowsGrandparent(t *testing.T) {
	// @const in H3 when H1 has @const → E021 must traverse 2+ levels
	doc := &ast.Document{
		Source: "# L1\nconst x=1\n## L2\ntext\n### L3\nconst x=3\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "L1", Slug: "l1",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 6}},
				Children: []*ast.Heading{
					{
						Level: 2, Text: "L2", Slug: "l2",
						Position: ast.Position{Line: 3},
						Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 6}},
						Children: []*ast.Heading{
							{
								Level: 3, Text: "L3", Slug: "l3",
								Position: ast.Position{Line: 5},
								Content:  ast.Range{Start: ast.Position{Line: 6}, End: ast.Position{Line: 6}},
							},
						},
					},
				},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2}},
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 3, Raw: "3"}, Position: ast.Position{Line: 6}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "E021" {
		t.Fatalf("expected E021, got %s", diags[0].Code)
	}

	// Grandchild should still resolve to grandparent's const
	l3 := FindScopeForLine(root, 6)
	if v, ok := l3.Resolve("x"); !ok || v.Value.Num != 1 {
		t.Fatalf("expected x=1 from grandparent, got %v", v)
	}
}

// TestBuildScopeTree_DiagnosticHasCorrectRange verifies that a diagnostic's Range field points to the position of the offending (duplicate) variable declaration.
func TestBuildScopeTree_DiagnosticHasCorrectRange(t *testing.T) {
	// Verify diagnostic Range field is populated with the variable's position
	doc := &ast.Document{
		Source: "# A\nconst x=1\nconst x=2\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 1},
				Content:  ast.Range{Start: ast.Position{Line: 2}, End: ast.Position{Line: 3}},
			},
		},
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 1, Raw: "1"}, Position: ast.Position{Line: 2, Column: 5}},
			{Name: "x", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeNumber, Num: 2, Raw: "2"}, Position: ast.Position{Line: 3, Column: 10}},
		},
	}

	_, diags := BuildScopeTree(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	// Diagnostic should point to the second variable's position (the one that failed)
	if diags[0].Range.Start.Line != 3 {
		t.Errorf("expected diagnostic at line 3, got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Range.Start.Column != 10 {
		t.Errorf("expected diagnostic at column 10, got %d", diags[0].Range.Start.Column)
	}
}

// TestBuildScopeTree_VariableInGapBeforeFirstHeading verifies that a variable declared in the preamble (before the first heading) lands in the root scope and is visible from heading scopes.
func TestBuildScopeTree_VariableInGapBeforeFirstHeading(t *testing.T) {
	// Variable on line between root start and first heading → root scope
	doc := &ast.Document{
		Source: "preamble\nvar here\n# A\ncontent\n",
		Headings: []*ast.Heading{
			{
				Level: 1, Text: "A", Slug: "a",
				Position: ast.Position{Line: 3},
				Content:  ast.Range{Start: ast.Position{Line: 4}, End: ast.Position{Line: 4}},
			},
		},
		Variables: []ast.Variable{
			{Name: "pre", Mutability: ast.MutConst, Value: &ast.Value{Kind: ast.TypeString, Str: "before"}, Position: ast.Position{Line: 2}},
		},
	}

	root, diags := BuildScopeTree(doc)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// Line 2 is before heading A (starts at line 3), should be root
	s := FindScopeForLine(root, 2)
	if s.Name != "__root__" {
		t.Fatalf("expected root scope for line 2, got %q", s.Name)
	}
	if _, ok := s.Vars["pre"]; !ok {
		t.Fatal("expected 'pre' in root scope")
	}

	// Should be visible from A's scope via parent chain
	scopeA := FindScopeForLine(root, 4)
	if _, ok := scopeA.Resolve("pre"); !ok {
		t.Fatal("expected 'pre' resolvable from heading A")
	}
}

// TestFindScopeForLine_AdjacentSiblingsSharedBoundary verifies that when two sibling scopes share a boundary line, the later sibling wins due to reverse iteration order.
func TestFindScopeForLine_AdjacentSiblingsSharedBoundary(t *testing.T) {
	// Scope A ends at line 5, scope B starts at line 5 → B wins (last child in reverse iteration)
	root := NewScope("root", ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 10

	a := NewScope("a", ScopeHeading, root)
	a.StartLine = 2
	a.EndLine = 5

	b := NewScope("b", ScopeHeading, root)
	b.StartLine = 5
	b.EndLine = 8

	// line 5 is in both A and B's range; reverse iteration picks B (last child)
	s := FindScopeForLine(root, 5)
	if s.Name != "b" {
		t.Errorf("expected 'b' for shared boundary line 5, got %q", s.Name)
	}

	// line 4 is only in A
	s = FindScopeForLine(root, 4)
	if s.Name != "a" {
		t.Errorf("expected 'a' for line 4, got %q", s.Name)
	}

	// line 6 is only in B
	s = FindScopeForLine(root, 6)
	if s.Name != "b" {
		t.Errorf("expected 'b' for line 6, got %q", s.Name)
	}
}
