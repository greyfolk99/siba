package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/control"
	"github.com/greyfolk99/siba/pkg/parser"
	"github.com/greyfolk99/siba/pkg/scope"
	"github.com/greyfolk99/siba/pkg/template"
	"github.com/greyfolk99/siba/pkg/workspace"
)

// StreamRender renders a document line-by-line to an io.Writer.
// Phase 1: scan directives (hoisting)
// Phase 2: line-by-line streaming interpreter
func StreamRender(doc *ast.Document, w io.Writer, ws *workspace.Workspace) error {
	ctx := NewEvalContext()
	return StreamRenderWithContext(doc, w, ctx, ws)
}

// StreamRenderWithContext renders with a shared eval context for cross-doc cycle detection.
func StreamRenderWithContext(doc *ast.Document, w io.Writer, ctx *EvalContext, ws *workspace.Workspace) error {
	// Cycle detection
	docKey := "doc:" + doc.Path
	if doc.Name != "" {
		docKey = "doc:" + doc.Name
	}
	if err := ctx.Enter(docKey); err != nil {
		return err
	}
	defer ctx.Leave(docKey)

	// Apply template inheritance
	var tmplDoc *ast.Document
	var defaults []defaultSection
	if ws != nil && doc.ExtendsName != "" {
		tmplDoc, _ = template.ResolveTemplate(doc, ws)
		if tmplDoc != nil {
			// Build default plan BEFORE merging (uses original child headings)
			defaults = buildDefaultPlan(doc, tmplDoc)
			doc.Variables = template.InheritVariables(doc, tmplDoc)
			doc.Headings = template.MergeHeadings(doc, tmplDoc)
		}
	}

	// Phase 1: Build scope tree (hoists variables to correct scopes)
	rootScope, _ := scope.BuildScopeTree(doc)

	// Phase 2: Line-by-line streaming
	lines := strings.Split(doc.Source, "\n")
	interp := &interpreter{
		lines:     lines,
		rootScope: rootScope,
		doc:       doc,
		ctx:       ctx,
		ws:        ws,
		writer:    w,
		defaults:  defaults,
	}

	return interp.run()
}

// defaultSection represents a @default section to inject during streaming
type defaultSection struct {
	afterSlug string   // insert after this child heading slug
	lines     []string // content lines to inject (directives stripped)
	emitted   bool
}

// flattenHeadings collects all headings recursively into a flat list (pre-order)
func flattenHeadings(headings []*ast.Heading) []*ast.Heading {
	var result []*ast.Heading
	for _, h := range headings {
		result = append(result, h)
		result = append(result, flattenHeadings(h.Children)...)
	}
	return result
}

// headingExists checks if a heading with given slug/text exists in a flat list
func headingExists(flat []*ast.Heading, slug, text string) bool {
	for _, h := range flat {
		if h.Slug == slug || h.Text == text {
			return true
		}
	}
	return false
}

// buildDefaultPlan creates injection plan for @default sections.
// Recursively checks all heading levels (H2, H3, H4, ...).
func buildDefaultPlan(child, tmpl *ast.Document) []defaultSection {
	tmplHeadings := tmpl.Headings
	if len(tmplHeadings) > 0 && tmplHeadings[0].Level == 1 {
		tmplHeadings = tmplHeadings[0].Children
	}

	// Flatten all headings for lookup
	tmplFlat := flattenHeadings(tmplHeadings)
	childFlat := flattenHeadings(child.Headings)

	tmplLines := strings.Split(tmpl.Source, "\n")
	var plan []defaultSection

	for i, th := range tmplFlat {
		if th.Annotation != ast.AnnotationDefault {
			continue
		}

		// Skip if child already has this heading
		if headingExists(childFlat, th.Slug, th.Text) {
			continue
		}

		// Extract content from template
		start := th.Position.Line - 1
		end := th.Content.End.Line
		if end <= 0 || end > len(tmplLines) {
			end = len(tmplLines)
		}
		var sectionLines []string
		for _, l := range tmplLines[start:end] {
			if !parser.IsDirectiveLine(l) {
				sectionLines = append(sectionLines, l)
			}
		}

		// Find preceding heading slug that exists in child
		afterSlug := ""
		for j := i - 1; j >= 0; j-- {
			prev := tmplFlat[j]
			if headingExists(childFlat, prev.Slug, prev.Text) {
				afterSlug = prev.Slug
				break
			}
		}

		plan = append(plan, defaultSection{
			afterSlug: afterSlug,
			lines:     sectionLines,
		})
	}

	return plan
}

