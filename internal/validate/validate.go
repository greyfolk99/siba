package validate

import (
	"sort"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/refs"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/template"
	"github.com/hjseo/siba/internal/workspace"
)

// ValidateDocument runs all validation passes on a single document.
// Returns diagnostics from scope building, reference resolution, and template validation.
func ValidateDocument(doc *ast.Document, ws *workspace.Workspace) []ast.Diagnostic {
	var diags []ast.Diagnostic

	// 1. Build scope tree (catches duplicate declarations, const shadowing)
	rootScope, scopeDiags := scope.BuildScopeTree(doc)
	diags = append(diags, scopeDiags...)

	// 2. Validate references (unresolved refs, missing docs/sections)
	refDiags := refs.ValidateReferences(doc, rootScope, ws)
	diags = append(diags, refDiags...)

	// 3. Template contract validation (if extending a template)
	if doc.ExtendsName != "" && ws != nil {
		tmplDiags := validateTemplate(doc, ws)
		diags = append(diags, tmplDiags...)
	}

	// set file path on all diagnostics
	for i := range diags {
		if diags[i].File == "" {
			diags[i].File = doc.Path
		}
	}

	return diags
}

func validateTemplate(doc *ast.Document, ws *workspace.Workspace) []ast.Diagnostic {
	tmpl, diag := template.ResolveTemplate(doc, ws)
	if diag != nil {
		return []ast.Diagnostic{*diag}
	}
	if tmpl == nil {
		return nil
	}
	return template.ValidateContract(doc, tmpl)
}

// ValidateWorkspace validates all documents in a workspace.
// Returns a map of file path → diagnostics, plus workspace-level diagnostics.
func ValidateWorkspace(ws *workspace.Workspace) (map[string][]ast.Diagnostic, []ast.Diagnostic) {
	fileDiags := make(map[string][]ast.Diagnostic)
	var wsDiags []ast.Diagnostic

	// 1. Validate each document
	for path, doc := range ws.DocsByPath {
		diags := ValidateDocument(doc, ws)
		if len(diags) > 0 {
			fileDiags[path] = diags
		}
	}

	// 2. Workspace-level: circular reference detection
	g := refs.BuildDependencyGraph(ws)
	cycleDiags := refs.DetectCycles(g)
	wsDiags = append(wsDiags, cycleDiags...)

	return fileDiags, wsDiags
}

// HasErrors checks if any diagnostics are errors
func HasErrors(diags []ast.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == ast.SeverityError {
			return true
		}
	}
	return false
}

// FilterBySeverity filters diagnostics by severity
func FilterBySeverity(diags []ast.Diagnostic, severity ast.Severity) []ast.Diagnostic {
	var result []ast.Diagnostic
	for _, d := range diags {
		if d.Severity == severity {
			result = append(result, d)
		}
	}
	return result
}

// AllDiagnostics flattens file diagnostics and workspace diagnostics into one slice.
// File diagnostics are ordered by file path for deterministic output.
func AllDiagnostics(fileDiags map[string][]ast.Diagnostic, wsDiags []ast.Diagnostic) []ast.Diagnostic {
	// sort file paths for deterministic ordering
	paths := make([]string, 0, len(fileDiags))
	for path := range fileDiags {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var all []ast.Diagnostic
	for _, path := range paths {
		all = append(all, fileDiags[path]...)
	}
	all = append(all, wsDiags...)
	return all
}
