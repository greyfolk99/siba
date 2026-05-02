// Package parser provides tests for the siba markdown directive parser,
// covering directive extraction, heading tree construction, slug generation,
// full document parsing, control block handling, and diagnostic validation.
package parser

import (
	"testing"

	"github.com/greyfolk99/siba/pkg/ast"
)

// TestParseDirectives verifies that ParseDirectives correctly extracts all directive kinds
// (@doc, @const, @let, @default) and their arguments from raw markdown source.
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

// TestParseHeadings verifies that headings are parsed and assembled into a correct
// parent-child tree structure via BuildHeadingTree.
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

// TestGenerateSlug verifies that GenerateSlug lowercases text, strips special characters,
// and joins words with hyphens to produce URL-safe slugs.
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

// TestParseDocument verifies that ParseDocument produces a complete Document with the
// correct name, variables, template references, heading tree, and zero diagnostics.
func TestParseDocument(t *testing.T) {
	source := `<!-- @const service-name = "payment-api" -->
<!-- @const version = "2.1.0" -->
<!-- @doc PaymentApi -->

# Payment API

이 문서는 {{service-name}} v{{version}} 의 API 명세입니다.

## Endpoints

### Authentication
<!-- @let auth-type = "Bearer" -->

## Error Handling
`

	doc := ParseDocument("test.md", source)

	if doc.Name != "PaymentApi" {
		t.Errorf("expected doc name 'PaymentApi', got %q", doc.Name)
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

// TestControlBlocks verifies that @if/@endif and @for/@endfor control blocks are
// parsed with the correct kind, condition, iterator, and collection fields.
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

// TestUnmatchedControlBlocks verifies that an unclosed @if block without a matching
// @endif produces an E012 diagnostic.
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

// TestDocTemplateExclusive verifies that using both @doc and @template in the same
// file produces an E001 diagnostic, since they are mutually exclusive.
func TestDocTemplateExclusive(t *testing.T) {
	source := `<!-- @doc my-doc -->
<!-- @template my-tmpl -->
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

// TestTemplateRequiresName verifies that a @template directive without a name argument
// produces an E002 diagnostic.
func TestTemplateRequiresName(t *testing.T) {
	source := `<!-- @template -->
# Test`

	doc := ParseDocument("test.md", source)

	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E002" {
			found = true
		}
	}
	if !found {
		t.Error("expected E002 diagnostic for @template without name")
	}
}

// TestTemplateWithName verifies that a valid @template directive with a name sets
// the document Name and IsTemplate fields correctly without producing diagnostics.
func TestTemplateWithName(t *testing.T) {
	source := `<!-- @template ApiSpec -->
# API Spec
## Endpoints
## Error Handling`

	doc := ParseDocument("tmpl.md", source)

	if doc.Name != "ApiSpec" {
		t.Errorf("expected Name='ApiSpec', got %q", doc.Name)
	}
	if !doc.IsTemplate {
		t.Error("expected IsTemplate=true")
	}
	if len(doc.Diagnostics) > 0 {
		t.Errorf("expected no diagnostics, got %v", doc.Diagnostics)
	}
}

// TestParseImport verifies that an @import directive with "alias from path" syntax
// is correctly parsed into an Import with the expected alias and path fields.
func TestParseImport(t *testing.T) {
	source := `<!-- @import utils from ./shared/utils -->
# Main`

	doc := ParseDocument("test.md", source)

	if len(doc.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(doc.Imports))
	}
	imp := doc.Imports[0]
	if imp.Alias != "utils" {
		t.Errorf("expected alias 'utils', got %q", imp.Alias)
	}
	if imp.Path != "./shared/utils" {
		t.Errorf("expected path './shared/utils', got %q", imp.Path)
	}
}

