package refs

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/workspace"
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
		if d.IsTemplate {
			ws.Templates[d.Path] = d
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

func TestResolveReference_CrossDocSection(t *testing.T) {
	targetHeadings := []*ast.Heading{
		{Level: 1, Text: "API", Slug: "api"},
	}
	targetDoc := makeDocWithHeadings("config", "config.md", targetHeadings)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config#api", "config", "api", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedSection {
		t.Fatalf("expected ResolvedSection, got %v", result.Kind)
	}
}

func TestResolveReference_CrossDocSectionNoWorkspace(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config#api", "config", "api", "", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E051" {
		t.Fatalf("expected E051, got %s", diag.Code)
	}
}

func TestResolveReference_CrossDocSectionDocNotFound(t *testing.T) {
	ws := makeWorkspace()
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("missing#section", "missing", "section", "", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E052" {
		t.Fatalf("expected E052, got %s", diag.Code)
	}
}

// --- ResolveReference: document variable reference ---

func TestResolveReference_DocVariable(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "port", Access: ast.AccessPublic, Value: numVal(8080)},
	}
	targetDoc := makeDocWithVars("config", "config.md", targetVars)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.port", "config", "", "port", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedVariable {
		t.Fatalf("expected ResolvedVariable, got %v", result.Kind)
	}
	if result.Value != "8080" {
		t.Fatalf("expected '8080', got %q", result.Value)
	}
}

func TestResolveReference_DocVariablePrivate(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "secret", Access: ast.AccessPrivate, Value: strVal("hidden")},
	}
	targetDoc := makeDocWithVars("config", "config.md", targetVars)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.secret", "config", "", "secret", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic for private variable")
	}
	if diag.Code != "E054" {
		t.Fatalf("expected E054, got %s", diag.Code)
	}
}

func TestResolveReference_DocVariableNotFound(t *testing.T) {
	targetDoc := makeDocWithVars("config", "config.md", []ast.Variable{})
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.missing", "config", "", "missing", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E054" {
		t.Fatalf("expected E054, got %s", diag.Code)
	}
}

func TestResolveReference_DocVariableNoWorkspace(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.port", "config", "", "port", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E051" {
		t.Fatalf("expected E051, got %s", diag.Code)
	}
}

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

// --- ResolveReference: path-based reference ---

