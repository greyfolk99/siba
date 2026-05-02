// Package validate tests verify document-level and workspace-level validation,
// including reference resolution, duplicate variable detection, template contract
// enforcement, circular reference detection, and diagnostic filtering utilities.
package validate

import (
	"testing"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/workspace"
)

// --- helpers ---

func strVal(s string) *ast.Value {
	return &ast.Value{Kind: ast.TypeString, Str: s}
}

func makeDoc(name, path, source string) *ast.Document {
	return &ast.Document{Name: name, Path: path, Source: source}
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

// --- ValidateDocument ---

// TestValidateDocument_NoIssues verifies that a well-formed document with resolved references produces no diagnostics.
func TestValidateDocument_NoIssues(t *testing.T) {
	doc := &ast.Document{
		Path:   "test.md",
		Source: "# Hello\n\nContent here.\n",
		Variables: []ast.Variable{
			{Name: "title", Mutability: ast.MutConst, Value: strVal("Hello"), Position: ast.Position{Line: 1}},
		},
		References: []ast.Reference{
			{Raw: "title", PathPart: "title", Position: ast.Position{Line: 3, Column: 1}},
		},
	}

	diags := ValidateDocument(doc, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

// TestValidateDocument_UnresolvedRef verifies that an unresolved reference produces an E050 diagnostic.
func TestValidateDocument_UnresolvedRef(t *testing.T) {
	doc := &ast.Document{
		Path:   "test.md",
		Source: "# Hello\n\nContent {{missing}} here.\n",
		References: []ast.Reference{
			{Raw: "missing", PathPart: "missing", Position: ast.Position{Line: 3, Column: 10}},
		},
	}

	diags := ValidateDocument(doc, nil)
	if len(diags) == 0 {
		t.Fatal("expected diagnostic for unresolved reference")
	}
	found := false
	for _, d := range diags {
		if d.Code == "E050" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E050 diagnostic")
	}
}

// TestValidateDocument_DuplicateConst verifies that declaring the same const variable twice produces an E020 diagnostic.
func TestValidateDocument_DuplicateConst(t *testing.T) {
	doc := &ast.Document{
		Path:   "test.md",
		Source: "line1\nline2\nline3\n",
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: strVal("a"), Position: ast.Position{Line: 1}},
			{Name: "x", Mutability: ast.MutConst, Value: strVal("b"), Position: ast.Position{Line: 2}},
		},
	}

	diags := ValidateDocument(doc, nil)
	found := false
	for _, d := range diags {
		if d.Code == "E020" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E020 diagnostic for duplicate const")
	}
}

// TestValidateDocument_SetsFilePath verifies that every diagnostic carries the correct file path from the document.
func TestValidateDocument_SetsFilePath(t *testing.T) {
	doc := &ast.Document{
		Path:   "docs/api.md",
		Source: "# Hello\n",
		References: []ast.Reference{
			{Raw: "missing", PathPart: "missing", Position: ast.Position{Line: 1}},
		},
	}

	diags := ValidateDocument(doc, nil)
	for _, d := range diags {
		if d.File != "docs/api.md" {
			t.Fatalf("expected file 'docs/api.md', got %q", d.File)
		}
	}
}

// TestValidateDocument_TemplateValidation verifies that a child missing a required template heading gets an E070 diagnostic.
func TestValidateDocument_TemplateValidation(t *testing.T) {
	tmpl := &ast.Document{
		Name:       "base",
		Path:       "base.md",
		IsTemplate: true,
		Headings: []*ast.Heading{
			{Level: 1, Text: "Template", Slug: "template", Annotation: ast.AnnotationRequired,
				Children: []*ast.Heading{
					{Level: 2, Text: "Required", Slug: "required", Annotation: ast.AnnotationRequired},
				}},
		},
	}
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		Source:      "# Different\n",
		ExtendsName: "base",
		Headings:    nil, // missing required heading
	}
	ws := makeWorkspace(tmpl, child)

	diags := ValidateDocument(child, ws)
	found := false
	for _, d := range diags {
		if d.Code == "E070" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E070 for missing required heading")
	}
}

// TestValidateDocument_TemplateNotFound verifies that extending a nonexistent template produces an E071 diagnostic.
func TestValidateDocument_TemplateNotFound(t *testing.T) {
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		Source:      "# Test\n",
		ExtendsName: "nonexistent",
	}
	ws := makeWorkspace(child)

	diags := ValidateDocument(child, ws)
	found := false
	for _, d := range diags {
		if d.Code == "E071" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E071 for template not found")
	}
}

