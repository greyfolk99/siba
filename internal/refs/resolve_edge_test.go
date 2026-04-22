package refs

import (
	"strings"
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/workspace"
)

// --- Edge case tests from Codex review ---

// C3: nil Value on local variable
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

// C3: nil Value on doc variable
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

// M4: nil currentDoc on local section reference
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

// M5: local object property access without workspace
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

// C2: multiple independent cycles should all be detected
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

// C1: verify cycle path is correctly reported (not corrupted by slice aliasing)
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

// Leaf node in graph (node referenced but has no outgoing edges)
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

// Diamond graph: a→b, a→c, b→d, c→d (no cycle)
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

// Node with multiple outgoing edges, one forming a cycle
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

// Validate cross-doc section with both name and slug available
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

// Cross-doc section not found
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

// Validate all references: multiple errors collected
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

// BuildDependencyGraph: both @extends and ref from same doc
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

// BuildDependencyGraph: ref to non-existent doc creates no edge
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

// ResolveReference: doc found by path via resolveDocByNameOrPath
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

// ResolveReference: scope line-based resolution
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

// Empty headings list for section reference
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

// Path-based document with exact .md match
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

// Variable with object value but requested property doesn't exist
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

// Long cycle: a→b→c→d→e→a
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

// Single node, no edges
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
