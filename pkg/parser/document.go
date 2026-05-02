package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
)

var (
	refRe         = regexp.MustCompile(`(?:\\)?\{\{([^}]+)\}\}`)
	linkRe        = regexp.MustCompile(`(?:\\)?\[\[([^\]]+)\]\]`)
	varDeclRe     = regexp.MustCompile(`^(private\s+)?(\w[\w-]*)\s*(?::\s*(.+?))?\s*=\s*(.+)$`)
	varTypeDeclRe = regexp.MustCompile(`^(private\s+)?(\w[\w-]*)\s*:\s*(.+)$`)
	forRe         = regexp.MustCompile(`^(\w[\w-]*)\s+in\s+(.+)$`)
	aliasRe       = regexp.MustCompile(`^[A-Za-z_][\w-]*$`)
)

// ParseDocuments parses a file that may contain multiple @template/@doc declarations.
// Returns one Document per declaration. If none found, returns a single unnamed Document.
func ParseDocuments(path string, source string) []*ast.Document {
	directives := ParseDirectives(source)
	lines := strings.Split(source, "\n")

	// Find all @template/@doc declaration positions
	type declInfo struct {
		line       int
		name       string
		isTemplate bool
	}
	var decls []declInfo
	for _, d := range directives {
		if d.Kind == ast.DirectiveTemplate {
			nm, _ := parseDocArgs(d.Args)
			decls = append(decls, declInfo{line: d.Position.Line, name: nm, isTemplate: true})
		} else if d.Kind == ast.DirectiveDoc {
			nm, _ := parseDocArgs(d.Args)
			decls = append(decls, declInfo{line: d.Position.Line, name: nm, isTemplate: false})
		}
	}

	// If 0 or 1 declaration, use single ParseDocument
	if len(decls) <= 1 {
		doc := ParseDocument(path, source)
		return []*ast.Document{doc}
	}

	// Multiple declarations — split source into segments
	var docs []*ast.Document
	for i, decl := range decls {
		startLine := decl.line - 1 // 0-based
		var endLine int
		if i+1 < len(decls) {
			endLine = decls[i+1].line - 2 // line before next declaration's directive
			// scan backwards to include any directives above the next declaration
			for endLine > startLine && strings.TrimSpace(lines[endLine]) == "" {
				endLine--
			}
			endLine++ // include the last non-empty line
		} else {
			endLine = len(lines)
		}

		if startLine < 0 {
			startLine = 0
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}

		// Include file-level directives (@import, @const before first decl) for first segment
		segmentStart := startLine
		if i == 0 {
			segmentStart = 0
		}

		segment := strings.Join(lines[segmentStart:endLine], "\n")
		doc := ParseDocument(path, segment)
		// override name/template from the declaration
		doc.Name = decl.name
		doc.IsTemplate = decl.isTemplate
		docs = append(docs, doc)
	}

	return docs
}

