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

	// Apply template inheritance (lightweight — no parsing, just variable/heading merge)
	var defaults []defaultSection
	inheritedVars := make([]ast.Variable, len(doc.Variables))
	copy(inheritedVars, doc.Variables)
	if ws != nil && doc.ExtendsName != "" {
		tmplDoc, _ := template.ResolveTemplate(doc, ws)
		if tmplDoc != nil {
			tmplLines := strings.Split(tmplDoc.Source, "\n")
			defaults = buildDefaultPlan(doc, tmplDoc, tmplLines)
			inheritedVars, _ = template.InheritVariables(doc, tmplDoc)
		}
	}

	// Pure interpreter — no Phase 0 scope tree build
	// Scope is built on-the-fly as headings and variables are encountered
	lines := strings.Split(doc.Source, "\n")
	interp := &interpreter{
		lines:    lines,
		doc:      doc,
		ctx:      ctx,
		ws:       ws,
		writer:   w,
		defaults: defaults,
		scopeStack: []*scope.Scope{
			scope.NewScope("__root__", scope.ScopeHeading, nil),
		},
		inheritedVars: inheritedVars,
	}

	return interp.run()
}

// defaultSection represents a @default section to inject during streaming
type defaultSection struct {
	afterSlug string   // insert after this child heading slug
	lines     []string // content lines to inject (directives stripped)
	emitted   bool
}