// TestMultilineConst verifies that a @const directive spanning multiple lines
// (e.g., an array literal split across lines inside <!-- ... -->) is parsed
// into a single variable with the correct array value.
func TestMultilineConst(t *testing.T) {
	source := `<!-- @const items = [
  "alpha",
  "beta",
  "gamma"
] -->
# Test`

	doc := ParseDocument("test.md", source)

	var found *ast.Variable
	for i := range doc.Variables {
		if doc.Variables[i].Name == "items" {
			found = &doc.Variables[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected variable 'items' to be parsed")
	}
	if found.Value == nil {
		t.Fatal("expected 'items' to have a value")
	}
	if found.Value.Kind != ast.TypeArray {
		t.Errorf("expected array type, got %d", found.Value.Kind)
	}
	if len(found.Value.Array) != 3 {
		t.Errorf("expected 3 elements, got %d", len(found.Value.Array))
	}
}

// TestParsePrivateConst verifies that a @const directive with the "private"
// access modifier sets the variable's Access field to AccessPrivate.
func TestParsePrivateConst(t *testing.T) {
	source := `<!-- @const private secret = "hidden" -->
# Test`

	doc := ParseDocument("test.md", source)

	var found *ast.Variable
	for i := range doc.Variables {
		if doc.Variables[i].Name == "secret" {
			found = &doc.Variables[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected variable 'secret' to be parsed")
	}
	if found.Access != ast.AccessPrivate {
		t.Errorf("expected AccessPrivate, got %d", found.Access)
	}
	if found.Value == nil || found.Value.Str != "hidden" {
		t.Errorf("expected value 'hidden', got %v", found.Value)
	}
}

// TestParseHashRef verifies that a {{#section}} reference is parsed with an
// empty PathPart and the section name in the Section field.
func TestParseHashRef(t *testing.T) {
	source := `# Main
Content with {{#section}} reference`

	doc := ParseDocument("test.md", source)

	found := false
	for _, ref := range doc.References {
		if ref.Section == "section" && ref.PathPart == "" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a reference with Section='section' and empty PathPart, got refs: %+v", doc.References)
	}
}

// TestParseAliasHashRef verifies that a {{alias#symbol}} reference is parsed
// with the alias in PathPart and the symbol in the Section field.
func TestParseAliasHashRef(t *testing.T) {
	source := `<!-- @import utils from ./utils -->
# Main
See {{utils#helper}} for details`

	doc := ParseDocument("test.md", source)

	found := false
	for _, ref := range doc.References {
		if ref.PathPart == "utils" && ref.Section == "helper" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a reference with PathPart='utils' and Section='helper', got refs: %+v", doc.References)
	}
}

// TestParseDocuments_Multi verifies that a single file containing multiple
// @template declarations is split into separate Document instances, each with
// the correct name and IsTemplate flag set.
func TestParseDocuments_Multi(t *testing.T) {
	source := `<!-- @template api-spec -->
# API Spec
## Endpoints

<!-- @template error-spec -->
# Error Spec
## Error Codes`

	docs := ParseDocuments("multi.md", source)

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}
	if docs[0].Name != "api-spec" {
		t.Errorf("expected first doc name 'api-spec', got %q", docs[0].Name)
	}
	if !docs[0].IsTemplate {
		t.Error("expected first doc IsTemplate=true")
	}
	if docs[1].Name != "error-spec" {
		t.Errorf("expected second doc name 'error-spec', got %q", docs[1].Name)
	}
	if !docs[1].IsTemplate {
		t.Error("expected second doc IsTemplate=true")
	}
}

// TestParseDoc_ExtendsModifier verifies that "@doc <name> extends <parent>" sets
// both Name and ExtendsName via the modifier syntax (replaces separate @extends directive).
func TestParseDoc_ExtendsModifier(t *testing.T) {
	source := `<!-- @doc alice extends employee -->
# Alice`
	doc := ParseDocument("test.md", source)
	if doc.Name != "alice" {
		t.Errorf("expected Name='alice', got %q", doc.Name)
	}
	if doc.ExtendsName != "employee" {
		t.Errorf("expected ExtendsName='employee', got %q", doc.ExtendsName)
	}
	if doc.IsTemplate {
		t.Error("expected IsTemplate=false")
	}
	for _, d := range doc.Diagnostics {
		if d.Severity == ast.SeverityError {
			t.Errorf("unexpected error diagnostic: %v", d)
		}
	}
}

// TestParseTemplate_ExtendsModifier verifies @template name extends parent via modifier.
func TestParseTemplate_ExtendsModifier(t *testing.T) {
	source := `<!-- @template senior-employee extends employee -->
# Senior Employee
## Profile`
	doc := ParseDocument("test.md", source)
	if doc.Name != "senior-employee" {
		t.Errorf("expected Name='senior-employee', got %q", doc.Name)
	}
	if doc.ExtendsName != "employee" {
		t.Errorf("expected ExtendsName='employee', got %q", doc.ExtendsName)
	}
	if !doc.IsTemplate {
		t.Error("expected IsTemplate=true")
	}
}

// TestParseDoc_ExtendsAliasModifier verifies "@doc x extends alias#parent" works.
func TestParseDoc_ExtendsAliasModifier(t *testing.T) {
	source := `<!-- @import tmpl from ./t.md -->
<!-- @doc x extends tmpl#project -->
# X`
	doc := ParseDocument("test.md", source)
	if doc.ExtendsName != "tmpl#project" {
		t.Errorf("expected ExtendsName='tmpl#project', got %q", doc.ExtendsName)
	}
}

// TestParseDoc_ExtendsDirectiveDeprecated verifies the legacy "@extends X" directive
// still works but produces an I001 deprecation Info diagnostic.
func TestParseDoc_ExtendsDirectiveDeprecated(t *testing.T) {
	source := `<!-- @doc alice -->
<!-- @extends employee -->
# Alice`
	doc := ParseDocument("test.md", source)
	if doc.ExtendsName != "employee" {
		t.Errorf("expected ExtendsName='employee', got %q", doc.ExtendsName)
	}
	hasInfo := false
	for _, d := range doc.Diagnostics {
		if d.Code == "I001" && d.Severity == ast.SeverityInfo {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Error("expected I001 SeverityInfo diagnostic for deprecated @extends directive")
	}
}

// TestParseDoc_ExtendsConflict verifies E075 when modifier and directive disagree.
func TestParseDoc_ExtendsConflict(t *testing.T) {
	source := `<!-- @doc alice extends employee -->
<!-- @extends manager -->
# Alice`
	doc := ParseDocument("test.md", source)
	hasErr := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E075" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Error("expected E075 for conflicting extends declarations")
	}
}

// TestPrelude_LetAfterHeading verifies that a file-level @let after the body start
// (first @doc/@template/heading) raises E007.
func TestPrelude_LetAfterHeading(t *testing.T) {
	source := `<!-- @doc x -->
<!-- @let bad = 1 -->
# X
text`
	doc := ParseDocument("test.md", source)
	hasErr := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E007" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Error("expected E007 for file-level @let after body start")
	}
}

// TestPrelude_ConstBeforeDoc_OK verifies that @const before @doc is fine.
func TestPrelude_ConstBeforeDoc_OK(t *testing.T) {
	source := `<!-- @import a from ./a.md -->
<!-- @const greeting = "hi" -->
<!-- @doc x -->
# X`
	doc := ParseDocument("test.md", source)
	for _, d := range doc.Diagnostics {
		if d.Code == "E007" {
			t.Errorf("unexpected E007: %v", d)
		}
	}
}

// TestPrelude_LetInsideHeadingScope_OK verifies that @let inside a heading scope
// (section-scope, i.e., line > firstHeadingLine) is allowed and does NOT raise E007.
func TestPrelude_LetInsideHeadingScope_OK(t *testing.T) {
	source := `<!-- @doc x -->
# X
## Section
<!-- @let inner = 1 -->
{{inner}}`
	doc := ParseDocument("test.md", source)
	for _, d := range doc.Diagnostics {
		if d.Code == "E007" {
			t.Errorf("unexpected E007 for section-scope @let: %v", d)
		}
	}
}

// TestParseLink_BasicAlias verifies that [[alias]] is parsed as a link reference.
func TestParseLink_BasicAlias(t *testing.T) {
	source := `<!-- @import alice from ./alice.md -->

<!-- @doc index -->
# Index

See [[alice]] for details.`
	doc := ParseDocument("test.md", source)
	var found *ast.Reference
	for i := range doc.References {
		if doc.References[i].Raw == "[[alice]]" {
			found = &doc.References[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected a [[alice]] reference, got refs: %+v", doc.References)
	}
	if !found.IsLink {
		t.Error("expected IsLink=true")
	}
	if found.PathPart != "alice" {
		t.Errorf("expected PathPart='alice', got %q", found.PathPart)
	}
}

// TestParseEmbed_RawPathRejected verifies that {{some/path}} (raw path) raises E023.
func TestParseEmbed_RawPathRejected(t *testing.T) {
	source := `<!-- @doc index -->
# Index
{{some/path}}`
	doc := ParseDocument("test.md", source)
	hasErr := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E023" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Error("expected E023 for raw path in {{}}")
	}
}

// TestParseLink_RawPathRejected verifies that [[./some/path]] raises E023.
func TestParseLink_RawPathRejected(t *testing.T) {
	source := `<!-- @doc index -->
# Index
[[./some/path]]`
	doc := ParseDocument("test.md", source)
	hasErr := false
	for _, d := range doc.Diagnostics {
		if d.Code == "E023" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Error("expected E023 for raw path in [[]]")
	}
}

// TestPascalCase_DocLowercase verifies that lowercase doc/template names
// produce I002 SeverityInfo (not error).
func TestPascalCase_DocLowercase(t *testing.T) {
	source := `<!-- @doc alice -->
# Alice`
	doc := ParseDocument("test.md", source)
	hasInfo := false
	hasErr := false
	for _, d := range doc.Diagnostics {
		if d.Code == "I002" && d.Severity == ast.SeverityInfo {
			hasInfo = true
		}
		if d.Severity == ast.SeverityError {
			hasErr = true
		}
	}
	if !hasInfo {
		t.Error("expected I002 Info diagnostic for lowercase @doc name")
	}
	if hasErr {
		t.Error("expected no error diagnostics for lowercase @doc name")
	}
}

// TestPascalCase_DocPascal_OK verifies that a PascalCase name produces no diagnostic.
func TestPascalCase_DocPascal_OK(t *testing.T) {
	source := `<!-- @doc Alice -->
# Alice`
	doc := ParseDocument("test.md", source)
	for _, d := range doc.Diagnostics {
		if d.Code == "I002" {
			t.Errorf("unexpected I002 for PascalCase name: %v", d)
		}
	}
}
