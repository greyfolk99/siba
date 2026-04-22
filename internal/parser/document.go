package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hjseo/siba/internal/ast"
)

var (
	refRe        = regexp.MustCompile(`(?:\\)?\{\{([^}]+)\}\}`)
	varDeclRe    = regexp.MustCompile(`^(private\s+|protected\s+)?(\w[\w-]*)\s*(?::\s*(.+?))?\s*=\s*(.+)$`)
	varTypeDeclRe = regexp.MustCompile(`^(private\s+|protected\s+)?(\w[\w-]*)\s*:\s*(.+)$`)
	forRe        = regexp.MustCompile(`^(\w[\w-]*)\s+in\s+(.+)$`)
)

// ParseDocument parses a complete document from source
func ParseDocument(path string, source string) *ast.Document {
	doc := &ast.Document{
		Path:   path,
		Source: source,
	}

	// Parse directives
	doc.Directives = ParseDirectives(source)

	// Extract document metadata
	doc.Name, doc.IsTemplate = extractDocMeta(doc.Directives)
	doc.ExtendsName = extractExtends(doc.Directives)
	doc.Imports = extractImports(doc.Directives)

	// Validate @doc + @template exclusivity
	if diag := validateDocTemplateExclusive(doc.Directives); diag != nil {
		doc.Diagnostics = append(doc.Diagnostics, *diag)
	}

	// @template without name is an error
	if doc.IsTemplate && doc.Name == "" {
		doc.Diagnostics = append(doc.Diagnostics, ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E002",
			Message:  "@template requires a name: <!-- @template name -->",
		})
	}

	// Parse headings and build tree
	flatHeadings := ParseHeadings(source)
	attachAnnotationsToHeadings(flatHeadings, doc.Directives)
	attachNamesToHeadings(flatHeadings, doc.Directives)
	doc.Headings = BuildHeadingTree(flatHeadings)

	// Calculate content ranges
	lines := strings.Split(source, "\n")
	calculateContentRanges(flatHeadings, len(lines))

	// Extract variables
	doc.Variables = extractVariables(doc.Directives)

	// Extract references
	doc.References = extractReferences(source)

	// Extract control blocks
	blocks, diags := extractControlBlocks(doc.Directives)
	doc.ControlBlocks = blocks
	doc.Diagnostics = append(doc.Diagnostics, diags...)

	return doc
}

// extractDocMeta returns (name, isTemplate) from @doc or @template directives.
// @template name → name from template, isTemplate=true
// @doc name → name from doc, isTemplate=false
func extractDocMeta(directives []ast.Directive) (string, bool) {
	for _, d := range directives {
		if d.Kind == ast.DirectiveTemplate {
			return strings.TrimSpace(d.Args), true
		}
	}
	for _, d := range directives {
		if d.Kind == ast.DirectiveDoc {
			return strings.TrimSpace(d.Args), false
		}
	}
	return "", false
}

func extractExtends(directives []ast.Directive) string {
	for _, d := range directives {
		if d.Kind == ast.DirectiveExtends {
			return strings.TrimSpace(d.Args)
		}
	}
	return ""
}

// extractImports parses @import directives: <!-- @import alias from path -->
var importRe = regexp.MustCompile(`^(\w[\w-]*)\s+from\s+(.+)$`)

func extractImports(directives []ast.Directive) []ast.Import {
	var imports []ast.Import
	for _, d := range directives {
		if d.Kind != ast.DirectiveImport {
			continue
		}
		matches := importRe.FindStringSubmatch(strings.TrimSpace(d.Args))
		if matches == nil {
			continue
		}
		imports = append(imports, ast.Import{
			Alias:    matches[1],
			Path:     strings.TrimSpace(matches[2]),
			Position: d.Position,
		})
	}
	return imports
}

func validateDocTemplateExclusive(directives []ast.Directive) *ast.Diagnostic {
	hasDoc := false
	hasTemplate := false
	for _, d := range directives {
		if d.Kind == ast.DirectiveDoc {
			hasDoc = true
		}
		if d.Kind == ast.DirectiveTemplate {
			hasTemplate = true
		}
	}
	if hasDoc && hasTemplate {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E001",
			Message:  "@doc and @template are mutually exclusive",
		}
	}
	return nil
}

func extractVariables(directives []ast.Directive) []ast.Variable {
	var vars []ast.Variable

	for _, d := range directives {
		if d.Kind != ast.DirectiveConst && d.Kind != ast.DirectiveLet {
			continue
		}

		mut := ast.MutConst
		if d.Kind == ast.DirectiveLet {
			mut = ast.MutLet
		}

		args := strings.TrimSpace(d.Args)
		// Normalize multiline args (newlines → spaces)
		args = strings.Join(strings.Fields(args), " ")

		// Try full declaration: [access] name [: type] = value
		matches := varDeclRe.FindStringSubmatch(args)
		if matches != nil {
			access := parseAccessLevel(matches[1])
			name := matches[2]
			typeStr := matches[3]
			valStr := matches[4]

			val, err := ParseValue(valStr)
			if err != nil {
				continue
			}

			var typ *ast.TypeExpr
			if typeStr != "" {
				// TODO: parse type expression
				typ = InferType(val)
			} else {
				typ = InferType(val)
			}

			vars = append(vars, ast.Variable{
				Name:       name,
				Type:       typ,
				Value:      &val,
				Mutability: mut,
				Access:     access,
				Position:   d.Position,
			})
			continue
		}

		// Try type-only declaration: [access] name : type
		matches = varTypeDeclRe.FindStringSubmatch(args)
		if matches != nil {
			access := parseAccessLevel(matches[1])
			name := matches[2]

			vars = append(vars, ast.Variable{
				Name:       name,
				Type:       &ast.TypeExpr{Kind: ast.TypeAny}, // TODO: parse type
				Value:      nil,
				Mutability: mut,
				Access:     access,
				Position:   d.Position,
			})
		}
	}

	return vars
}

