// Package render provides tests for the StreamRender function,
// covering variable substitution, conditional blocks, for loops,
// nested control flow, escape handling, directive stripping, and scope isolation.
package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greyfolk99/siba/pkg/parser"
)

// TestBasicVariableSubstitution verifies that {{var}} references are replaced
// with the corresponding @const values in the rendered output.
func TestBasicVariableSubstitution(t *testing.T) {
	source := `<!-- @const name = "siba" -->
<!-- @const version = "1.0" -->
Hello {{name}} v{{version}}!`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Hello siba v1.0!") {
		t.Errorf("expected variable substitution, got:\n%s", output)
	}
}

// TestIfTrue verifies that an @if block with a truthy condition renders its body.
func TestIfTrue(t *testing.T) {
	source := `<!-- @const enabled = true -->
<!-- @if enabled -->
Feature is ON
<!-- @endif -->`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Feature is ON") {
		t.Errorf("expected @if true body to appear, got:\n%s", output)
	}
}

// TestIfFalse verifies that an @if block with a falsy condition skips its body.
func TestIfFalse(t *testing.T) {
	source := `<!-- @const enabled = false -->
<!-- @if enabled -->
Feature is ON
<!-- @endif -->
After`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "Feature is ON") {
		t.Errorf("expected @if false body to be skipped, got:\n%s", output)
	}
	if !strings.Contains(output, "After") {
		t.Errorf("expected content after @endif to appear, got:\n%s", output)
	}
}

// TestForSimpleArray verifies that @for iterates over a simple string array
// and substitutes the iterator variable in each repetition.
func TestForSimpleArray(t *testing.T) {
	source := `<!-- @const items = ["a", "b", "c"] -->
<!-- @for item in items -->
- {{item}}
<!-- @endfor -->`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"- a", "- b", "- c"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got:\n%s", expected, output)
		}
	}
}

// TestForObjectArray verifies that @for iterates over an array of objects
// and substitutes {{iterator.property}} references correctly.
func TestForObjectArray(t *testing.T) {
	source := `<!-- @const users = [{name: "x"}, {name: "y"}] -->
<!-- @for u in users -->
Name: {{u.name}}
<!-- @endfor -->`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Name: x") {
		t.Errorf("expected 'Name: x' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Name: y") {
		t.Errorf("expected 'Name: y' in output, got:\n%s", output)
	}
}

// TestNestedIf verifies that nested @if blocks work correctly — an outer true
// block containing an inner false block should render the outer content but
// skip the inner content.
func TestNestedIf(t *testing.T) {
	source := `<!-- @const outer = true -->
<!-- @const inner = false -->
<!-- @if outer -->
Outer visible
<!-- @if inner -->
Inner hidden
<!-- @endif -->
Still outer
<!-- @endif -->`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Outer visible") {
		t.Errorf("expected 'Outer visible', got:\n%s", output)
	}
	if strings.Contains(output, "Inner hidden") {
		t.Errorf("expected 'Inner hidden' to be skipped, got:\n%s", output)
	}
	if !strings.Contains(output, "Still outer") {
		t.Errorf("expected 'Still outer', got:\n%s", output)
	}
}

// TestEscapedRef verifies that \{{x}} is rendered as the literal text {{x}}
// without attempting variable substitution.
func TestEscapedRef(t *testing.T) {
	source := `<!-- @const x = "hello" -->
Escaped: \{{x}}
Normal: {{x}}`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Escaped: {{x}}") {
		t.Errorf("expected escaped ref to render as literal {{x}}, got:\n%s", output)
	}
	if !strings.Contains(output, "Normal: hello") {
		t.Errorf("expected normal ref to be substituted, got:\n%s", output)
	}
}

// TestDirectiveStrip verifies that directive lines (@const, @doc, etc.) are
// stripped from the rendered output and do not appear as visible text.
func TestDirectiveStrip(t *testing.T) {
	source := `<!-- @doc my-doc -->
<!-- @const title = "Test" -->
# {{title}}`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "@doc") {
		t.Errorf("expected @doc directive to be stripped, got:\n%s", output)
	}
	if strings.Contains(output, "@const") {
		t.Errorf("expected @const directive to be stripped, got:\n%s", output)
	}
	if !strings.Contains(output, "# Test") {
		t.Errorf("expected heading with substituted title, got:\n%s", output)
	}
}

// TestLetScopeIsolation verifies that @let variables are scoped to their
// respective heading sections — Section A and Section B can each declare
// a @let with the same name but different values without conflict.
func TestLetScopeIsolation(t *testing.T) {
	source := `# Doc

## Section A
<!-- @let mode = "alpha" -->
A: {{mode}}

## Section B
<!-- @let mode = "beta" -->
B: {{mode}}`

	var buf bytes.Buffer
	doc := parser.ParseDocument("test.md", source)
	err := StreamRender(doc, &buf, nil)
	if err != nil {
		t.Fatalf("StreamRender error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "A: alpha") {
		t.Errorf("expected Section A to have mode=alpha, got:\n%s", output)
	}
	if !strings.Contains(output, "B: beta") {
		t.Errorf("expected Section B to have mode=beta, got:\n%s", output)
	}
}

// TestStreamRender_Link verifies that [[alias]] compiles to a markdown link
// using the @import path. [[alice]] with @import alice from ./alice.md → [alice](./alice.md)
func TestStreamRender_Link(t *testing.T) {
	src := `<!-- @import alice from ./alice.md -->

<!-- @doc index -->
# Index

See [[alice]] for details.`
	doc := parser.ParseDocument("index.md", src)

	var buf bytes.Buffer
	if err := StreamRender(doc, &buf, nil); err != nil {
		t.Fatalf("render error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[alice](./alice.md)") {
		t.Errorf("expected [alice](./alice.md) in output, got:\n%s", out)
	}
}

// TestStreamRender_LinkWithSection verifies [[alias#section]] compiles with anchor.
func TestStreamRender_LinkWithSection(t *testing.T) {
	src := `<!-- @import api from ./api.md -->

<!-- @doc index -->
# Index

[[api#endpoints]]`
	doc := parser.ParseDocument("index.md", src)

	var buf bytes.Buffer
	if err := StreamRender(doc, &buf, nil); err != nil {
		t.Fatalf("render error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[api#endpoints](./api.md#endpoints)") {
		t.Errorf("expected [api#endpoints](./api.md#endpoints), got:\n%s", out)
	}
}
