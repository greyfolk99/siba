// Package refs tests cover reference resolution, validation, dependency graph
// construction, and cycle detection for the siba document system.
package refs

import (
	"testing"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/scope"
	"github.com/greyfolk99/siba/pkg/workspace"
)

// --- helpers ---

func makeScope(vars map[string]ast.Variable) *scope.Scope {
	s := scope.NewScope("root", scope.ScopeHeading, nil)
	s.StartLine = 1
	s.EndLine = 100
	for name, v := range vars {
		s.Declare(name, v)
	}
	return s
}

func strVal(s string) *ast.Value {
	return &ast.Value{Kind: ast.TypeString, Str: s}
}

func numVal(n float64) *ast.Value {
	return &ast.Value{Kind: ast.TypeNumber, Num: n, Raw: ""}
}

func makeDoc(name, path string) *ast.Document {
	return &ast.Document{
		Name: name,
		Path: path,
	}
}

func makeDocWithHeadings(name, path string, headings []*ast.Heading) *ast.Document {
	return &ast.Document{
		Name:     name,
		Path:     path,
		Headings: headings,
	}
}

func makeDocWithVars(name, path string, vars []ast.Variable) *ast.Document {
	return &ast.Document{
		Name:      name,
		Path:      path,
		Variables: vars,
	}
}

func makeWorkspace(docs ...*ast.Document) *workspace.Workspace {
	ws := &workspace.Workspace{
		Documents:  make(map[string]*ast.Document),
		DocsByPath: make(map[string]*ast.Document),
		Templates:  make(map[string]*ast.Document),
	}
	for _, d := range docs {
		if d.Name != "" {
			ws.Documents[d.Name] = d
		}
		if d.Path != "" {
			ws.DocsByPath[d.Path] = d
		}
		if d.IsTemplate && d.Name != "" {
			ws.Templates[d.Name] = d
		}
	}
	return ws
}

func makeRef(raw, pathPart, section, variable string, line int) ast.Reference {
	return ast.Reference{
		Raw:      raw,
		PathPart: pathPart,
		Section:  section,
		Variable: variable,
		Position: ast.Position{Line: line, Column: 1},
	}
}

// --- ResolveReference: local variable ---

// TestResolveReference_LocalVariable verifies that a reference resolves to a local variable in scope.
func TestResolveReference_LocalVariable(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Value: strVal("Hello")},
	})
	ref := makeRef("title", "title", "", "", 1)
	doc := makeDoc("", "test.md")

	result, diag := ResolveReference(ref, doc, s, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedVariable {
		t.Fatalf("expected ResolvedVariable, got %v", result.Kind)
	}
	if result.Value != "Hello" {
		t.Fatalf("expected 'Hello', got %q", result.Value)
	}
	if result.Variable == nil {
		t.Fatal("expected non-nil Variable")
	}
}

// TestResolveReference_LocalVariableNotFound verifies that referencing an undeclared variable produces an E050 diagnostic.
func TestResolveReference_LocalVariableNotFound(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("unknown", "unknown", "", "", 1)
	doc := makeDoc("", "test.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E050" {
		t.Fatalf("expected E050, got %s", diag.Code)
	}
}

// TestResolveReference_EscapedReference verifies that escaped references return nil result and nil diagnostic.
func TestResolveReference_EscapedReference(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := ast.Reference{
		Raw:       "\\{{title}}",
		PathPart:  "title",
		IsEscaped: true,
		Position:  ast.Position{Line: 1, Column: 1},
	}
	doc := makeDoc("", "test.md")

	result, diag := ResolveReference(ref, doc, s, nil)
	if result != nil {
		t.Fatal("expected nil result for escaped reference")
	}
	if diag != nil {
		t.Fatal("expected nil diagnostic for escaped reference")
	}
}

// --- ResolveReference: document reference ---

// TestResolveReference_DocumentByName verifies that a simple name reference resolves to a document in the workspace.
func TestResolveReference_DocumentByName(t *testing.T) {
	targetDoc := makeDoc("config", "config.md")
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config", "config", "", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedDocument {
		t.Fatalf("expected ResolvedDocument, got %v", result.Kind)
	}
	if result.Document != targetDoc {
		t.Fatal("expected target document reference")
	}
}