// TestValidateDocument_NoExtends_SkipsTemplate verifies that a standalone document skips template validation entirely.
func TestValidateDocument_NoExtends_SkipsTemplate(t *testing.T) {
	doc := &ast.Document{
		Path:   "standalone.md",
		Source: "# Hello\n",
	}
	ws := makeWorkspace(doc)

	diags := ValidateDocument(doc, ws)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}

// TestValidateDocument_NoWorkspace_SkipsTemplate verifies that a nil workspace gracefully skips template validation without panic.
func TestValidateDocument_NoWorkspace_SkipsTemplate(t *testing.T) {
	doc := &ast.Document{
		Path:        "child.md",
		Source:      "# Hello\n",
		ExtendsName: "base",
	}

	diags := ValidateDocument(doc, nil)
	// no workspace → skip template validation, no panic
	// may have other diags but no panic
	_ = diags
}

// --- ValidateWorkspace ---

// TestValidateWorkspace_NoIssues verifies that a clean workspace with one valid document produces no diagnostics.
func TestValidateWorkspace_NoIssues(t *testing.T) {
	doc := &ast.Document{
		Name:   "main",
		Path:   "main.md",
		Source: "# Hello\n",
	}
	ws := makeWorkspace(doc)

	fileDiags, wsDiags := ValidateWorkspace(ws)
	if len(fileDiags) != 0 {
		t.Fatalf("expected no file diagnostics, got %d", len(fileDiags))
	}
	if len(wsDiags) != 0 {
		t.Fatalf("expected no workspace diagnostics, got %d", len(wsDiags))
	}
}

// TestValidateWorkspace_CircularEmbed verifies that mutually-embedding documents
// produce an E022 circular embed diagnostic (spec-v4: embed cycles → E022).
func TestValidateWorkspace_CircularEmbed(t *testing.T) {
	docA := &ast.Document{
		Name:   "a",
		Path:   "a.md",
		Source: "# A\n",
		References: []ast.Reference{
			{Raw: "{{b}}", PathPart: "b", Position: ast.Position{Line: 1}},
		},
	}
	docB := &ast.Document{
		Name:   "b",
		Path:   "b.md",
		Source: "# B\n",
		References: []ast.Reference{
			{Raw: "{{a}}", PathPart: "a", Position: ast.Position{Line: 1}},
		},
	}
	ws := makeWorkspace(docA, docB)

	_, wsDiags := ValidateWorkspace(ws)
	found := false
	for _, d := range wsDiags {
		if d.Code == "E022" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E022 for circular embed")
	}
}

// TestValidateWorkspace_CircularExtends verifies that mutually-extending templates
// produce an E060 circular reference diagnostic.
func TestValidateWorkspace_CircularExtends(t *testing.T) {
	docA := &ast.Document{
		Name:        "a",
		Path:        "a.md",
		Source:      "# A\n",
		ExtendsName: "b",
	}
	docB := &ast.Document{
		Name:        "b",
		Path:        "b.md",
		Source:      "# B\n",
		ExtendsName: "a",
	}
	ws := makeWorkspace(docA, docB)

	_, wsDiags := ValidateWorkspace(ws)
	found := false
	for _, d := range wsDiags {
		if d.Code == "E060" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected E060 for circular extends")
	}
}

// TestStreamRender_LinkCycleAllowed verifies that a [[]] link cycle (a → b → a)
// is allowed and does not raise E022/E060.
func TestValidateWorkspace_LinkCycleAllowed(t *testing.T) {
	docA := &ast.Document{
		Name:   "a",
		Path:   "a.md",
		Source: "# A\n",
		References: []ast.Reference{
			{Raw: "[[b]]", PathPart: "b", IsLink: true, Position: ast.Position{Line: 1}},
		},
	}
	docB := &ast.Document{
		Name:   "b",
		Path:   "b.md",
		Source: "# B\n",
		References: []ast.Reference{
			{Raw: "[[a]]", PathPart: "a", IsLink: true, Position: ast.Position{Line: 1}},
		},
	}
	ws := makeWorkspace(docA, docB)

	_, wsDiags := ValidateWorkspace(ws)
	for _, d := range wsDiags {
		if d.Code == "E022" || d.Code == "E060" {
			t.Errorf("unexpected cycle diagnostic %s for link cycle: %v", d.Code, d)
		}
	}
}

