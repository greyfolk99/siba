package template

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/workspace"
)

// --- helpers ---

func makeDoc(name, path string) *ast.Document {
	return &ast.Document{Name: name, Path: path}
}

func makeTemplate(name, path string, headings []*ast.Heading) *ast.Document {
	return &ast.Document{
		Name:       name,
		Path:       path,
		IsTemplate: true,
		Headings:   headings,
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

func strVal(s string) *ast.Value {
	return &ast.Value{Kind: ast.TypeString, Str: s}
}

func h(level int, text, slug, name string, ann ast.Annotation, children ...*ast.Heading) *ast.Heading {
	return &ast.Heading{
		Level:      level,
		Text:       text,
		Slug:       slug,
		Name:       name,
		Annotation: ann,
		Children:   children,
	}
}

// --- ResolveTemplate ---

func TestResolveTemplate_Found(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", nil)
	ws := makeWorkspace(tmpl)
	doc := &ast.Document{Name: "child", ExtendsName: "base"}

	result, diag := ResolveTemplate(doc, ws)
	if diag != nil {
		t.Fatalf("unexpected diagnostic: %v", diag)
	}
	if result != tmpl {
		t.Fatal("expected template reference")
	}
}

func TestResolveTemplate_NotFound(t *testing.T) {
	ws := makeWorkspace()
	doc := &ast.Document{Name: "child", ExtendsName: "missing"}

	_, diag := ResolveTemplate(doc, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	if diag.Code != "E071" {
		t.Fatalf("expected E071, got %s", diag.Code)
	}
}

func TestResolveTemplate_NoExtends(t *testing.T) {
	ws := makeWorkspace()
	doc := &ast.Document{Name: "standalone"}

	result, diag := ResolveTemplate(doc, ws)
	if result != nil || diag != nil {
		t.Fatal("expected nil result and nil diagnostic for no extends")
	}
}

func TestResolveTemplate_NonTemplateDoc(t *testing.T) {
	regular := makeDoc("base", "base.md") // not a template
	ws := makeWorkspace(regular)
	doc := &ast.Document{Name: "child", ExtendsName: "base"}

	_, diag := ResolveTemplate(doc, ws)
	if diag == nil {
		t.Fatal("expected diagnostic")
	}
	// GetTemplate checks IsTemplate, so it returns nil → E071
	if diag.Code != "E071" {
		t.Fatalf("expected E071, got %s", diag.Code)
	}
}

// --- ValidateContract ---

func TestValidateContract_AllRequiredPresent(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
		h(1, "Summary", "summary", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
			h(1, "Summary", "summary", "", ast.AnnotationRequired),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestValidateContract_RequiredMissing(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
		h(1, "Summary", "summary", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
			// Summary missing
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != "E070" {
		t.Fatalf("expected E070, got %s", diags[0].Code)
	}
}

func TestValidateContract_DefaultHeadingOptional(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
		h(1, "Appendix", "appendix", "", ast.AnnotationDefault),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
			// Appendix omitted — @default so OK
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

func TestValidateContract_LevelMismatch(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Introduction", "introduction", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(2, "Introduction", "introduction", "", ast.AnnotationRequired), // wrong level
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != "E072" {
		t.Fatalf("expected E072, got %s", diags[0].Code)
	}
}

func TestValidateContract_NestedRequiredMissing(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Chapter", "chapter", "", ast.AnnotationRequired,
			h(2, "Section A", "section-a", "", ast.AnnotationRequired),
			h(2, "Section B", "section-b", "", ast.AnnotationRequired),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Chapter", "chapter", "", ast.AnnotationRequired,
				h(2, "Section A", "section-a", "", ast.AnnotationRequired),
				// Section B missing
			),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Code != "E070" {
		t.Fatalf("expected E070, got %s", diags[0].Code)
	}
}

func TestValidateContract_MatchByName(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Chapter One", "chapter-one", "ch1", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "First Chapter", "first-chapter", "ch1", ast.AnnotationRequired), // same @name
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

func TestValidateContract_EmptyTemplate(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", nil)
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Anything", "anything", "", ast.AnnotationRequired),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

func TestValidateContract_EmptyChild(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Required", "required", "", ast.AnnotationRequired),
	})
	child := &ast.Document{Headings: nil}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestValidateContract_AllDefault(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Optional A", "optional-a", "", ast.AnnotationDefault),
		h(1, "Optional B", "optional-b", "", ast.AnnotationDefault),
	})
	child := &ast.Document{Headings: nil}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

// --- InheritVariables ---

func TestInheritVariables_PublicInherited(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "version", Access: ast.AccessPublic, Value: strVal("1.0")},
		},
	}
	child := &ast.Document{Variables: nil}

	result := InheritVariables(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 var, got %d", len(result))
	}
	if result[0].Name != "version" {
		t.Fatalf("expected 'version', got %q", result[0].Name)
	}
}

func TestInheritVariables_PrivateExcluded(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "secret", Access: ast.AccessPrivate, Value: strVal("hidden")},
		},
	}
	child := &ast.Document{Variables: nil}

	result := InheritVariables(child, tmpl)
	if len(result) != 0 {
		t.Fatalf("expected 0 vars, got %d", len(result))
	}
}

func TestInheritVariables_ProtectedInherited(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "internal", Access: ast.AccessProtected, Value: strVal("protected")},
		},
	}
	child := &ast.Document{Variables: nil}

	result := InheritVariables(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 var, got %d", len(result))
	}
}