// buildDefaultPlan creates injection plan for @default sections.
// Uses a slug set for O(1) child heading lookup. Single pass through template headings.
func buildDefaultPlan(child, tmpl *ast.Document, tmplLines []string) []defaultSection {
	tmplHeadings := tmpl.Headings
	if len(tmplHeadings) > 0 && tmplHeadings[0].Level == 1 {
		tmplHeadings = tmplHeadings[0].Children
	}

	// Build child heading slug set — O(1) lookup instead of O(n)
	childSlugs := make(map[string]bool)
	var collectSlugs func([]*ast.Heading)
	collectSlugs = func(hs []*ast.Heading) {
		for _, h := range hs {
			childSlugs[h.Slug] = true
			collectSlugs(h.Children)
		}
	}
	collectSlugs(child.Headings)

	// Flatten template headings and build plan in one pass
	var tmplFlat []*ast.Heading
	var walkTmpl func([]*ast.Heading)
	walkTmpl = func(hs []*ast.Heading) {
		for _, h := range hs {
			tmplFlat = append(tmplFlat, h)
			walkTmpl(h.Children)
		}
	}
	walkTmpl(tmplHeadings)

	var plan []defaultSection
	for i, th := range tmplFlat {
		if th.Annotation != ast.AnnotationDefault {
			continue
		}
		if childSlugs[th.Slug] {
			continue
		}

		// Extract content lines (reuse tmplLines, no re-split)
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

		// Find nearest preceding heading that exists in child
		afterSlug := ""
		for j := i - 1; j >= 0; j-- {
			if childSlugs[tmplFlat[j].Slug] {
				afterSlug = tmplFlat[j].Slug
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
// Pure interpreter — scope built on-the-fly, no pre-parsing phase.
type interpreter struct {
	lines         []string
	doc           *ast.Document
	ctx           *EvalContext
	ws            *workspace.Workspace
	writer        io.Writer
	pos           int // current line index (0-based)
	defaults      []defaultSection
	lastSlug      string // slug of the last heading we passed
	scopeStack    []*scope.Scope
	headingLevels []int // heading level stack (parallel to scopeStack for heading scopes)
	inheritedVars []ast.Variable
	varsRegistered bool // whether inherited vars have been registered
}

func (ip *interpreter) currentScope() *scope.Scope {
	return ip.scopeStack[len(ip.scopeStack)-1]
}

func (ip *interpreter) pushScope(name string, kind scope.ScopeKind) *scope.Scope {
	s := scope.NewScope(name, kind, ip.currentScope())
	ip.scopeStack = append(ip.scopeStack, s)
	return s
}

func (ip *interpreter) popScope() {
	if len(ip.scopeStack) > 1 {
		ip.scopeStack = ip.scopeStack[:len(ip.scopeStack)-1]
	}
}

// popToHeadingLevel pops scopes until we're at the right level for a new heading
func (ip *interpreter) popToHeadingLevel(level int) {
	for len(ip.headingLevels) > 0 && ip.headingLevels[len(ip.headingLevels)-1] >= level {
		ip.headingLevels = ip.headingLevels[:len(ip.headingLevels)-1]
		ip.popScope()
	}
}

func (ip *interpreter) run() error {
	// Register inherited variables in root scope
	for _, v := range ip.inheritedVars {
		ip.currentScope().Declare(v.Name, v)
	}

	for ip.pos < len(ip.lines) {
		line := ip.lines[ip.pos]
		trimmed := strings.TrimSpace(line)

		// Directive line — handle variable declarations and control flow
		if parser.IsDirectiveLine(line) {
			if err := ip.handleDirective(trimmed); err != nil {
				return err
			}
			ip.pos++
			continue
		}

		// Multi-line directive — skip
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

		// Heading line — manage scope on-the-fly
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

			if level >= 2 {
				// @default injection before this heading
				if err := ip.emitPendingDefaults(slug); err != nil {
					return err
				}
				ip.lastSlug = slug

				// Pop scopes back to parent level, then push new scope
				ip.popToHeadingLevel(level)
				ip.pushScope(slug, scope.ScopeHeading)
				ip.headingLevels = append(ip.headingLevels, level)
			}
		}

		// Normal text — substitute variables using current scope and write
		processed := ip.substituteVars(line, ip.currentScope())
		if err := ip.writeLine(processed); err != nil {
			return err
		}
		ip.pos++
	}

	// Emit remaining @default sections
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
	matches := directiveCheckRe.FindStringSubmatch(trimmed)
	if matches == nil {
		return nil
	}

	keyword := matches[1]
	args := strings.TrimSpace(matches[2])

	switch keyword {
	case "if":
		return ip.handleIf(args)
	case "for":
		return ip.handleFor(args)
	case "const", "let":
		// Register variable in current scope on-the-fly
		ip.registerVar(keyword, args)
		return nil
	default:
		// @doc, @template, @extends, @import, @name, @default, @endif, @endfor — strip
		return nil
	}
}

// registerVar parses and registers a variable declaration in the current scope
func (ip *interpreter) registerVar(keyword, args string) {
	// Normalize multiline
	args = strings.Join(strings.Fields(args), " ")

	mut := ast.MutConst
	if keyword == "let" {
		mut = ast.MutLet
	}

	// Parse: [private] name [: type] = value
	access := ast.AccessDefault
	if strings.HasPrefix(args, "private ") {
		access = ast.AccessPrivate
		args = strings.TrimPrefix(args, "private ")
		args = strings.TrimSpace(args)
	}

	// Split name = value
	eqIdx := strings.Index(args, "=")
	if eqIdx < 0 {
		return // type-only declaration, no value
	}

	namePart := strings.TrimSpace(args[:eqIdx])
	valPart := strings.TrimSpace(args[eqIdx+1:])

	// Strip type annotation from name (name: type → name)
	if colonIdx := strings.Index(namePart, ":"); colonIdx >= 0 {
		namePart = strings.TrimSpace(namePart[:colonIdx])
	}

	val, err := parser.ParseValue(valPart)
	if err != nil {
		return
	}

	v := ast.Variable{
		Name:       namePart,
		Mutability: mut,
		Access:     access,
		Value:      &val,
		Type:       parser.InferType(val),
		Position:   ast.Position{Line: ip.pos + 1},
	}

	ip.currentScope().Declare(namePart, v)
}

func (ip *interpreter) handleIf(condition string) error {
	
	currentScope := ip.currentScope()

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

	currentScope := ip.currentScope()

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

	blockLines := make([]string, blockEnd-blockStart)
	copy(blockLines, ip.lines[blockStart:blockEnd])

	// Replay block for each iteration using a sub-interpreter
	for _, iter := range iterations {
		// Pre-substitute iterator references in block lines
		substLines := make([]string, len(blockLines))
		for i, bline := range blockLines {
			substLines[i] = ip.substituteForLine(bline, iter, iterName)
		}

		subInterp := &interpreter{
			lines:    substLines,
			doc:      ip.doc,
			ctx:      ip.ctx,
			ws:       ip.ws,
			writer:   ip.writer,
			defaults: nil,
			scopeStack: []*scope.Scope{iter.Scope},
			inheritedVars: nil,
		}
		if err := subInterp.run(); err != nil {
			return err
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

		
		currentScope := ip.currentScope()
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
				h := ast.FindHeading(ip.doc.Headings, symbol)
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
						targetDoc := ip.ws.ResolveImportDoc(imp.Path)
						if targetDoc != nil {
							h := ast.FindHeading(targetDoc.Headings, symbol)
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
						targetDoc := ip.ws.ResolveImportDoc(imp.Path)
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

var directiveCheckRe = refDirectiveRe

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