// TestResolveReference_LocalVariablePriority verifies that a local variable takes precedence over a document with the same name.
func TestResolveReference_LocalVariablePriority(t *testing.T) {
	// local variable should take priority over document name
	targetDoc := makeDoc("title", "title.md")
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Value: strVal("Local Title")},
	})
	ref := makeRef("title", "title", "", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedVariable {
		t.Fatalf("expected ResolvedVariable (local priority), got %v", result.Kind)
	}
	if result.Value != "Local Title" {
		t.Fatalf("expected 'Local Title', got %q", result.Value)
	}
}

// --- ResolveReference: section reference ---

// TestResolveReference_SectionInCurrentDoc verifies that a #section reference resolves to a heading by name in the current document.
func TestResolveReference_SectionInCurrentDoc(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Introduction", Slug: "introduction", Name: "intro"},
	}
	doc := makeDocWithHeadings("main", "main.md", headings)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#intro", "", "intro", "", 1)

	result, diag := ResolveReference(ref, doc, s, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
	if result.Heading != headings[0] {
		t.Fatal("expected matching heading")
	}
}

// TestResolveReference_SectionBySlug verifies that a #section reference resolves to a heading by its slug.
func TestResolveReference_SectionBySlug(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "My Section", Slug: "my-section"},
	}
	doc := makeDocWithHeadings("main", "main.md", headings)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#my-section", "", "my-section", "", 1)

	result, diag := ResolveReference(ref, doc, s, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
}

// TestResolveReference_SectionNotFound verifies that referencing a nonexistent section produces an E053 diagnostic.
func TestResolveReference_SectionNotFound(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Intro", Slug: "intro"},
	}
	doc := makeDocWithHeadings("main", "main.md", headings)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#nonexistent", "", "nonexistent", "", 1)

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E053" {
		t.Fatalf("expected E053, got %s", diag.Code)
	}
}

// TestResolveReference_SectionInNestedHeading verifies that a section reference resolves through deeply nested heading children.
func TestResolveReference_SectionInNestedHeading(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Chapter", Slug: "chapter", Children: []*ast.Heading{
			{Level: 2, Text: "Section", Slug: "section", Children: []*ast.Heading{
				{Level: 3, Text: "Subsection", Slug: "subsection"},
			}},
		}},
	}
	doc := makeDocWithHeadings("main", "main.md", headings)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("#subsection", "", "subsection", "", 1)

	result, diag := ResolveReference(ref, doc, s, nil)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
	if result.Heading.Slug != "subsection" {
		t.Fatalf("expected 'subsection' heading, got %q", result.Heading.Slug)
	}
}

// TestResolveReference_CrossDocSection verifies that an alias#section reference resolves a heading in an imported document.
func TestResolveReference_CrossDocSection(t *testing.T) {
	targetHeadings := []*ast.Heading{
		{Level: 1, Text: "API", Slug: "api"},
	}
	targetDoc := makeDocWithHeadings("config", "config.md", targetHeadings)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config#api", "config", "api", "", 1)
	doc := &ast.Document{
		Name: "main",
		Path: "main.md",
		Imports: []ast.Import{
			{Alias: "config", Path: "config.md"},
		},
	}

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
}

// TestResolveReference_CrossDocSectionNoWorkspace verifies that an alias#section ref without a workspace produces E052 (imported file not found).
func TestResolveReference_CrossDocSectionNoWorkspace(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config#api", "config", "api", "", 1)
	doc := &ast.Document{
		Name: "main",
		Path: "main.md",
		Imports: []ast.Import{
			{Alias: "config", Path: "config.md"},
		},
	}

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E052" {
		t.Fatalf("expected E052, got %s", diag.Code)
	}
}

