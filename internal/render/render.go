package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/control"
	"github.com/hjseo/siba/internal/parser"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/workspace"
)

const escapePlaceholder = "\x00SIBA_ESCAPE\x00"

var (
	escapeRefRe = regexp.MustCompile(`\\\{\{([^}]+)\}\}`)
	refRe       = regexp.MustCompile(`\{\{([^}]+)\}\}`)
)

// EvalContext tracks what is currently being evaluated to detect cycles at runtime.
// Uses the Nix-style "evaluating" marker pattern:
//   - Before evaluating a node, mark it as "evaluating"
//   - If we encounter a node already marked "evaluating", it's a cycle
//   - After evaluation completes, mark it as "evaluated"
//
// Cycle detection levels:
//   - Variable: {{a}} where a references {{b}} which references {{a}}
//   - Document: {{doc-a}} inserts content that references {{doc-b}} which references {{doc-a}}
//   - Extends:  doc-a extends tmpl-b which extends tmpl-c which extends tmpl-a
//   - Package:  pkg-a depends on pkg-b which depends on pkg-a
type EvalContext struct {
	mu         sync.Mutex
	evaluating map[string]bool   // nodes currently being evaluated (cycle = error)
	evaluated  map[string]string // nodes that finished evaluation (cache)
	path       []string          // current evaluation path (for error messages)
}

// NewEvalContext creates a new evaluation context
func NewEvalContext() *EvalContext {
	return &EvalContext{
		evaluating: make(map[string]bool),
		evaluated:  make(map[string]string),
	}
}

// CycleError represents a circular reference detected at runtime
type CycleError struct {
	Path []string
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("circular reference: %s", strings.Join(e.Path, " → "))
}

// Enter marks a node as "currently evaluating". Returns a CycleError if already evaluating.
func (ec *EvalContext) Enter(key string) *CycleError {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.evaluating[key] {
		// find the cycle path
		cyclePath := make([]string, len(ec.path)+1)
		copy(cyclePath, ec.path)
		cyclePath[len(ec.path)] = key
		return &CycleError{Path: cyclePath}
	}

	ec.evaluating[key] = true
	ec.path = append(ec.path, key)
	return nil
}

// Leave marks a node as "done evaluating".
func (ec *EvalContext) Leave(key string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	delete(ec.evaluating, key)
	if len(ec.path) > 0 {
		ec.path = ec.path[:len(ec.path)-1]
	}
}

// Cache stores a computed value for a node
func (ec *EvalContext) Cache(key, value string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.evaluated[key] = value
}

// GetCached retrieves a previously computed value
func (ec *EvalContext) GetCached(key string) (string, bool) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	v, ok := ec.evaluated[key]
	return v, ok
}

// Render processes a single document and returns clean markdown
func Render(doc *ast.Document) (string, error) {
	ctx := NewEvalContext()
	return RenderWithContext(doc, ctx)
}

// RenderWithContext renders a document with a shared EvalContext (for cross-document cycle detection)
func RenderWithContext(doc *ast.Document, ctx *EvalContext) (string, error) {
	// mark this document as being evaluated
	docKey := "doc:" + doc.Path
	if doc.Name != "" {
		docKey = "doc:" + doc.Name
	}
	if err := ctx.Enter(docKey); err != nil {
		return "", err
	}
	defer ctx.Leave(docKey)

	// Build scope tree
	rootScope, scopeDiags := scope.BuildScopeTree(doc)
	doc.Diagnostics = append(doc.Diagnostics, scopeDiags...)

	content := doc.Source

	// 1. Evaluate control blocks (@if/@for)
	var controlDiags []ast.Diagnostic
	content, controlDiags = control.ProcessControlBlocks(content, doc.ControlBlocks, rootScope)
	doc.Diagnostics = append(doc.Diagnostics, controlDiags...)

	// 2. Protect escaped refs: \{{x}} -> placeholder
	content = protectEscapes(content)

	// 3. Substitute variables (with cycle detection)
	content = substituteVariables(content, rootScope, doc, ctx)

	// 4. Restore escaped refs: placeholder -> {{x}}
	content = restoreEscapes(content)

	// 5. Strip directives
	content = stripDirectives(content)

	// 6. Clean up excessive blank lines
	content = cleanBlankLines(content)

	// cache result
	ctx.Cache(docKey, content)

	return content, nil
}