// ParseDocument parses a complete document from source
func ParseDocument(path string, source string) *ast.Document {
	doc := &ast.Document{
		Path:   path,
		Source: source,
	}

	// Parse directives
	doc.Directives = ParseDirectives(source)

	// Extract document metadata (name + isTemplate + modifier-extends)
	modifierExtends := ""
	doc.Name, doc.IsTemplate, modifierExtends = extractDocMeta(doc.Directives)
	directiveExtends := extractExtends(doc.Directives)
	doc.ExtendsName = mergeExtends(modifierExtends, directiveExtends)
	if diag := validateExtendsConflict(modifierExtends, directiveExtends, doc.Directives); diag != nil {
		doc.Diagnostics = append(doc.Diagnostics, *diag)
	}
	if diag := validateExtendsDeprecation(directiveExtends, doc.Directives); diag != nil {
		doc.Diagnostics = append(doc.Diagnostics, *diag)
	}
	doc.Imports = extractImports(doc.Directives)

	// Validate @doc + @template exclusivity
	if diag := validateDocTemplateExclusive(doc.Directives); diag != nil {
		doc.Diagnostics = append(doc.Diagnostics, *diag)
	}

	// Validate @import comes before @doc/@template
	if diag := validateImportOrder(doc.Directives); diag != nil {
		doc.Diagnostics = append(doc.Diagnostics, *diag)
	}

	// Template restrictions: E005 (control flow outside @default), E006 (heading inside control flow)
	if doc.IsTemplate {
		doc.Diagnostics = append(doc.Diagnostics, validateTemplateRestrictions(source, doc.Directives, doc.ControlBlocks)...)
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

	// E007: file-prelude rule — @import/@const/@let must be in the prelude
	doc.Diagnostics = append(doc.Diagnostics, validateFilePrelude(doc.Directives, flatHeadings)...)

	// Calculate content ranges
	lines := strings.Split(source, "\n")
	calculateContentRanges(flatHeadings, len(lines))

	// Extract variables
	doc.Variables = extractVariables(doc.Directives)

	// Extract references (both {{}} embed and [[]] link)
	doc.References = extractReferences(source)
	doc.Diagnostics = append(doc.Diagnostics, validateRefsAreAliasOnly(doc.References, doc.Imports)...)

	// Extract control blocks
	blocks, diags := extractControlBlocks(doc.Directives)
	doc.ControlBlocks = blocks
	doc.Diagnostics = append(doc.Diagnostics, diags...)

	return doc
}

// extractDocMeta returns (name, isTemplate, extendsName) from @doc or @template
// directives. Supports modifier syntax: "@doc Name extends Parent" or
// "@template Name extends Parent". Modifier extends suffix is optional.
func extractDocMeta(directives []ast.Directive) (string, bool, string) {
	for _, d := range directives {
		if d.Kind == ast.DirectiveTemplate {
			name, parent := parseDocArgs(d.Args)
			return name, true, parent
		}
	}
	for _, d := range directives {
		if d.Kind == ast.DirectiveDoc {
			name, parent := parseDocArgs(d.Args)
			return name, false, parent
		}
	}
	return "", false, ""
}

// parseDocArgs splits "<name>" or "<name> extends <parent>" into (name, parent).
// Whitespace-tolerant. If "extends" keyword is absent or malformed, parent is "".
func parseDocArgs(args string) (name, parent string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}
	fields := strings.Fields(args)
	if len(fields) >= 3 && fields[1] == "extends" {
		return fields[0], strings.Join(fields[2:], " ")
	}
	return fields[0], ""
}

// mergeExtends prefers the modifier syntax over the legacy @extends directive.
func mergeExtends(modifier, directive string) string {
	if modifier != "" {
		return modifier
	}
	return directive
}

// validateExtendsConflict returns E075 when modifier and directive declare
// different parents in the same document.
func validateExtendsConflict(modifier, directive string, directives []ast.Directive) *ast.Diagnostic {
	if modifier == "" || directive == "" || modifier == directive {
		return nil
	}
	pos := ast.Position{}
	for _, d := range directives {
		if d.Kind == ast.DirectiveExtends {
			pos = d.Position
			break
		}
	}
	return &ast.Diagnostic{
		Severity: ast.SeverityError,
		Code:     "E075",
		Message:  fmt.Sprintf("conflicting extends declarations: modifier %q vs directive %q", modifier, directive),
		Range:    ast.Range{Start: pos, End: pos},
	}
}