// TestResolveReference_CrossDocSectionDocNotFound verifies that an alias#section ref to a missing import produces E052.
func TestResolveReference_CrossDocSectionDocNotFound(t *testing.T) {
	ws := makeWorkspace()
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("missing#section", "missing", "section", "", 1)
	doc := &ast.Document{
		Name: "main",
		Path: "main.md",
		Imports: []ast.Import{
			{Alias: "missing", Path: "missing.md"},
		},
	}

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E052" {
		t.Fatalf("expected E052, got %s", diag.Code)
	}
}

// TestResolveReference_LocalObjectPropertyAccess verifies that local object property access takes priority over document lookup.
func TestResolveReference_LocalObjectPropertyAccess(t *testing.T) {
	// obj.prop should resolve as local object property first
	s := makeScope(map[string]ast.Variable{
		"config": {
			Name: "config",
			Value: &ast.Value{
				Kind: ast.TypeObject,
				Object: map[string]ast.Value{
					"port": {Kind: ast.TypeNumber, Num: 3000, Raw: "3000"},
				},
			},
		},
	})
	// Even with workspace, local object property should take priority
	ws := makeWorkspace(makeDoc("config", "config.md"))
	ref := makeRef("config.port", "config", "", "port", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedVariable {
		t.Fatalf("expected ResolvedVariable, got %v", result.Kind)
	}
	if result.Value != "3000" {
		t.Fatalf("expected '3000', got %q", result.Value)
	}
}

// --- ValidateReferences ---

// TestValidateReferences_AllValid verifies that a document with all valid references produces zero diagnostics.
func TestValidateReferences_AllValid(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Value: strVal("Hello")},
		"count": {Name: "count", Value: numVal(5)},
	})
	doc := &ast.Document{
		Path: "test.md",
		References: []ast.Reference{
			makeRef("title", "title", "", "", 1),
			makeRef("count", "count", "", "", 2),
		},
	}

	diags := ValidateReferences(doc, s, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

// TestValidateReferences_MixedValidInvalid verifies that only invalid references produce diagnostics when mixed with valid ones.
func TestValidateReferences_MixedValidInvalid(t *testing.T) {
	s := makeScope(map[string]ast.Variable{
		"title": {Name: "title", Value: strVal("Hello")},
	})
	doc := &ast.Document{
		Path: "test.md",
		References: []ast.Reference{
			makeRef("title", "title", "", "", 1),
			makeRef("missing", "missing", "", "", 2),
		},
	}

	diags := ValidateReferences(doc, s, nil)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != "E050" {
		t.Fatalf("expected E050, got %s", diags[0].Code)
	}
}

// TestValidateReferences_SkipsEscaped verifies that escaped references are skipped during validation.
func TestValidateReferences_SkipsEscaped(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	doc := &ast.Document{
		Path: "test.md",
		References: []ast.Reference{
			{Raw: "\\{{escaped}}", PathPart: "escaped", IsEscaped: true, Position: ast.Position{Line: 1}},
		},
	}

	diags := ValidateReferences(doc, s, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for escaped, got %d", len(diags))
	}
}

// TestValidateReferences_NoReferences verifies that a document with no references produces zero diagnostics.
func TestValidateReferences_NoReferences(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	doc := &ast.Document{
		Path:       "test.md",
		References: nil,
	}

	diags := ValidateReferences(doc, s, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

// --- BuildDependencyGraph ---

// TestBuildDependencyGraph_NoRefs verifies that a document with no references produces an empty dependency graph.
func TestBuildDependencyGraph_NoRefs(t *testing.T) {
	doc := makeDoc("main", "main.md")
	ws := makeWorkspace(doc)

	g := BuildDependencyGraph(ws)
	if len(g.Edges) != 0 {
		t.Fatalf("expected no edges, got %d", len(g.Edges))
	}
}

// TestBuildDependencyGraph_ExtendsCreatesEdge verifies that an @extends relationship creates a dependency edge.
func TestBuildDependencyGraph_ExtendsCreatesEdge(t *testing.T) {
	base := makeDoc("base", "base.md")
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		ExtendsName: "base",
	}
	ws := makeWorkspace(base, child)

	g := BuildDependencyGraph(ws)
	deps, ok := g.Edges["child"]
	if !ok {
		t.Fatal("expected edge from child")
	}
	if len(deps) != 1 || deps[0] != "base" {
		t.Fatalf("expected [base], got %v", deps)
	}
}

// TestBuildDependencyGraph_DocRefCreatesEdge verifies that a document reference creates a dependency edge.
func TestBuildDependencyGraph_DocRefCreatesEdge(t *testing.T) {
	config := makeDoc("config", "config.md")
	main := &ast.Document{
		Name: "main",
		Path: "main.md",
		References: []ast.Reference{
			makeRef("config", "config", "", "", 1),
		},
	}
	ws := makeWorkspace(config, main)

	g := BuildDependencyGraph(ws)
	deps, ok := g.Edges["main"]
	if !ok {
		t.Fatal("expected edge from main")
	}
	if len(deps) != 1 || deps[0] != "config" {
		t.Fatalf("expected [config], got %v", deps)
	}
}

// TestBuildDependencyGraph_EscapedRefIgnored verifies that escaped references do not create dependency edges.
func TestBuildDependencyGraph_EscapedRefIgnored(t *testing.T) {
	config := makeDoc("config", "config.md")
	main := &ast.Document{
		Name: "main",
		Path: "main.md",
		References: []ast.Reference{
			{Raw: "\\{{config}}", PathPart: "config", IsEscaped: true, Position: ast.Position{Line: 1}},
		},
	}
	ws := makeWorkspace(config, main)

	g := BuildDependencyGraph(ws)
	if _, ok := g.Edges["main"]; ok {
		t.Fatal("expected no edge from main (escaped ref)")
	}
}

// TestBuildDependencyGraph_PathRefsIgnored verifies that path-based references (containing /) do not create dependency edges.
func TestBuildDependencyGraph_PathRefsIgnored(t *testing.T) {
	// path refs (containing /) should not create dependency edges
	main := &ast.Document{
		Name: "main",
		Path: "main.md",
		References: []ast.Reference{
			makeRef("docs/api", "docs/api", "", "", 1),
		},
	}
	ws := makeWorkspace(main)

	g := BuildDependencyGraph(ws)
	if _, ok := g.Edges["main"]; ok {
		t.Fatal("expected no edge from path-based ref")
	}
}

// TestBuildDependencyGraph_UsesDocNameAsID verifies that the document Name field is used as the graph node ID.
func TestBuildDependencyGraph_UsesDocNameAsID(t *testing.T) {
	doc := &ast.Document{
		Name:        "my-doc",
		Path:        "some/path/doc.md",
		ExtendsName: "base",
	}
	base := makeDoc("base", "base.md")
	ws := makeWorkspace(doc, base)

	g := BuildDependencyGraph(ws)
	// should use Name "my-doc" as ID, not path
	if _, ok := g.Edges["my-doc"]; !ok {
		t.Fatal("expected edges keyed by doc Name")
	}
}

// TestBuildDependencyGraph_UsesPathAsIDWhenNoName verifies that the file path is used as node ID when the document has no name.
func TestBuildDependencyGraph_UsesPathAsIDWhenNoName(t *testing.T) {
	doc := &ast.Document{
		Name:        "",
		Path:        "orphan.md",
		ExtendsName: "base",
	}
	base := makeDoc("base", "base.md")
	ws := makeWorkspace(doc, base)

	g := BuildDependencyGraph(ws)
	if _, ok := g.Edges["orphan.md"]; !ok {
		t.Fatal("expected edges keyed by path when no name")
	}
}

// --- DetectCycles ---

// TestDetectCycles_NoCycle verifies that a linear dependency chain (a->b->c) produces no cycle diagnostics.
func TestDetectCycles_NoCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"b": {"c"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no cycles, got %d diagnostics", len(diags))
	}
}

// TestDetectCycles_DirectCycle verifies that a two-node cycle (a<->b) is detected with E060.
func TestDetectCycles_DirectCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"b": {"a"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) == 0 {
		t.Fatal("expected cycle detected")
	}
	if diags[0].Code != "E060" {
		t.Fatalf("expected E060, got %s", diags[0].Code)
	}
}

