package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// Render processes a single document and returns clean markdown
func Render(doc *ast.Document) (string, error) {
	// Build scope tree (variables assigned to correct scopes by position)
	rootScope, scopeDiags := scope.BuildScopeTree(doc)
	doc.Diagnostics = append(doc.Diagnostics, scopeDiags...)

	content := doc.Source

	// 1. Evaluate control blocks (@if/@for)
	var controlDiags []ast.Diagnostic
	content, controlDiags = control.ProcessControlBlocks(content, doc.ControlBlocks, rootScope)
	doc.Diagnostics = append(doc.Diagnostics, controlDiags...)

	// 2. Protect escaped refs: \{{x}} -> placeholder
	content = protectEscapes(content)

	// 3. Substitute variables
	content = substituteVariables(content, rootScope, doc)

	// 4. Restore escaped refs: placeholder -> {{x}}
	content = restoreEscapes(content)

	// 5. Strip directives
	content = stripDirectives(content)

	// 6. Clean up excessive blank lines
	content = cleanBlankLines(content)

	return content, nil
}

func substituteVariables(content string, rootScope *scope.Scope, doc *ast.Document) string {
	lines := strings.Split(content, "\n")
	var result []string

	for lineNum, line := range lines {
		lineNo := lineNum + 1 // 1-based line numbers

		// skip directive lines - they'll be stripped later
		if parser.IsDirectiveLine(line) {
			result = append(result, line)
			continue
		}

		// find the scope for this line
		currentScope := scope.FindScopeForLine(rootScope, lineNo)

		// replace {{variable}} references
		processed := refRe.ReplaceAllStringFunc(line, func(match string) string {
			inner := match[2 : len(match)-2]
			inner = strings.TrimSpace(inner)

			// try to resolve as local variable from current scope (walks up chain)
			if v, ok := currentScope.Resolve(inner); ok && v.Value != nil {
				return ast.ValueToString(*v.Value)
			}

			// try property access: obj.prop
			if dotIdx := strings.LastIndex(inner, "."); dotIdx >= 0 {
				objName := inner[:dotIdx]
				propName := inner[dotIdx+1:]
				if v, ok := currentScope.Resolve(objName); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
					if prop, ok := v.Value.Object[propName]; ok {
						return ast.ValueToString(prop)
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
func RenderWorkspace(w *workspace.Workspace, outputDir string) error {
	version := w.GetVersion()
	if outputDir == "" {
		outputDir = filepath.Join(w.Root, "_render")
	}
	versionDir := filepath.Join(outputDir, "v"+version)

	errorCount := 0

	for path, doc := range w.DocsByPath {
		if doc.IsTemplate {
			continue
		}

		output, err := Render(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error rendering %s: %v\n", path, err)
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