func parseAccessLevel(s string) ast.AccessLevel {
	s = strings.TrimSpace(s)
	switch s {
	case "private":
		return ast.AccessPrivate
	default:
		return ast.AccessDefault
	}
}

func extractReferences(source string) []ast.Reference {
	var refs []ast.Reference
	lines := strings.Split(source, "\n")

	for i, line := range lines {
		// skip directive lines
		if IsDirectiveLine(line) {
			continue
		}

		matches := refRe.FindAllStringSubmatchIndex(line, -1)
		for _, m := range matches {
			full := line[m[0]:m[1]]
			isEscaped := len(full) > 0 && full[0] == '\\'
			if isEscaped {
				refs = append(refs, ast.Reference{
					Raw:       full,
					IsEscaped: true,
					Position:  ast.Position{Line: i + 1, Column: m[0] + 1},
				})
				continue
			}

			inner := line[m[2]:m[3]]
			ref := parseReferenceInner(inner)
			ref.Raw = full
			ref.Position = ast.Position{Line: i + 1, Column: m[0] + 1}
			refs = append(refs, ref)
		}
	}

	return refs
}

func parseReferenceInner(inner string) ast.Reference {
	ref := ast.Reference{}

	hashIdx := strings.Index(inner, "#")

	if hashIdx >= 0 {
		// Has # — symbol reference
		// Before #: alias or empty (current file)
		// After #: symbol path (may contain / for nesting)
		ref.PathPart = inner[:hashIdx] // alias (empty = current file)
		rest := inner[hashIdx+1:]

		// Check for . after # — only for local property access, not cross-doc
		// Since cross-doc variable access is forbidden, . after # is not allowed
		ref.Section = rest
	} else {
		// No # — local variable or simple name
		dotIdx := strings.LastIndex(inner, ".")
		if dotIdx >= 0 {
			// obj.prop — local object property access
			ref.PathPart = inner[:dotIdx]
			ref.Variable = inner[dotIdx+1:]
		} else {
			// simple name — variable or symbol
			ref.PathPart = inner
		}
	}

	return ref
}

func extractControlBlocks(directives []ast.Directive) ([]ast.ControlBlock, []ast.Diagnostic) {
	var blocks []ast.ControlBlock
	var diags []ast.Diagnostic
	var stack []ast.Directive

	for _, d := range directives {
		switch d.Kind {
		case ast.DirectiveIf:
			stack = append(stack, d)
		case ast.DirectiveEndif:
			if len(stack) == 0 || stack[len(stack)-1].Kind != ast.DirectiveIf {
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E010",
					Message:  "@endif without matching @if",
					Range:    ast.Range{Start: d.Position},
				})
				continue
			}
			opener := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			blocks = append(blocks, ast.ControlBlock{
				Kind:      ast.DirectiveIf,
				Condition: opener.Args,
				Start:     opener.Position,
				End:       d.Position,
			})
		case ast.DirectiveFor:
			stack = append(stack, d)
		case ast.DirectiveEndfor:
			if len(stack) == 0 || stack[len(stack)-1].Kind != ast.DirectiveFor {
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E011",
					Message:  "@endfor without matching @for",
					Range:    ast.Range{Start: d.Position},
				})
				continue
			}
			opener := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			// parse "item in collection"
			matches := forRe.FindStringSubmatch(opener.Args)
			iter, coll := "", ""
			if matches != nil {
				iter = matches[1]
				coll = matches[2]
			}

			blocks = append(blocks, ast.ControlBlock{
				Kind:       ast.DirectiveFor,
				Iterator:   iter,
				Collection: coll,
				Start:      opener.Position,
				End:        d.Position,
			})
		}
	}

	// unclosed blocks
	for _, d := range stack {
		msg := fmt.Sprintf("unclosed @%s", directiveKindName(d.Kind))
		diags = append(diags, ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E012",
			Message:  msg,
			Range:    ast.Range{Start: d.Position},
		})
	}

	return blocks, diags
}

func directiveKindName(k ast.DirectiveKind) string {
	switch k {
	case ast.DirectiveIf:
		return "if"
	case ast.DirectiveFor:
		return "for"
	default:
		return "unknown"
	}
}

func attachAnnotationsToHeadings(headings []*ast.Heading, directives []ast.Directive) {
	for _, h := range headings {
		h.Annotation = ExtractAnnotation(directives, h.Position.Line)
	}
}

func attachNamesToHeadings(headings []*ast.Heading, directives []ast.Directive) {
	for _, h := range headings {
		for _, d := range directives {
			if d.Kind == ast.DirectiveName && d.Position.Line == h.Position.Line-1 {
				h.Name = strings.TrimSpace(d.Args)
				break
			}
			// also check 2 lines above (annotation + name)
			if d.Kind == ast.DirectiveName && d.Position.Line == h.Position.Line-2 {
				h.Name = strings.TrimSpace(d.Args)
				break
			}
		}
	}
}

func calculateContentRanges(headings []*ast.Heading, totalLines int) {
	for i, h := range headings {
		startLine := h.Position.Line + 1
		var endLine int
		if i+1 < len(headings) {
			// content ends at the line before the next heading (or its directives)
			endLine = headings[i+1].Position.Line - 1
		} else {
			endLine = totalLines
		}
		h.Content = ast.Range{
			Start: ast.Position{Line: startLine, Column: 1},
			End:   ast.Position{Line: endLine, Column: 1},
		}
	}
}
