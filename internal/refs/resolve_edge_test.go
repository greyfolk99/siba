// Package refs edge-case tests exercise boundary conditions and uncommon
// code paths in reference resolution, cycle detection, and dependency graphs.
package refs

import (
	"strings"
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/workspace"
)

// --- Edge case tests from Codex review ---

// TestResolveReference_LocalVariableNilValue verifies that a local variable with nil Value produces E050.
func TestResolveReference_LocalVariableNilValue(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Value: nil},
	})
	ref := makeRef("title", "title", "", "", 1)
	doc := makeDoc("", "test.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic for nil value")
	}
	if diag.Code != "E050" {
		t.Fatalf("expected E050, got %s", diag.Code)
	}
}

// TestResolveReference_DocVariableNilValue verifies that a public doc variable with nil Value produces E054.
func TestResolveReference_DocVariableNilValue(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "port", Access: ast.AccessPublic, Value: nil},
	}
	targetDoc := makeDocWithVars("config", "config.md", targetVars)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.port", "config", "", "port", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic for nil value")
	}
	if diag.Code != "E054" {
		t.Fatalf("expected E054, got %s", diag.Code)
	}
}

// TestResolveReference_NilCurrentDocSectionRef verifies that a section reference with nil currentDoc produces E050.
func TestResolveReference_NilCurrentDocSectionRef(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#intro", "", "intro", "", 1)

	_, diag := ResolveReference(ref, nil, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic for nil currentDoc")
	}
	if diag.Code != "E050" {
		t.Fatalf("expected E050, got %s", diag.Code)
	}
}

// TestResolveReference_LocalObjectPropertyNoWorkspace verifies that local object property access works without a workspace.
func TestResolveReference_LocalObjectPropertyNoWorkspace(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"settings": {
			Name: "settings",
			Value: &ast.Value{
				Kind: ast.TypeObject,
				Object: map[string]ast.Value{
					"theme": {Kind: ast.TypeString, Str: "dark"},
				},
			},
		},
	})
	ref := makeRef("settings.theme", "settings", "", "theme", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedVariable {
		t.Fatalf("expected ResolvedVariable, got %v", result.Kind)
	}
	if result.Value != "dark" {
		t.Fatalf("expected 'dark', got %q", result.Value)
	}
}

// TestDetectCycles_MultipleCycles verifies that multiple independent cycles each produce separate diagnostics.
func TestDetectCycles_MultipleCycles(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"b": {"a"},
			"c": {"d"},
			"d": {"c"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) < 2 {
		t.Fatalf("expected at least 2 cycle diagnostics, got %d", len(diags))
	}
	for _, d := range diags {
		if d.Code != "E060" {
			t.Fatalf("expected E060, got %s", d.Code)
		}
	}
}

// TestDetectCycles_PathCorrectness verifies that the reported cycle path contains all nodes and uses arrow notation.
func TestDetectCycles_PathCorrectness(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"b": {"c"},
			"c": {"a"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) == 0 {
		t.Fatal("expected cycle detected")
	}
	// message should contain the cycle path with →
	msg := diags[0].Message
	if !strings.Contains(msg, "→") {
		t.Fatalf("expected → in cycle path, got: %s", msg)
	}
	// should mention all three nodes
	if !strings.Contains(msg, "a") || !strings.Contains(msg, "b") || !strings.Contains(msg, "c") {
		t.Fatalf("expected all nodes in cycle path, got: %s", msg)
	}
}

// TestDetectCycles_LeafNode verifies that a graph with a leaf node (no outgoing edges) produces no cycle.
func TestDetectCycles_LeafNode(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			// "b" not in Edges — it's a leaf
		},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no cycles, got %d", len(diags))
	}
}

// TestDetectCycles_Diamond verifies that a diamond-shaped DAG (a->b,c; b->d; c->d) has no cycle.
func TestDetectCycles_Diamond(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b", "c"},
			"b": {"d"},
			"c": {"d"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no cycles, got %d", len(diags))
	}
}

// TestDetectCycles_MultiEdgeWithCycle verifies that a cycle is detected when one of multiple outgoing edges forms a loop.
func TestDetectCycles_MultiEdgeWithCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b", "c", "d"},
			"d": {"a"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) == 0 {
		t.Fatal("expected cycle detected")
	}
}

// TestResolveReference_CrossDocSectionBySlug verifies that a cross-document section reference resolves by heading slug.
func TestResolveReference_CrossDocSectionBySlug(t *testing.T) {
	targetHeadings := []*ast.Heading{
		{Level: 1, Text: "Getting Started", Slug: "getting-started"},
	}
	targetDoc := makeDocWithHeadings("guide", "guide.md", targetHeadings)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("guide#getting-started", "guide", "getting-started", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
}

// TestResolveReference_CrossDocSectionNotFound verifies that a cross-doc section ref to a missing heading produces E053.
func TestResolveReference_CrossDocSectionNotFound(t *testing.T) {
	targetHeadings := []*ast.Heading{
		{Level: 1, Text: "Intro", Slug: "intro"},
	}
	targetDoc := makeDocWithHeadings("guide", "guide.md", targetHeadings)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("guide#missing", "guide", "missing", "", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E053" {
		t.Fatalf("expected E053, got %s", diag.Code)
	}
}

// TestValidateReferences_MultipleErrors verifies that all invalid references are collected into separate diagnostics.
func TestValidateReferences_MultipleErrors(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	doc := &ast.Document{
		Path: "test.md",
		References: []ast.Reference{
			makeRef("missing1", "missing1", "", "", 1),
			makeRef("missing2", "missing2", "", "", 2),
			makeRef("missing3", "missing3", "", "", 3),
		},
	}

	diags := ValidateReferences(doc, s, nil)
	if len(diags) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(diags))
	}
}

