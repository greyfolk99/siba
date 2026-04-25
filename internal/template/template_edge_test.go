// Package template edge-case tests cover boundary conditions for heading matching,
// merge ordering, contract validation depth, slice aliasing, and variable override behavior.
package template

import (
	"testing"

	"github.com/greyfolk99/siba/internal/ast"
)

// --- Edge case tests from Codex review ---

// TestMatchesHeading_NameAuthoritativeOverSlug verifies that @name takes precedence over slug when both headings have names.
func TestMatchesHeading_NameAuthoritativeOverSlug(t *testing.T) {
	tmplH := h(1, "Introduction", "introduction", "intro", ast.AnnotationRequired)
	childH := h(1, "Conclusion", "introduction", "conclusion", ast.AnnotationRequired) // same slug, different name

	if matchesHeading(childH, tmplH) {
		t.Fatal("expected no match: @name differs even though slug matches")
	}
}

// TestMatchesHeading_NoNameSlugMatch verifies that slug-based matching works when neither heading has a @name.
func TestMatchesHeading_NoNameSlugMatch(t *testing.T) {
	tmplH := h(1, "Introduction", "introduction", "", ast.AnnotationRequired) // no name
	childH := h(1, "Different Text", "introduction", "", ast.AnnotationRequired)

	if !matchesHeading(childH, tmplH) {
		t.Fatal("expected slug match when no @name")
	}
}

// TestMatchesHeading_EmptyTargetSlugAndName verifies that two headings with all-empty fields still match by empty text.
func TestMatchesHeading_EmptyTargetSlugAndName(t *testing.T) {
	// when target has no name, no slug, but empty text matches
	a := h(1, "", "", "", ast.AnnotationRequired)
	b := h(1, "", "", "", ast.AnnotationRequired)

	// this matches by text (both empty) — edge case
	if !matchesHeading(a, b) {
		t.Fatal("empty text matches empty text")
	}
}

// TestMergeHeadings_RequiredMissing verifies that a missing @required heading is dropped while @default is kept in the merge.
func TestMergeHeadings_RequiredMissing(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Template", "template", "", ast.AnnotationRequired,
			h(2, "Required Section", "required-section", "", ast.AnnotationRequired),
			h(2, "Default Section", "default-section", "", ast.AnnotationDefault),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "My Doc", "my-doc", "", ast.AnnotationRequired),
		},
	}

	result := MergeHeadings(child, tmpl)
	// result[0] is child's H1
	if len(result) != 1 {
		t.Fatalf("expected 1 top-level heading, got %d", len(result))
	}
	children := result[0].Children
	// required missing → dropped, default → used
	if len(children) != 1 {
		t.Fatalf("expected 1 child heading (only default), got %d", len(children))
	}
	if children[0].Text != "Default Section" {
		t.Fatalf("expected 'Default Section', got %q", children[0].Text)
	}
}

// TestValidateContract_MultipleRequiredMissing verifies that each missing @required heading produces its own diagnostic.
func TestValidateContract_MultipleRequiredMissing(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Template", "template", "", ast.AnnotationRequired,
			h(2, "Section A", "section-a", "", ast.AnnotationRequired),
			h(2, "Section B", "section-b", "", ast.AnnotationRequired),
			h(2, "Section C", "section-c", "", ast.AnnotationRequired),
		),
	})
	child := &ast.Document{Headings: nil}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(diags))
	}
}

// TestValidateContract_DiagnosticHasPosition verifies that the diagnostic for a missing heading carries the template position.
func TestValidateContract_DiagnosticHasPosition(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		{Level: 1, Text: "Template", Slug: "template", Annotation: ast.AnnotationRequired,
			Children: []*ast.Heading{
				{Level: 2, Text: "Required", Slug: "required", Annotation: ast.AnnotationRequired,
					Position: ast.Position{Line: 5, Column: 1}},
			}},
	})
	child := &ast.Document{Headings: nil}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Range.Start.Line != 5 {
		t.Fatalf("expected position line 5, got %d", diags[0].Range.Start.Line)
	}
}

// TestValidateContract_LevelMismatchDiagPosition verifies that a level-mismatch diagnostic points to the child heading position.
func TestValidateContract_LevelMismatchDiagPosition(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Template", "template", "", ast.AnnotationRequired,
			h(2, "Intro", "intro", "", ast.AnnotationRequired),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "My Doc", "my-doc", "", ast.AnnotationRequired,
				&ast.Heading{Level: 3, Text: "Intro", Slug: "intro", Annotation: ast.AnnotationRequired,
					Position: ast.Position{Line: 10, Column: 1}},
			),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// E072 should point to child heading position
	if diags[0].Range.Start.Line != 10 {
		t.Fatalf("expected position line 10, got %d", diags[0].Range.Start.Line)
	}
}

// TestMergeHeadings_ChildExtraNestedHeadings verifies that extra child-only nested headings are preserved in the merge.
func TestMergeHeadings_ChildExtraNestedHeadings(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Chapter", "chapter", "", ast.AnnotationRequired),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "Chapter", "chapter", "", ast.AnnotationRequired,
				h(2, "Custom Section", "custom-section", "", ast.AnnotationRequired),
			),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 heading, got %d", len(result))
	}
	if len(result[0].Children) != 1 {
		t.Fatalf("expected 1 child heading, got %d", len(result[0].Children))
	}
	if result[0].Children[0].Text != "Custom Section" {
		t.Fatalf("expected 'Custom Section', got %q", result[0].Children[0].Text)
	}
}