// validateExtendsDeprecation emits I001 when the legacy @extends directive is used.
func validateExtendsDeprecation(directive string, directives []ast.Directive) *ast.Diagnostic {
	if directive == "" {
		return nil
	}
	pos := ast.Position{}
	for _, d := range directives {
		if d.Kind == ast.DirectiveExtends {
			pos = d.Position
			break
		}
	}
	return &ast.Diagnostic{
		Severity: ast.SeverityInfo,
		Code:     "I001",
		Message:  "@extends directive is deprecated; use modifier syntax: @doc <name> extends <parent>",
		Range:    ast.Range{Start: pos, End: pos},
	}
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

// validateTemplateRestrictions checks E005 and E006 for template files.
// E005: @if/@for outside @default heading body
// E006: heading (# line) inside @if/@for block
func validateTemplateRestrictions(source string, directives []ast.Directive, blocks []ast.ControlBlock) []ast.Diagnostic {
	var diags []ast.Diagnostic
	lines := strings.Split(source, "\n")

	// Find @default heading ranges
	type defaultRange struct {
		start, end int
	}
	var defaults []defaultRange
	for i, d := range directives {
		if d.Kind == ast.DirectiveDefault {
			// Find the heading this @default annotates (next line)
			headingLine := d.Position.Line + 1
			// Find end of this heading's content
			endLine := len(lines)
			for j := i + 1; j < len(directives); j++ {
				if directives[j].Kind == ast.DirectiveDefault || directives[j].Kind == ast.DirectiveDoc || directives[j].Kind == ast.DirectiveTemplate {
					endLine = directives[j].Position.Line - 1
					break
				}
			}
			// Also check for next heading in source
			for li := headingLine; li < len(lines); li++ {
				trimmed := strings.TrimSpace(lines[li])
				if li > headingLine-1 && strings.HasPrefix(trimmed, "#") {
					endLine = li
					break
				}
			}
			defaults = append(defaults, defaultRange{start: headingLine, end: endLine})
		}
	}

	isInDefault := func(line int) bool {
		for _, dr := range defaults {
			if line >= dr.start && line <= dr.end {
				return true
			}
		}
		return false
	}

	// E005: control blocks outside @default
	for _, cb := range blocks {
		if !isInDefault(cb.Start.Line) {
			diags = append(diags, ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E005",
				Message:  "control flow (@if/@for) not allowed in template outside @default section",
				Range:    ast.Range{Start: cb.Start, End: cb.Start},
			})
		}
	}

	// E006: heading inside control block
	for _, cb := range blocks {
		for li := cb.Start.Line; li < cb.End.Line && li <= len(lines); li++ {
			trimmed := strings.TrimSpace(lines[li-1])
			if strings.HasPrefix(trimmed, "#") {
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E006",
					Message:  "heading inside control flow block is not allowed in template",
					Range:    ast.Range{Start: ast.Position{Line: li}, End: ast.Position{Line: li}},
				})
			}
		}
	}

	return diags
}

func validateImportOrder(directives []ast.Directive) *ast.Diagnostic {
	docTemplateSeen := false
	for _, d := range directives {
		if d.Kind == ast.DirectiveDoc || d.Kind == ast.DirectiveTemplate {
			docTemplateSeen = true
		}
		if d.Kind == ast.DirectiveImport && docTemplateSeen {
			return &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E004",
				Message:  "@import must come before @doc/@template",
				Range:    ast.Range{Start: d.Position, End: d.Position},
			}
		}
	}
	return nil
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
			ref.IsLink = false
			ref.Position = ast.Position{Line: i + 1, Column: m[0] + 1}
			refs = append(refs, ref)
		}

		linkMatches := linkRe.FindAllStringSubmatchIndex(line, -1)
		for _, m := range linkMatches {
			full := line[m[0]:m[1]]
			isEscaped := len(full) > 0 && full[0] == '\\'
			if isEscaped {
				refs = append(refs, ast.Reference{
					Raw:       full,
					IsEscaped: true,
					IsLink:    true,
					Position:  ast.Position{Line: i + 1, Column: m[0] + 1},
				})
				continue
			}

			inner := line[m[2]:m[3]]
			ref := parseReferenceInner(inner)
			ref.Raw = full
			ref.IsLink = true
			ref.Position = ast.Position{Line: i + 1, Column: m[0] + 1}
			refs = append(refs, ref)
		}
	}

	return refs
}