// TestValidateWorkspace_MultipleFiles verifies that diagnostics are correctly partitioned by file path.
func TestValidateWorkspace_MultipleFiles(t *testing.T) {
	good := &ast.Document{
		Name:   "good",
		Path:   "good.md",
		Source: "# Good\n",
	}
	bad := &ast.Document{
		Name:   "bad",
		Path:   "bad.md",
		Source: "# Bad\n",
		References: []ast.Reference{
			{Raw: "missing", PathPart: "missing", Position: ast.Position{Line: 1}},
		},
	}
	ws := makeWorkspace(good, bad)

	fileDiags, _ := ValidateWorkspace(ws)
	if _, ok := fileDiags["good.md"]; ok {
		t.Fatal("expected no diagnostics for good.md")
	}
	if _, ok := fileDiags["bad.md"]; !ok {
		t.Fatal("expected diagnostics for bad.md")
	}
}

// TestValidateWorkspace_EmptyWorkspace verifies that an empty workspace produces no diagnostics.
func TestValidateWorkspace_EmptyWorkspace(t *testing.T) {
	ws := makeWorkspace()

	fileDiags, wsDiags := ValidateWorkspace(ws)
	if len(fileDiags) != 0 {
		t.Fatalf("expected no file diagnostics, got %d", len(fileDiags))
	}
	if len(wsDiags) != 0 {
		t.Fatalf("expected no workspace diagnostics, got %d", len(wsDiags))
	}
}

// --- HasErrors ---

// TestHasErrors_WithErrors verifies that HasErrors returns true when at least one error-severity diagnostic exists.
func TestHasErrors_WithErrors(t *testing.T) {
	diags := []ast.Diagnostic{
		{Severity: ast.SeverityWarning},
		{Severity: ast.SeverityError},
	}
	if !HasErrors(diags) {
		t.Fatal("expected true")
	}
}

// TestHasErrors_NoErrors verifies that HasErrors returns false when only warnings and info diagnostics are present.
func TestHasErrors_NoErrors(t *testing.T) {
	diags := []ast.Diagnostic{
		{Severity: ast.SeverityWarning},
		{Severity: ast.SeverityInfo},
	}
	if HasErrors(diags) {
		t.Fatal("expected false")
	}
}

// TestHasErrors_Empty verifies that HasErrors returns false for a nil diagnostic slice.
func TestHasErrors_Empty(t *testing.T) {
	if HasErrors(nil) {
		t.Fatal("expected false for nil")
	}
}

// --- FilterBySeverity ---

// TestFilterBySeverity verifies that filtering by severity returns only diagnostics of the requested level.
func TestFilterBySeverity(t *testing.T) {
	diags := []ast.Diagnostic{
		{Severity: ast.SeverityError, Code: "E001"},
		{Severity: ast.SeverityWarning, Code: "W001"},
		{Severity: ast.SeverityError, Code: "E002"},
		{Severity: ast.SeverityInfo, Code: "I001"},
	}

	errors := FilterBySeverity(diags, ast.SeverityError)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}

	warnings := FilterBySeverity(diags, ast.SeverityWarning)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

// TestFilterBySeverity_Empty verifies that filtering a nil slice returns an empty result.
func TestFilterBySeverity_Empty(t *testing.T) {
	result := FilterBySeverity(nil, ast.SeverityError)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

// --- AllDiagnostics ---

// TestAllDiagnostics verifies that file-level and workspace-level diagnostics are correctly combined.
func TestAllDiagnostics(t *testing.T) {
	fileDiags := map[string][]ast.Diagnostic{
		"a.md": {{Code: "E001"}, {Code: "E002"}},
		"b.md": {{Code: "E003"}},
	}
	wsDiags := []ast.Diagnostic{{Code: "E060"}}

	all := AllDiagnostics(fileDiags, wsDiags)
	if len(all) != 4 {
		t.Fatalf("expected 4, got %d", len(all))
	}
}

// TestAllDiagnostics_Empty verifies that combining nil file and workspace diagnostics returns an empty slice.
func TestAllDiagnostics_Empty(t *testing.T) {
	all := AllDiagnostics(nil, nil)
	if len(all) != 0 {
		t.Fatalf("expected 0, got %d", len(all))
	}
}