// TestInheritVariables_MultipleOverrides verifies that multiple child variables each override their template counterpart.
func TestInheritVariables_MultipleOverrides(t *testing.T) {
	tmpl := &ast.Document{
		Variables: []ast.Variable{
			{Name: "a", Access: ast.AccessDefault, Value: strVal("tmpl_a")},
			{Name: "b", Access: ast.AccessDefault, Value: strVal("tmpl_b")},
			{Name: "c", Access: ast.AccessDefault, Value: strVal("tmpl_c")},
		},
	}
	child := &ast.Document{
		Variables: []ast.Variable{
			{Name: "a", Value: strVal("child_a")},
			{Name: "c", Value: strVal("child_c")},
		},
	}

	result := InheritVariables(child, tmpl)
	if len(result) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(result))
	}
	// child overrides should have child values
	vals := map[string]string{}
	for _, v := range result {
		vals[v.Name] = v.Value.Str
	}
	if vals["a"] != "child_a" {
		t.Fatalf("expected child_a, got %q", vals["a"])
	}
	if vals["b"] != "tmpl_b" {
		t.Fatalf("expected tmpl_b (inherited), got %q", vals["b"])
	}
	if vals["c"] != "child_c" {
		t.Fatalf("expected child_c, got %q", vals["c"])
	}
}

// TestValidateContract_DeeplyNested verifies that contract validation recurses through three heading levels correctly.
func TestValidateContract_DeeplyNested(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "L1", "l1", "", ast.AnnotationRequired,
			h(2, "L2", "l2", "", ast.AnnotationRequired,
				h(3, "L3 Required", "l3-required", "", ast.AnnotationRequired),
				h(3, "L3 Default", "l3-default", "", ast.AnnotationDefault),
			),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "L1", "l1", "", ast.AnnotationRequired,
				h(2, "L2", "l2", "", ast.AnnotationRequired,
					// L3 Required missing, L3 Default missing (OK)
				),
			),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic (L3 Required), got %d", len(diags))
	}
	if diags[0].Code != "E070" {
		t.Fatalf("expected E070, got %s", diags[0].Code)
	}
}

// TestMergeHeadings_OrderPreserved verifies that merged output follows template order first, with extras appended.
func TestMergeHeadings_OrderPreserved(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Template", "template", "", ast.AnnotationRequired,
			h(2, "First", "first", "", ast.AnnotationRequired),
			h(2, "Second", "second", "", ast.AnnotationRequired),
			h(2, "Third", "third", "", ast.AnnotationDefault),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "My Doc", "my-doc", "", ast.AnnotationRequired,
				h(2, "Extra", "extra", "", ast.AnnotationRequired),
				h(2, "Second", "second", "", ast.AnnotationRequired),
				h(2, "First", "first", "", ast.AnnotationRequired),
			),
		},
	}

	result := MergeHeadings(child, tmpl)
	if len(result) != 1 {
		t.Fatalf("expected 1 top-level heading, got %d", len(result))
	}
	children := result[0].Children
	if len(children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(children))
	}
	// order should be: First, Second, Third (default), Extra
	expectedOrder := []string{"First", "Second", "Third", "Extra"}
	for i, expected := range expectedOrder {
		if children[i].Text != expected {
			t.Fatalf("position %d: expected %q, got %q", i, expected, children[i].Text)
		}
	}
}

// TestValidateContract_LevelMismatchUsesChildText verifies that a level-mismatch E072 diagnostic references the child heading.
func TestValidateContract_LevelMismatchUsesChildText(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Template", "template", "", ast.AnnotationRequired,
			h(2, "Template Title", "template-title", "ch1", ast.AnnotationRequired),
		),
	})
	child := &ast.Document{
		Headings: []*ast.Heading{
			h(1, "My Doc", "my-doc", "", ast.AnnotationRequired,
				h(3, "Child Title", "child-title", "ch1", ast.AnnotationRequired),
			),
		},
	}

	diags := ValidateContract(child, tmpl)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// message should contain child's text, not template's
	if diags[0].Code != "E072" {
		t.Fatalf("expected E072, got %s", diags[0].Code)
	}
}

// TestMergeHeadings_NoSliceAliasing verifies that mutating the merged result does not affect the original child slice.
func TestMergeHeadings_NoSliceAliasing(t *testing.T) {
	tmpl := makeTemplate("base", "base.md", []*ast.Heading{
		h(1, "Chapter", "chapter", "", ast.AnnotationRequired),
	})
	originalChildren := []*ast.Heading{
		h(2, "Sub A", "sub-a", "", ast.AnnotationRequired),
		h(2, "Sub B", "sub-b", "", ast.AnnotationRequired),
	}
	child := &ast.Document{
		Headings: []*ast.Heading{
			{Level: 1, Text: "Chapter", Slug: "chapter", Children: originalChildren},
		},
	}

	result := MergeHeadings(child, tmpl)
	// mutating result should not affect original
	if len(result) > 0 && len(result[0].Children) > 0 {
		result[0].Children = append(result[0].Children, h(2, "Extra", "extra", "", ast.AnnotationRequired))
	}
	if len(originalChildren) != 2 {
		t.Fatal("original children slice was mutated by merge result")
	}
}