// validateRefsAreAliasOnly enforces spec-v4 rule 3: {{}} and [[]] target tokens
// must be either local variables, alias-prefixed property/symbol access, or
// (for [[]]) a known import alias. Raw filesystem paths trigger E023; unknown
// or invalid aliases inside [[]] trigger E024.
func validateRefsAreAliasOnly(refs []ast.Reference, imports []ast.Import) []ast.Diagnostic {
	aliases := make(map[string]bool)
	for _, imp := range imports {
		aliases[imp.Alias] = true
	}

	var diags []ast.Diagnostic
	for _, ref := range refs {
		if ref.IsEscaped {
			continue
		}
		inner := refInner(ref)
		// {{#section}} — current-file symbol — fine
		if strings.HasPrefix(inner, "#") {
			continue
		}
		if strings.Contains(inner, "/") || strings.HasPrefix(inner, ".") {
			kind := "{{}}"
			if ref.IsLink {
				kind = "[[]]"
			}
			diags = append(diags, ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E023",
				Message:  fmt.Sprintf("raw path not allowed in %s; declare with @import alias from <path> first", kind),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			})
			continue
		}
		if ref.IsLink {
			head := ref.PathPart
			if !aliasRe.MatchString(head) {
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E024",
					Message:  fmt.Sprintf("invalid alias in %s", ref.Raw),
					Range:    ast.Range{Start: ref.Position, End: ref.Position},
				})
				continue
			}
			if !aliases[head] {
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E024",
					Message:  fmt.Sprintf("unknown alias %q in [[]]; declare with @import first", head),
					Range:    ast.Range{Start: ref.Position, End: ref.Position},
				})
			}
		}
	}
	return diags
}

// refInner strips the surrounding [[/]] or {{/}} from ref.Raw.
func refInner(ref ast.Reference) string {
	if ref.IsLink {
		return strings.TrimSuffix(strings.TrimPrefix(ref.Raw, "[["), "]]")
	}
	return strings.TrimSuffix(strings.TrimPrefix(ref.Raw, "{{"), "}}")
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
	case ast.DirectiveImport:
		return "import"
	case ast.DirectiveConst:
		return "const"
	case ast.DirectiveLet:
		return "let"
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

// validateFilePrelude returns E007 diagnostics for any file-level @import/@const/@let
// directives that appear after the body start (first @doc/@template/heading).
// Section-scope @const/@let inside a heading scope (line > firstHeadingLine) are allowed.
func validateFilePrelude(directives []ast.Directive, headings []*ast.Heading) []ast.Diagnostic {
	bodyStart := -1
	for _, d := range directives {
		if d.Kind == ast.DirectiveDoc || d.Kind == ast.DirectiveTemplate {
			bodyStart = d.Position.Line
			break
		}
	}
	firstHeadingLine := -1
	if len(headings) > 0 {
		firstHeadingLine = headings[0].Position.Line
		if bodyStart < 0 || firstHeadingLine < bodyStart {
			bodyStart = firstHeadingLine
		}
	}
	if bodyStart < 0 {
		return nil
	}

	var diags []ast.Diagnostic
	for _, d := range directives {
		if d.Kind != ast.DirectiveImport && d.Kind != ast.DirectiveConst && d.Kind != ast.DirectiveLet {
			continue
		}
		if d.Position.Line <= bodyStart {
			continue // prelude — fine
		}
		if firstHeadingLine > 0 && d.Position.Line > firstHeadingLine {
			continue // section-scope — fine
		}
		diags = append(diags, ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E007",
			Message:  fmt.Sprintf("file-level declaration after body start: @%s must be in the prelude (before first @doc/@template/heading)", directiveKindName(d.Kind)),
			Range:    ast.Range{Start: d.Position, End: d.Position},
		})
	}
	return diags
}

