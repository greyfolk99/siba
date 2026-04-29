package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/greyfolk99/siba/pkg/workspace"
)

const escapePlaceholder = "\x00SIBA_ESCAPE\x00"

var (
	escapeRefRe    = regexp.MustCompile(`\\\{\{([^}]+)\}\}`)
	refRe          = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	refDirectiveRe = regexp.MustCompile(`<!--\s*@(\w+)\s*(.*?)\s*-->`)
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

// RenderWorkspace exports all documents to _export/{version}/ using StreamRender.
func RenderWorkspace(w *workspace.Workspace, outputDir string) error {
	version := w.GetVersion()
	if outputDir == "" {
		outputDir = filepath.Join(w.Root, "_export")
	}
	versionDir := filepath.Join(outputDir, "v"+version)

	// shared context across all documents — catches cross-document cycles
	ctx := NewEvalContext()
	errorCount := 0

	for path, doc := range w.DocsByPath {
		if doc.IsTemplate {
			continue
		}

		outPath := filepath.Join(versionDir, path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("failed to create output dir: %w", err)
		}

		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", outPath, err)
		}

		err = StreamRenderWithContext(doc, f, ctx, w)
		f.Close()

		if err != nil {
			if cycleErr, ok := err.(*CycleError); ok {
				fmt.Fprintf(os.Stderr, "cycle error in %s: %s\n", path, cycleErr.Error())
			} else {
				fmt.Fprintf(os.Stderr, "error exporting %s: %v\n", path, err)
			}
			os.Remove(outPath)
			errorCount++
			continue
		}

		fmt.Printf("  exported: %s\n", outPath)
	}

	if errorCount > 0 {
		return fmt.Errorf("%d file(s) failed to export", errorCount)
	}
	return nil
}