// interpreter is the line-by-line streaming render engine.
type interpreter struct {
	lines     []string
	rootScope *scope.Scope
	doc       *ast.Document
	ctx       *EvalContext
	ws        *workspace.Workspace
	writer    io.Writer
	pos       int // current line index (0-based)
	defaults  []defaultSection
	lastSlug  string // slug of the last heading we passed
}

func (ip *interpreter) run() error {
	for ip.pos < len(ip.lines) {
		line := ip.lines[ip.pos]
		trimmed := strings.TrimSpace(line)

		// Directive line — check for control flow
		if parser.IsDirectiveLine(line) {
			if err := ip.handleDirective(trimmed); err != nil {
				return err
			}
			ip.pos++
			continue
		}

		// Check multi-line directive start
		if strings.Contains(trimmed, "<!--") && strings.Contains(trimmed, "@") && !strings.Contains(trimmed, "-->") {
			ip.pos++
			for ip.pos < len(ip.lines) {
				if strings.Contains(ip.lines[ip.pos], "-->") {
					ip.pos++
					break
				}
				ip.pos++
			}
			continue
		}

		// If this is a heading line, track slug and check @default injections
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for _, c := range trimmed {
				if c == '#' {
					level++
				} else {
					break
				}
			}
			headingText := strings.TrimSpace(trimmed[level:])
			slug := parser.GenerateSlug(headingText)

			// H2+ — check for @default injections before this heading
			if level >= 2 {
				if err := ip.emitPendingDefaults(slug); err != nil {
					return err
				}
				ip.lastSlug = slug
			}
		}

		// Normal text line — substitute variables and write
		lineNo := ip.pos + 1
		currentScope := scope.FindScopeForLine(ip.rootScope, lineNo)
		processed := ip.substituteVars(line, currentScope)
		if err := ip.writeLine(processed); err != nil {
			return err
		}
		ip.pos++
	}

	// Emit any remaining @default sections at the end
	return ip.emitPendingDefaults("")
}