// TestDetectCycles_SelfCycle verifies that a self-referencing node (a->a) is detected with E060.
func TestDetectCycles_SelfCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"a"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) == 0 {
		t.Fatal("expected cycle detected")
	}
	if diags[0].Code != "E060" {
		t.Fatalf("expected E060, got %s", diags[0].Code)
	}
}

// TestDetectCycles_IndirectCycle verifies that a three-node cycle (a->b->c->a) is detected.
func TestDetectCycles_IndirectCycle(t *testing.T) {
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
}

// TestDetectCycles_EmptyGraph verifies that an empty dependency graph produces no diagnostics.
func TestDetectCycles_EmptyGraph(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

// TestDetectCycles_DisconnectedNoCycle verifies that disconnected acyclic subgraphs produce no cycle diagnostics.
func TestDetectCycles_DisconnectedNoCycle(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{
			"a": {"b"},
			"c": {"d"},
		},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no cycles, got %d", len(diags))
	}
}

// --- ast.FindHeading ---

// TestFindHeading_ByName verifies that a heading is found by its explicit name attribute.
func TestFindHeading_ByName(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Introduction", Slug: "introduction", Name: "intro"},
	}

	result := ast.FindHeading(headings, "intro")
	if result == nil {
		t.Fatal("expected to find heading by name")
	}
}