// TestBuildDependencyGraph_ExtendsAndRef verifies that a doc with both @extends and a reference creates two dependency edges.
func TestBuildDependencyGraph_ExtendsAndRef(t *testing.T) {
	base := makeDoc("base", "base.md")
	utils := makeDoc("utils", "utils.md")
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		ExtendsName: "base",
		References: []ast.Reference{
			makeRef("utils", "utils", "", "", 1),
		},
	}
	ws := makeWorkspace(base, utils, child)

	g := BuildDependencyGraph(ws)
	deps := g.Edges["child"]
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	found := map[string]bool{}
	for _, d := range deps {
		found[d] = true
	}
	if !found["base"] || !found["utils"] {
		t.Fatalf("expected [base, utils], got %v", deps)
	}
}

// TestBuildDependencyGraph_RefToNonexistentDoc verifies that a reference to a nonexistent document creates no edge.
func TestBuildDependencyGraph_RefToNonexistentDoc(t *testing.T) {
	main := &ast.Document{
		Name: "main",
		Path: "main.md",
		References: []ast.Reference{
			makeRef("ghost", "ghost", "", "", 1),
		},
	}
	ws := makeWorkspace(main)

	g := BuildDependencyGraph(ws)
	if _, ok := g.Edges["main"]; ok {
		t.Fatal("expected no edge for reference to nonexistent doc")
	}
}

// TestResolveReference_CrossDocVarByPath verifies that a cross-doc variable reference resolves via file path lookup.
func TestResolveReference_CrossDocVarByPath(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "host", Access: ast.AccessPublic, Value: strVal("example.com")},
	}
	targetDoc := &ast.Document{
		Name:      "",
		Path:      "config.md",
		Variables: targetVars,
	}
	ws := &workspace.Workspace{
		Documents:  make(map[string]*ast.Document),
		DocsByPath: map[string]*ast.Document{"config.md": targetDoc},
		Templates:  make(map[string]*ast.Document),
	}
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.md.host", "config.md", "", "host", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Value != "example.com" {
		t.Fatalf("expected 'example.com', got %q", result.Value)
	}
}

// TestResolveReference_ScopeLineResolution verifies that variable visibility is determined by scope line ranges.
func TestResolveReference_ScopeLineResolution(t *testing.T) {
	root := scope.NewScope("root", scope.ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = 20

	child := scope.NewScope("child", scope.ScopeHeading, root)
	child.StartLine = 5
	child.EndLine = 10
	child.Declare("local", ast.Variable{Name: "local", Value: strVal("child_val")})

	// ref at line 7 should see child scope
	ref := makeRef("local", "local", "", "", 7)
	doc := makeDoc("", "test.md")

	result, diag := ResolveReference(ref, doc, root, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Value != "child_val" {
		t.Fatalf("expected 'child_val', got %q", result.Value)
	}

	// ref at line 15 should NOT see child scope variable
	ref2 := makeRef("local", "local", "", "", 15)
	_, diag2 := ResolveReference(ref2, doc, root, nil)
	if diag2 == nil {
		t.Fatal("expected diagnostic for out-of-scope ref")
	}
}

// TestResolveReference_SectionEmptyHeadings verifies that a section ref on a document with no headings produces E053.
func TestResolveReference_SectionEmptyHeadings(t *testing.T) {
	doc := &ast.Document{
		Path:     "test.md",
		Headings: nil,
	}
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#intro", "", "intro", "", 1)

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E053" {
		t.Fatalf("expected E053, got %s", diag.Code)
	}
}

// TestResolveReference_PathBasedExactMdMatch verifies that a path reference with explicit .md extension resolves directly.
func TestResolveReference_PathBasedExactMdMatch(t *testing.T) {
	targetDoc := makeDoc("", "docs/api.md")
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("docs/api.md", "docs/api.md", "", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedDocument {
		t.Fatalf("expected ResolvedDocument, got %v", result.Kind)
	}
}

// TestResolveReference_LocalObjectPropertyMissing verifies that a missing property on a local object falls through to workspace lookup.
func TestResolveReference_LocalObjectPropertyMissing(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"settings": {
			Name: "settings",
			Value: &ast.Value{
				Kind: ast.TypeObject,
				Object: map[string]ast.Value{
					"theme": {Kind: ast.TypeString, Str: "dark"},
				},
			},
		},
	})
	// "settings" exists as local object, but "missing" property doesn't exist
	// should fall through to workspace lookup
	ref := makeRef("settings.missing", "settings", "", "missing", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	// falls through local check, then ws==nil → E051
	if diag.Code != "E051" {
		t.Fatalf("expected E051, got %s", diag.Code)
	}
}

// TestDetectCycles_LongCycle verifies that a five-node cycle (a->b->c->d->e->a) is detected.
func TestDetectCycles_LongCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"b": {"c"},
			"c": {"d"},
			"d": {"e"},
			"e": {"a"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) == 0 {
		t.Fatal("expected cycle detected")
	}
}

// TestDetectCycles_SingleNodeNoEdges verifies that a single node with an empty edge list produces no cycle.
func TestDetectCycles_SingleNodeNoEdges(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {},
		},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no cycles, got %d", len(diags))
	}
}