func TestResolveReference_PathBasedDocument(t *testing.T) {
	targetDoc := makeDoc("", "docs/api/config.md")
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("docs/api/config", "docs/api/config", "", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedDocument {
		t.Fatalf("expected ResolvedDocument, got %v", result.Kind)
	}
}

func TestResolveReference_PathBasedWithExtension(t *testing.T) {
	targetDoc := makeDoc("", "docs/api.md")
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	// reference without .md, should try adding .md extension
	ref := makeRef("docs/api", "docs/api", "", "", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Kind != ResolvedDocument {
		t.Fatalf("expected ResolvedDocument, got %v", result.Kind)
	}
}

func TestResolveReference_PathBasedNotFound(t *testing.T) {
	ws := makeWorkspace()
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("docs/missing", "docs/missing", "", "", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E055" {
		t.Fatalf("expected E055, got %s", diag.Code)
	}
}

func TestResolveReference_PathBasedNoWorkspace(t *testing.T) {
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("docs/api", "docs/api", "", "", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, nil)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E051" {
		t.Fatalf("expected E051, got %s", diag.Code)
	}
}

// --- ValidateReferences ---

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

func TestBuildDependencyGraph_NoRefs(t *testing.T) {
	doc := makeDoc("main", "main.md")
	ws := makeWorkspace(doc)

	g := BuildDependencyGraph(ws)
	if len(g.Edges) != 0 {
		t.Fatalf("expected no edges, got %d", len(g.Edges))
	}
}

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

func TestDetectCycles_EmptyGraph(t *testing.T) {
	g := DependencyGraph{
		Edges: map[string][]string{},
	}

	diags := DetectCycles(g)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

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

// --- resolveDocByNameOrPath ---

func TestResolveDocByNameOrPath_ByName(t *testing.T) {
	doc := makeDoc("config", "config.md")
	ws := makeWorkspace(doc)

	result := resolveDocByNameOrPath("config", ws)
	if result != doc {
		t.Fatal("expected to find doc by name")
	}
}

func TestResolveDocByNameOrPath_ByPath(t *testing.T) {
	doc := makeDoc("", "docs/config.md")
	ws := makeWorkspace(doc)

	result := resolveDocByNameOrPath("docs/config.md", ws)
	if result != doc {
		t.Fatal("expected to find doc by path")
	}
}

func TestResolveDocByNameOrPath_ByPathWithMd(t *testing.T) {
	doc := makeDoc("", "docs/config.md")
	ws := makeWorkspace(doc)

	result := resolveDocByNameOrPath("docs/config", ws)
	if result != doc {
		t.Fatal("expected to find doc by path with .md extension")
	}
}

func TestResolveDocByNameOrPath_NotFound(t *testing.T) {
	ws := makeWorkspace()

	result := resolveDocByNameOrPath("nonexistent", ws)
	if result != nil {
		t.Fatal("expected nil for not found")
	}
}

// --- findHeading ---

func TestFindHeading_ByName(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Introduction", Slug: "introduction", Name: "intro"},
	}

	result := findHeading(headings, "intro")
	if result == nil {
		t.Fatal("expected to find heading by name")
	}
}

func TestFindHeading_BySlug(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "My Section", Slug: "my-section"},
	}

	result := findHeading(headings, "my-section")
	if result == nil {
		t.Fatal("expected to find heading by slug")
	}
}

func TestFindHeading_InChildren(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Root", Slug: "root", Children: []*ast.Heading{
			{Level: 2, Text: "Child", Slug: "child"},
		}},
	}

	result := findHeading(headings, "child")
	if result == nil {
		t.Fatal("expected to find child heading")
	}
}

func TestFindHeading_NotFound(t *testing.T) {
	headings := []*ast.Heading{
		{Level: 1, Text: "Intro", Slug: "intro"},
	}

	result := findHeading(headings, "missing")
	if result != nil {
		t.Fatal("expected nil for not found")
	}
}

func TestFindHeading_EmptyList(t *testing.T) {
	result := findHeading(nil, "any")
	if result != nil {
		t.Fatal("expected nil for empty list")
	}
}

// --- ResolveReference: edge cases ---

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

func TestResolveReference_DocVariableProtectedAccess(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "internal", Access: ast.AccessProtected, Value: strVal("hidden")},
	}
	targetDoc := makeDocWithVars("config", "config.md", targetVars)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.internal", "config", "", "internal", 1)
	doc := makeDoc("main", "main.md")

	_, diag := ResolveReference(ref, doc, s, ws)
	if diag == nil {
		t.Fatal("expected diagnostic for protected variable")
	}
	if diag.Code != "E054" {
		t.Fatalf("expected E054, got %s", diag.Code)
	}
}

func TestResolveReference_MultiplePublicVarsFirstMatch(t *testing.T) {
	targetVars := []ast.Variable{
		{Name: "port", Access: ast.AccessPublic, Value: numVal(8080)},
		{Name: "host", Access: ast.AccessPublic, Value: strVal("localhost")},
	}
	targetDoc := makeDocWithVars("config", "config.md", targetVars)
	ws := makeWorkspace(targetDoc)
	s := makeScope(map[string]ast.Variable{})
	ref := makeRef("config.host", "config", "", "host", 1)
	doc := makeDoc("main", "main.md")

	result, diag := ResolveReference(ref, doc, s, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result.Value != "localhost" {
		t.Fatalf("expected 'localhost', got %q", result.Value)
	}
}
