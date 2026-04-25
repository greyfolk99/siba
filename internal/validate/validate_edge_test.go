// Package validate edge-case tests cover deterministic diagnostic ordering,
// combined error scenarios, escaped references, template satisfaction, hint
// filtering, empty documents, and file-path propagation on diagnostics.
package validate

import (
	"testing"

	"github.com/greyfolk99/siba/internal/ast"
)

// --- Edge case tests from Codex review ---

// TestAllDiagnostics_DeterministicOrder verifies that AllDiagnostics returns file diagnostics sorted by path for stable output.
func TestAllDiagnostics_DeterministicOrder(t *testing.T) {
	fileDiags := map[string][]ast.Diagnostic{
		"c.md": {{Code: "E003"}},
		"a.md": {{Code: "E001"}},
		"b.md": {{Code: "E002"}},
	}
	wsDiags := []ast.Diagnostic{{Code: "E060"}}

	// run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		all := AllDiagnostics(fileDiags, wsDiags)
		if len(all) != 4 {
			t.Fatalf("expected 4, got %d", len(all))
		}
		// should be sorted by file path: a.md, b.md, c.md, then ws
		if all[0].Code != "E001" {
			t.Fatalf("iteration %d: expected E001 first, got %s", i, all[0].Code)
		}
		if all[1].Code != "E002" {
			t.Fatalf("iteration %d: expected E002 second, got %s", i, all[1].Code)
		}
		if all[2].Code != "E003" {
			t.Fatalf("iteration %d: expected E003 third, got %s", i, all[2].Code)
		}
		if all[3].Code != "E060" {
			t.Fatalf("iteration %d: expected E060 last, got %s", i, all[3].Code)
		}
	}
}

// TestValidateDocument_ScopeAndRefErrors verifies that duplicate-const and unresolved-ref errors are reported together.
func TestValidateDocument_ScopeAndRefErrors(t *testing.T) {
	doc := &ast.Document{
		Path:   "test.md",
		Source: "line1\nline2\nline3\n",
		Variables: []ast.Variable{
			{Name: "x", Mutability: ast.MutConst, Value: strVal("a"), Position: ast.Position{Line: 1}},
			{Name: "x", Mutability: ast.MutConst, Value: strVal("b"), Position: ast.Position{Line: 2}},
		},
		References: []ast.Reference{
			{Raw: "missing", PathPart: "missing", Position: ast.Position{Line: 3}},
		},
	}

	diags := ValidateDocument(doc, nil)
	codes := map[string]bool{}
	for _, d := range diags {
		codes[d.Code] = true
	}
	if !codes["E020"] {
		t.Fatal("expected E020 for duplicate const")
	}
	if !codes["E050"] {
		t.Fatal("expected E050 for unresolved ref")
	}
}

// TestValidateDocument_EscapedRefNoError verifies that an escaped reference is skipped and produces no diagnostic.
func TestValidateDocument_EscapedRefNoError(t *testing.T) {
	doc := &ast.Document{
		Path:   "test.md",
		Source: "# Hello\n",
		References: []ast.Reference{
			{Raw: "\\{{escaped}}", PathPart: "escaped", IsEscaped: true, Position: ast.Position{Line: 1}},
		},
	}

	diags := ValidateDocument(doc, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for escaped ref, got %d", len(diags))
	}
}

// TestValidateDocument_TemplateContractSatisfied verifies that a child satisfying all template requirements produces no template errors.
func TestValidateDocument_TemplateContractSatisfied(t *testing.T) {
	tmpl := &ast.Document{
		Name:       "base",
		Path:       "base.md",
		IsTemplate: true,
		Headings: []*ast.Heading{
			{Level: 1, Text: "Introduction", Slug: "introduction", Annotation: ast.AnnotationRequired},
		},
	}
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		Source:      "# Introduction\n",
		ExtendsName: "base",
		Headings: []*ast.Heading{
			{Level: 1, Text: "Introduction", Slug: "introduction", Annotation: ast.AnnotationRequired},
		},
	}
	ws := makeWorkspace(tmpl, child)

	diags := ValidateDocument(child, ws)
	for _, d := range diags {
		if d.Code == "E070" || d.Code == "E071" || d.Code == "E072" {
			t.Fatalf("unexpected template diagnostic: %s: %s", d.Code, d.Message)
		}
	}
}