// emitPendingDefaults writes @default sections that should appear before nextSlug.
// If nextSlug is empty, emits all remaining defaults (end of file).
func (ip *interpreter) emitPendingDefaults(nextSlug string) error {
	for i := range ip.defaults {
		d := &ip.defaults[i]
		if d.emitted {
			continue
		}

		// Emit if: this default should come after lastSlug and before nextSlug
		// or if nextSlug is empty (end of file)
		if d.afterSlug == ip.lastSlug || (nextSlug == "" && !d.emitted) {
			d.emitted = true
			if err := ip.writeLine(""); err != nil {
				return err
			}
			for _, dl := range d.lines {
				if err := ip.writeLine(dl); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (ip *interpreter) handleDirective(trimmed string) error {
	// Parse to check if it's @if or @for
	matches := directiveCheckRe.FindStringSubmatch(trimmed)
	if matches == nil {
		// Not a recognized control directive — just skip (strip)
		return nil
	}

	keyword := matches[1]

	switch keyword {
	case "if":
		return ip.handleIf(matches[2])
	case "for":
		return ip.handleFor(matches[2])
	default:
		// Other directives (@doc, @const, etc.) are stripped
		return nil
	}
}

func (ip *interpreter) handleIf(condition string) error {
	lineNo := ip.pos + 1
	currentScope := scope.FindScopeForLine(ip.rootScope, lineNo)

	result, _ := control.EvaluateIf(condition, currentScope)

	if result {
		// true — process lines until @endif, outputting them
		ip.pos++
		return ip.processUntilEnd("endif")
	}
	// false — skip until @endif
	ip.pos++
	ip.skipUntilEnd("endif")
	return nil
}

func (ip *interpreter) handleFor(args string) error {
	lineNo := ip.pos + 1
	currentScope := scope.FindScopeForLine(ip.rootScope, lineNo)

	// Parse "iterator in collection"
	parts := strings.SplitN(strings.TrimSpace(args), " in ", 2)
	if len(parts) != 2 {
		ip.pos++
		ip.skipUntilEnd("endfor")
		return nil
	}
	iterName := strings.TrimSpace(parts[0])
	collName := strings.TrimSpace(parts[1])

	iterations, _ := control.EvaluateFor(iterName, collName, currentScope)

	// Collect block lines (between @for and @endfor, exclusive of both)
	ip.pos++
	blockStart := ip.pos
	ip.skipUntilEnd("endfor")
	blockEnd := ip.pos // ip.pos points at @endfor line

	blockLines := ip.lines[blockStart:blockEnd]

	// Replay block for each iteration
	for _, iter := range iterations {
		for _, bline := range blockLines {
			if parser.IsDirectiveLine(bline) {
				continue // strip inner directives
			}
			processed := ip.substituteForLine(bline, iter, iterName)
			if err := ip.writeLine(processed); err != nil {
				return err
			}
		}
	}

	return nil
}

// processUntilEnd processes and outputs lines until @end{keyword}
func (ip *interpreter) processUntilEnd(endKeyword string) error {
	for ip.pos < len(ip.lines) {
		line := ip.lines[ip.pos]
		trimmed := strings.TrimSpace(line)

		if parser.IsDirectiveLine(line) {
			matches := directiveCheckRe.FindStringSubmatch(trimmed)
			if matches != nil {
				kw := matches[1]

				// Found our closing directive
				if kw == endKeyword {
					return nil
				}

				// Nested control block — recurse (handles its own @end)
				if kw == "if" {
					if err := ip.handleIf(matches[2]); err != nil {
						return err
					}
					ip.pos++ // skip past @endif
					continue
				}
				if kw == "for" {
					if err := ip.handleFor(matches[2]); err != nil {
						return err
					}
					ip.pos++ // skip past @endfor
					continue
				}
			}
			// Other directives — strip
			ip.pos++
			continue
		}

		lineNo := ip.pos + 1
		currentScope := scope.FindScopeForLine(ip.rootScope, lineNo)
		processed := ip.substituteVars(line, currentScope)
		if err := ip.writeLine(processed); err != nil {
			return err
		}
		ip.pos++
	}
	return nil
}

// skipUntilEnd skips lines until @end{keyword}, respecting nesting
func (ip *interpreter) skipUntilEnd(endKeyword string) {
	depth := 1
	startKw := "if"
	if endKeyword == "endfor" {
		startKw = "for"
	}
	for ip.pos < len(ip.lines) {
		trimmed := strings.TrimSpace(ip.lines[ip.pos])
		if parser.IsDirectiveLine(ip.lines[ip.pos]) {
			matches := directiveCheckRe.FindStringSubmatch(trimmed)
			if matches != nil {
				if matches[1] == startKw {
					depth++
				}
				if matches[1] == endKeyword {
					depth--
					if depth == 0 {
						return
					}
				}
			}
		}
		ip.pos++
	}
}

func (ip *interpreter) substituteVars(line string, currentScope *scope.Scope) string {
	// protect escapes
	line = protectEscapes(line)

	line = refRe.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[2 : len(match)-2]
		inner = strings.TrimSpace(inner)

		// #symbol — current file section/symbol reference (content insertion)
		if strings.HasPrefix(inner, "#") {
			symbol := inner[1:]
			if ip.doc != nil {
				h := findHeadingInList(ip.doc.Headings, symbol)
				if h != nil {
					return extractHeadingContent(ip.doc.Source, h)
				}
			}
			return match
		}

		// alias#symbol — imported file symbol reference
		if hashIdx := strings.Index(inner, "#"); hashIdx > 0 {
			alias := inner[:hashIdx]
			symbol := inner[hashIdx+1:]
			if ip.ws != nil && ip.doc != nil {
				for _, imp := range ip.doc.Imports {
					if imp.Alias == alias {
						targetDoc := resolveImportForRender(imp.Path, ip.ws)
						if targetDoc != nil {
							h := findHeadingInList(targetDoc.Headings, symbol)
							if h != nil {
								return extractHeadingContent(targetDoc.Source, h)
							}
						}
					}
				}
			}
			return match
		}

		// cycle detection for variables
		varKey := "var:" + ip.doc.Path + ":" + inner
		if err := ip.ctx.Enter(varKey); err != nil {
			return match // cycle — leave unresolved
		}
		defer ip.ctx.Leave(varKey)

		// local variable (with TDZ — declared line must be before reference)
		lineNo := ip.pos + 1
		if v, ok := currentScope.ResolveAt(inner, lineNo); ok && v.Value != nil {
			return ast.ValueToString(*v.Value)
		}

		// obj.prop
		if dotIdx := strings.LastIndex(inner, "."); dotIdx >= 0 {
			objName := inner[:dotIdx]
			propName := inner[dotIdx+1:]
			if v, ok := currentScope.ResolveAt(objName, lineNo); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
				if prop, ok := v.Value.Object[propName]; ok {
					return ast.ValueToString(prop)
				}
			}
			// module-level variable via @import alias
			if ip.ws != nil && ip.doc != nil {
				for _, imp := range ip.doc.Imports {
					if imp.Alias == objName {
						targetDoc := resolveImportForRender(imp.Path, ip.ws)
						if targetDoc != nil {
							for _, tv := range targetDoc.Variables {
								if tv.Name == propName && tv.Access != ast.AccessPrivate {
									if tv.Value != nil {
										return ast.ValueToString(*tv.Value)
									}
								}
							}
						}
					}
				}
			}
		}

		return match
	})

	// restore escapes
	line = restoreEscapes(line)
	return line
}

func (ip *interpreter) substituteForLine(line string, iter control.ForIteration, iterName string) string {
	// handle {{iterator.prop}} first
	if iter.Value.Kind == ast.TypeObject {
		for key, val := range iter.Value.Object {
			placeholder := "{{" + iterName + "." + key + "}}"
			line = strings.ReplaceAll(line, placeholder, ast.ValueToString(val))
		}
	}
	// handle {{iterator}}
	placeholder := "{{" + iterName + "}}"
	if iter.Value.Kind != ast.TypeObject {
		line = strings.ReplaceAll(line, placeholder, ast.ValueToString(iter.Value))
	}
	return line
}

func (ip *interpreter) writeLine(line string) error {
	_, err := fmt.Fprintln(ip.writer, line)
	return err
}

func resolveImportForRender(importPath string, ws *workspace.Workspace) *ast.Document {
	clean := strings.TrimPrefix(importPath, "./")
	if doc := ws.GetDocumentByPath(clean); doc != nil {
		return doc
	}
	if doc := ws.GetDocumentByPath(clean + ".md"); doc != nil {
		return doc
	}
	if doc := ws.GetDocument(importPath); doc != nil {
		return doc
	}
	return nil
}
var directiveCheckRe = refDirectiveRe

func findHeadingInList(headings []*ast.Heading, nameOrSlug string) *ast.Heading {
	for _, h := range headings {
		if h.Name == nameOrSlug || h.Slug == nameOrSlug {
			return h
		}
		if found := findHeadingInList(h.Children, nameOrSlug); found != nil {
			return found
		}
	}
	return nil
}

func extractHeadingContent(source string, h *ast.Heading) string {
	lines := strings.Split(source, "\n")
	start := h.Position.Line - 1
	end := h.Content.End.Line
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start >= len(lines) {
		return ""
	}
	// Strip directives from the section content
	var result []string
	for _, line := range lines[start:end] {
		if !parser.IsDirectiveLine(line) {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