func substituteVariables(content string, rootScope *scope.Scope, doc *ast.Document, ctx *EvalContext) string {
	lines := strings.Split(content, "\n")
	var result []string

	for lineNum, line := range lines {
		lineNo := lineNum + 1

		if parser.IsDirectiveLine(line) {
			result = append(result, line)
			continue
		}

		currentScope := scope.FindScopeForLine(rootScope, lineNo)

		processed := refRe.ReplaceAllStringFunc(line, func(match string) string {
			inner := match[2 : len(match)-2]
			inner = strings.TrimSpace(inner)

			// mark variable as being evaluated
			varKey := "var:" + doc.Path + ":" + inner
			if err := ctx.Enter(varKey); err != nil {
				doc.Diagnostics = append(doc.Diagnostics, ast.Diagnostic{
					File:     doc.Path,
					Severity: ast.SeverityError,
					Code:     "E061",
					Message:  err.Error(),
					Range:    ast.Range{Start: ast.Position{Line: lineNo}},
				})
				return match // leave unresolved
			}
			defer ctx.Leave(varKey)

			// try local variable
			if v, ok := currentScope.Resolve(inner); ok && v.Value != nil {
				value := ast.ValueToString(*v.Value)
				ctx.Cache(varKey, value)
				return value
			}

			// try property access: obj.prop
			if dotIdx := strings.LastIndex(inner, "."); dotIdx >= 0 {
				objName := inner[:dotIdx]
				propName := inner[dotIdx+1:]
				if v, ok := currentScope.Resolve(objName); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
					if prop, ok := v.Value.Object[propName]; ok {
						value := ast.ValueToString(prop)
						ctx.Cache(varKey, value)
						return value
					}
				}
			}

			// unresolved - leave as is
			return match
		})

		result = append(result, processed)
	}

	return strings.Join(result, "\n")
}

func protectEscapes(content string) string {
	return escapeRefRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := match[3 : len(match)-2]
		return escapePlaceholder + inner + escapePlaceholder
	})
}

func restoreEscapes(content string) string {
	parts := strings.Split(content, escapePlaceholder)
	if len(parts) == 1 {
		return content
	}
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString("{{")
			b.WriteString(part)
			b.WriteString("}}")
		} else {
			b.WriteString(part)
		}
	}
	return b.String()
}

func stripDirectives(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inMultiLine := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inMultiLine {
			if strings.Contains(trimmed, "-->") {
				inMultiLine = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "<!--") && strings.Contains(trimmed, "@") {
			if strings.Contains(trimmed, "-->") {
				continue
			}
			inMultiLine = true
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func cleanBlankLines(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	blankCount := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 2 {
				result = append(result, line)
			}
		} else {
			blankCount = 0
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// RenderWorkspace renders all documents in a workspace to _render/{version}/
// Uses a shared EvalContext so cross-document cycles are detected.
func RenderWorkspace(w *workspace.Workspace, outputDir string) error {
	version := w.GetVersion()
	if outputDir == "" {
		outputDir = filepath.Join(w.Root, "_render")
	}
	versionDir := filepath.Join(outputDir, "v"+version)

	// shared context across all documents — catches cross-document cycles
	ctx := NewEvalContext()
	errorCount := 0

	for path, doc := range w.DocsByPath {
		if doc.IsTemplate {
			continue
		}

		output, err := RenderWithContext(doc, ctx)
		if err != nil {
			if cycleErr, ok := err.(*CycleError); ok {
				fmt.Fprintf(os.Stderr, "cycle error in %s: %s\n", path, cycleErr.Error())
			} else {
				fmt.Fprintf(os.Stderr, "error rendering %s: %v\n", path, err)
			}
			errorCount++
			continue
		}

		outPath := filepath.Join(versionDir, path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("failed to create output dir: %w", err)
		}
		if err := os.WriteFile(outPath, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}

		fmt.Printf("  rendered: %s\n", outPath)
	}

	if errorCount > 0 {
		return fmt.Errorf("%d file(s) failed to render", errorCount)
	}
	return nil
}

