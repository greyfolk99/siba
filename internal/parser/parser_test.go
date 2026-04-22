package parser

import (
	"testing"

	"github.com/hjseo/siba/internal/ast"
)

func TestParseDirectives(t *testing.T) {
	source := `<!-- @doc test-doc -->
<!-- @const name = "hello" -->
<!-- @let count = 42 -->
# Heading
some text
<!-- @default -->
## Sub Heading`

	directives := ParseDirectives(source)

	if len(directives) != 4 {
		t.Fatalf("expected 4 directives, got %d", len(directives))
	}

	tests := []struct {
		kind ast.DirectiveKind
		args string
	}{
		{ast.DirectiveDoc, "test-doc"},
		{ast.DirectiveConst, `name = "hello"`},
		{ast.DirectiveLet, "count = 42"},
		{ast.DirectiveDefault, ""},
	}

	for i, tt := range tests {
		if directives[i].Kind != tt.kind {
			t.Errorf("directive %d: expected kind %d, got %d", i, tt.kind, directives[i].Kind)
		}
		if directives[i].Args != tt.args {
			t.Errorf("directive %d: expected args %q, got %q", i, tt.args, directives[i].Args)
		}
	}
}

func TestParseHeadings(t *testing.T) {
	source := `# Top
## Section A
### Detail A1
### Detail A2
## Section B`

	headings := ParseHeadings(source)
	if len(headings) != 5 {
		t.Fatalf("expected 5 headings, got %d", len(headings))
	}

	tree := BuildHeadingTree(headings)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root heading, got %d", len(tree))
	}

	root := tree[0]
	if root.Text != "Top" {
		t.Errorf("expected root text 'Top', got %q", root.Text)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}
	if root.Children[0].Text != "Section A" {
		t.Errorf("expected child 0 'Section A', got %q", root.Children[0].Text)
	}
	if len(root.Children[0].Children) != 2 {
		t.Fatalf("expected Section A to have 2 children, got %d", len(root.Children[0].Children))
	}
	if root.Children[1].Text != "Section B" {
		t.Errorf("expected child 1 'Section B', got %q", root.Children[1].Text)
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Authentication & Authorization", "authentication-authorization"},
		{"API 명세", "api-명세"},
		{"Create Payment", "create-payment"},
	}

	for _, tt := range tests {
		got := GenerateSlug(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateSlug(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseDocument(t *testing.T) {
	source := `<!-- @doc payment-api -->
<!-- @const service-name = "payment-api" -->
<!-- @const version = "2.1.0" -->

# Payment API

이 문서는 {{service-name}} v{{version}} 의 API 명세입니다.

## Endpoints

### Authentication
<!-- @let auth-type = "Bearer" -->

## Error Handling
`

	doc := ParseDocument("test.md", source)

	if doc.Name != "payment-api" {
		t.Errorf("expected doc name 'payment-api', got %q", doc.Name)
	}
	if len(doc.Variables) != 3 {
		t.Errorf("expected 3 variables, got %d", len(doc.Variables))
	}
	if len(doc.References) != 2 {
		t.Errorf("expected 2 references, got %d", len(doc.References))
	}
	if len(doc.Headings) != 1 {
		t.Fatalf("expected 1 root heading, got %d", len(doc.Headings))
	}
	if len(doc.Diagnostics) != 0 {
		t.Errorf("expected no diagnostics, got %d", len(doc.Diagnostics))
	}
}

func TestControlBlocks(t *testing.T) {
	source := `<!-- @if env == "production" -->
## Prod Config
<!-- @endif -->

<!-- @for item in items -->
### {{item.name}}
<!-- @endfor -->`

	doc := ParseDocument("test.md", source)

	if len(doc.ControlBlocks) != 2 {
		t.Fatalf("expected 2 control blocks, got %d", len(doc.ControlBlocks))
	}
	if doc.ControlBlocks[0].Kind != ast.DirectiveIf {
		t.Errorf("expected first block to be @if")
	}
	if doc.ControlBlocks[0].Condition != `env == "production"` {
		t.Errorf("expected condition 'env == \"production\"', got %q", doc.ControlBlocks[0].Condition)
	}
	if doc.ControlBlocks[1].Kind != ast.DirectiveFor {
		t.Errorf("expected second block to be @for")
	}
	if doc.ControlBlocks[1].Iterator != "item" {
		t.Errorf("expected iterator 'item', got %q", doc.ControlBlocks[1].Iterator)
	}
	if doc.ControlBlocks[1].Collection != "items" {
		t.Errorf("expected collection 'items', got %q", doc.ControlBlocks[1].Collection)
	}
}

func TestUnmatchedControlBlocks(t *testing.T) {
	source := `<!-- @if env == "production" -->
## Prod Config
`
	doc := ParseDocument("test.md", source)

	if len(doc.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for unclosed @if")
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E012" {
			found = true
		}
	}
	if !found {
		t.Error("expected E012 diagnostic for unclosed @if")
	}
}

func TestDocTemplateExclusive(t *testing.T) {
	source := `<!-- @doc my-doc -->
<!-- @template -->
# Test`

	doc := ParseDocument("test.md", source)

	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E001" {
			found = true
		}
	}
	if !found {
		t.Error("expected E001 diagnostic for @doc + @template")
	}
}
