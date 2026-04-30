package validate

import (
	"sort"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/refs"
	"github.com/greyfolk99/siba/pkg/scope"
	"github.com/greyfolk99/siba/pkg/template"
	"github.com/greyfolk99/siba/pkg/workspace"
)

// ValidateDocument runs all validation passes on a single document.
// Returns diagnostics from scope building, reference resolution, and template validation.
func ValidateDocument(doc *ast.Document, ws *workspace.Workspace) []ast.Diagnostic {
	var diags []ast.Diagnostic

	// 0. Resolve template once (reuse for inheritance + contract validation)
	var tmpl *ast.Document
	if doc.ExtendsName != "" && ws != nil {
		var tmplDiag *ast.Diagnostic
		tmpl, tmplDiag = template.ResolveTemplate(doc, ws)
		if tmplDiag != nil {
			diags = append(diags, *tmplDiag)
		}
	}

	// 1. Build scope tree using inherited variables (without mutating doc.Variables)
	mergedVars := doc.Variables
	if tmpl != nil {
		var inheritDiags []ast.Diagnostic
		mergedVars, inheritDiags = template.InheritVariables(doc, tmpl)
		diags = append(diags, inheritDiags...)
	}
	rootScope, scopeDiags := scope.BuildScopeTreeWithVars(doc, mergedVars)
	diags = append(diags, scopeDiags...)

	// 2. Validate references — skip refs inside @for/@if blocks that use iterator vars
	refDiags := refs.ValidateReferences(doc, rootScope, ws)
	refDiags = filterForIteratorFalsePositives(doc, refDiags)
	diags = append(diags, refDiags...)

	// 3. Template contract validation (reuse resolved template)
	if tmpl != nil {
		diags = append(diags, template.ValidateContract(doc, tmpl)...)
	}

	// set file path on all diagnostics
	for i := range diags {
		if diags[i].File == "" {
			diags[i].File = doc.Path
		}
	}

	return diags
}

// filterForIteratorFalsePositives removes E050 diagnostics for variables that are
// @for iterator names or iterator property accesses (e.g., {{item}}, {{item.name}}).
// These are resolved at render time, not at static analysis time.
func filterForIteratorFalsePositives(doc *ast.Document, diags []ast.Diagnostic) []ast.Diagnostic {
	// Collect all @for iterator names and their line ranges
	type forInfo struct {
		iterator string
		start    int
		end      int
	}
	var fors []forInfo
	for _, cb := range doc.ControlBlocks {
		if cb.Kind == ast.DirectiveFor && cb.Iterator != "" {
			fors = append(fors, forInfo{
				iterator: cb.Iterator,
				start:    cb.Start.Line,
				end:      cb.End.Line,
			})
		}
	}

	if len(fors) == 0 {
		return diags
	}

	var filtered []ast.Diagnostic
	for _, d := range diags {
		if d.Code == "E050" {
			skip := false
			for _, f := range fors {
				line := d.Range.Start.Line
				if line >= f.start && line <= f.end {
					// Check if the unresolved ref matches the iterator name or iterator.prop
					msg := d.Message
					if strings.Contains(msg, f.iterator) {
						skip = true
						break
					}
				}
			}
			if skip {
				continue
			}
		}
		filtered = append(filtered, d)
	}
	return filtered
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