// TestFindHeading_BySlug verifies that a heading is found by its auto-generated slug.
func TestFindHeading_BySlug(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "My Section", Slug: "my-section"},
	}

	result := ast.FindHeading(headings, "my-section")
	if result == nil {
		t.Fatal("expected to find heading by slug")
	}
}

// TestFindHeading_InChildren verifies that ast.FindHeading recursively searches child headings.
func TestFindHeading_InChildren(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Root", Slug: "root", Children: []*ast.Heading{
			{Level: 2, Text: "Child", Slug: "child"},
		}},
	}

	result := ast.FindHeading(headings, "child")
	if result == nil {
		t.Fatal("expected to find child heading")
	}
}

// TestFindHeading_NotFound verifies that a missing heading identifier returns nil.
func TestFindHeading_NotFound(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Intro", Slug: "intro"},
	}

	result := ast.FindHeading(headings, "missing")
	if result != nil {
		t.Fatal("expected nil for not found")
	}
}

// TestFindHeading_EmptyList verifies that searching a nil heading list returns nil.
func TestFindHeading_EmptyList(t *testing.T) {
	result := ast.FindHeading(nil, "any")
	if result != nil {
		t.Fatal("expected nil for empty list")
	}
}

// --- ResolveReference: edge cases ---

// TestResolveReference_UnresolvedRawFallback verifies that a reference with empty PathPart falls back to E050 using raw text.
func TestResolveReference_UnresolvedRawFallback(t *testing.T) {
	// ref with empty PathPart should return E050 with raw
	s := makeScope(map[string]ast.Variable{})
	ref := ast.Reference{
		Raw:      "{{}}",
		PathPart: "",
		Position: ast.Position{Line: 1},
	}
	doc := makeDoc("", "test.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E050" {
		t.Fatalf("expected E050, got %s", diag.Code)
	}
}