// TestValidateWorkspace_TemplateChild verifies end-to-end workspace validation with a template and conforming child document.
func TestValidateWorkspace_TemplateChild(t *testing.T) {
	tmpl := &ast.Document{
		Name:       "base",
		Path:       "base.md",
		Source:     "# Template\n",
		IsTemplate: true,
		Headings: []*ast.Heading{
			{Level: 1, Text: "Section", Slug: "section", Annotation: ast.AnnotationRequired},
		},
	}
	child := &ast.Document{
		Name:        "child",
		Path:        "child.md",
		Source:      "# Section\n",
		ExtendsName: "base",
		Headings: []*ast.Heading{
			{Level: 1, Text: "Section", Slug: "section", Annotation: ast.AnnotationRequired},
		},
	}
	ws := makeWorkspace(tmpl, child)

	fileDiags, wsDiags := ValidateWorkspace(ws)
	// no file-level template errors
	if diags, ok := fileDiags["child.md"]; ok {
		for _, d := range diags {
			if d.Code == "E070" {
				t.Fatalf("unexpected E070: %s", d.Message)
			}
		}
	}
	if len(wsDiags) != 0 {
		t.Fatalf("expected no ws diagnostics, got %d", len(wsDiags))
	}
}

// TestHasErrors_OnlyHints verifies that HasErrors returns false when only hint-severity diagnostics are present.
func TestHasErrors_OnlyHints(t *testing.T) {
	diags := []ast.Diagnostic{
		{Severity: ast.SeverityHint},
	}
	if HasErrors(diags) {
		t.Fatal("expected false for hints only")
	}
}

// TestFilterBySeverity_Hints verifies that filtering by hint severity correctly isolates hint diagnostics.
func TestFilterBySeverity_Hints(t *testing.T) {
	diags := []ast.Diagnostic{
		{Severity: ast.SeverityError},
		{Severity: ast.SeverityHint, Code: "H001"},
		{Severity: ast.SeverityHint, Code: "H002"},
	}

	hints := FilterBySeverity(diags, ast.SeverityHint)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}
}

// TestAllDiagnostics_OnlyWsDiags verifies that AllDiagnostics works correctly with nil file diagnostics.
func TestAllDiagnostics_OnlyWsDiags(t *testing.T) {
	wsDiags := []ast.Diagnostic{{Code: "E060"}, {Code: "E061"}}
	all := AllDiagnostics(nil, wsDiags)
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

// TestAllDiagnostics_OnlyFileDiags verifies that AllDiagnostics works correctly with nil workspace diagnostics.
func TestAllDiagnostics_OnlyFileDiags(t *testing.T) {
	fileDiags := map[string][]ast.Diagnostic{
		"a.md": {{Code: "E001"}},
	}
	all := AllDiagnostics(fileDiags, nil)
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}
}

// TestValidateDocument_EmptySource verifies that a document with an empty source string produces no diagnostics.
func TestValidateDocument_EmptySource(t *testing.T) {
	doc := &ast.Document{
		Path:   "empty.md",
		Source: "",
	}

	diags := ValidateDocument(doc, nil)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for empty doc, got %d", len(diags))
	}
}

// TestValidateDocument_AllDiagsHaveFile verifies that every emitted diagnostic carries the correct file path.
func TestValidateDocument_AllDiagsHaveFile(t *testing.T) {
	doc := &ast.Document{
		Path:   "myfile.md",
		Source: "line1\nline2\n",
		Variables: []ast.Variable{
			{Name: "a", Mutability: ast.MutConst, Value: strVal("x"), Position: ast.Position{Line: 1}},
			{Name: "a", Mutability: ast.MutConst, Value: strVal("y"), Position: ast.Position{Line: 2}},
		},
		References: []ast.Reference{
			{Raw: "missing", PathPart: "missing", Position: ast.Position{Line: 1}},
		},
	}

	diags := ValidateDocument(doc, nil)
	for _, d := range diags {
		if d.File != "myfile.md" {
			t.Fatalf("expected file 'myfile.md', got %q (code %s)", d.File, d.Code)
		}
	}
}