func TestInheritVariables_ChildOverrides(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "version", Access: ast.AccessPublic, Value: strVal("1.0")},
		},
	}
	child := &ast.Document{
		Variables: []ast.Variable{
			{Name: "version", Value: strVal("2.0")},
		},
	}

	result := InheritVariables(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 var, got %d", len(result))
	}
	if result[0].Value.Str != "2.0" {
		t.Fatalf("expected '2.0' (child override), got %q", result[0].Value.Str)
	}
}

func TestInheritVariables_MixedAccess(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "pub", Access: ast.AccessPublic, Value: strVal("public")},
			{Name: "priv", Access: ast.AccessPrivate, Value: strVal("private")},
			{Name: "prot", Access: ast.AccessProtected, Value: strVal("protected")},
		},
	}
	child := &ast.Document{Variables: nil}

	result := InheritVariables(child, tmpl)
	if len(result) != 2 {
		t.Fatalf("expected 2 vars (pub+prot), got %d", len(result))
	}
	names := map[string]bool{}
	for _, v := range result {
		names[v.Name] = true
	}
	if !names["pub"] || !names["prot"] {
		t.Fatal("expected pub and prot to be inherited")
	}
	if names["priv"] {
		t.Fatal("private should not be inherited")
	}
}

func TestInheritVariables_ChildFirst(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "base_var", Access: ast.AccessPublic, Value: strVal("from_template")},
		},
	}
	child := &ast.Document{
		Variables: []ast.Variable{
			{Name: "child_var", Value: strVal("from_child")},
		},
	}

	result := InheritVariables(child, tmpl)
	if len(result) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(result))
	}
	// child vars should come first
	if result[0].Name != "child_var" {
		t.Fatalf("expected child var first, got %q", result[0].Name)
	}
	if result[1].Name != "base_var" {
		t.Fatalf("expected inherited var second, got %q", result[1].Name)
	}
}

func TestInheritVariables_BothEmpty(t *testing.T) {
	tmpl := &ast.Document{Variables: nil}
	child := &ast.Document{Variables: nil}

	result := InheritVariables(child, tmpl)
	if len(result) != 0 {
		t.Fatalf("expected 0 vars, got %d", len(result))
	}
}

// --- MergeHeadings ---

func TestMergeHeadings_ChildOverrides(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Intro", "intro", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Intro", "intro", "", ast.AnnotationRequired),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 heading, got %d", len(result))
	}
}

func TestMergeHeadings_DefaultUsed(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Appendix", "appendix", "", ast.AnnotationDefault),
	})
	child := &ast.Document{Headings: nil}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 heading (from template default), got %d", len(result))
	}
	if result[0].Text != "Appendix" {
		t.Fatalf("expected 'Appendix', got %q", result[0].Text)
	}
}

func TestMergeHeadings_ExtraChildHeading(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Intro", "intro", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Intro", "intro", "", ast.AnnotationRequired),
			h(1, "Extra", "extra", "", ast.AnnotationRequired),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 2 {
		t.Fatalf("expected 2 headings, got %d", len(result))
	}
	if result[1].Text != "Extra" {
		t.Fatalf("expected 'Extra' appended, got %q", result[1].Text)
	}
}

func TestMergeHeadings_NestedMerge(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Chapter", "chapter", "", ast.AnnotationRequired,
			h(2, "Default Section", "default-section", "", ast.AnnotationDefault),
			h(2, "Required Section", "required-section", "", ast.AnnotationRequired),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Chapter", "chapter", "", ast.AnnotationRequired,
				h(2, "Required Section", "required-section", "", ast.AnnotationRequired),
				// Default Section not provided → should use template's
			),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 top-level heading, got %d", len(result))
	}
	children := result[0].Children
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}
}

func TestMergeHeadings_EmptyTemplate(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", nil)
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Custom", "custom", "", ast.AnnotationRequired),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 heading, got %d", len(result))
	}
}

func TestMergeHeadings_EmptyChild(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Default", "default", "", ast.AnnotationDefault),
	})
	child := &ast.Document{Headings: nil}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 heading (from default), got %d", len(result))
	}
}

// --- matchesHeading ---

func TestMatchesHeading_ByText(t *testing.T) {
	a := h(1, "Hello", "hello", "", ast.AnnotationRequired)
	b := h(1, "Hello", "hello", "", ast.AnnotationRequired)

	if !matchesHeading(a, b) {
		t.Fatal("expected match by text")
	}
}

func TestMatchesHeading_BySlug(t *testing.T) {
	a := h(1, "Different Text", "same-slug", "", ast.AnnotationRequired)
	b := h(1, "Another Text", "same-slug", "", ast.AnnotationRequired)

	if !matchesHeading(a, b) {
		t.Fatal("expected match by slug")
	}
}

func TestMatchesHeading_ByName(t *testing.T) {
	a := h(1, "Different", "different", "samename", ast.AnnotationRequired)
	b := h(1, "Also Different", "also-different", "samename", ast.AnnotationRequired)

	if !matchesHeading(a, b) {
		t.Fatal("expected match by name")
	}
}

func TestMatchesHeading_NoMatch(t *testing.T) {
	a := h(1, "Hello", "hello", "", ast.AnnotationRequired)
	b := h(1, "World", "world", "", ast.AnnotationRequired)

	if matchesHeading(a, b) {
		t.Fatal("expected no match")
	}
}
