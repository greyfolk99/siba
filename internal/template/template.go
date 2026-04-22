package template

import (
	"fmt"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/workspace"
)

// ResolveTemplate finds the template for a document that uses @extends
func ResolveTemplate(doc *ast.Document, ws *workspace.Workspace) (*ast.Document, *ast.Diagnostic) {
	if doc.ExtendsName == "" {
		return nil, nil
	}

	tmpl := ws.GetTemplate(doc.ExtendsName)
	if tmpl == nil {
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E071",
			Message:  fmt.Sprintf("template not found: %s", doc.ExtendsName),
		}
	}

	if !tmpl.IsTemplate {
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E073",
			Message:  fmt.Sprintf("cannot extend non-template document: %s", doc.ExtendsName),
		}
	}

	return tmpl, nil
}

// ValidateContract validates that a child document satisfies a template's contract.
// It checks:
// - Required headings (non-@default) must exist in child
// - Heading levels must match template structure
func ValidateContract(child, tmpl *ast.Document) []ast.Diagnostic {
	var diags []ast.Diagnostic

	for _, th := range tmpl.Headings {
		validateHeading(th, child.Headings, &diags)
	}

	return diags
}

func validateHeading(tmplHeading *ast.Heading, childHeadings []*ast.Heading, diags *[]ast.Diagnostic) {
	childH := findMatchingHeading(childHeadings, tmplHeading)

	if childH == nil {
		// heading not found in child
		if tmplHeading.Annotation != ast.AnnotationDefault {
			// required heading is missing
			*diags = append(*diags, ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E070",
				Message:  fmt.Sprintf("required heading missing: %q (level %d)", tmplHeading.Text, tmplHeading.Level),
				Range:    ast.Range{Start: tmplHeading.Position, End: tmplHeading.Position},
			})
		}
		return
	}

	// heading found — check level match
	if childH.Level != tmplHeading.Level {
		*diags = append(*diags, ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E072",
			Message:  fmt.Sprintf("heading level mismatch for %q: expected %d, got %d", childH.Text, tmplHeading.Level, childH.Level),
			Range:    ast.Range{Start: childH.Position, End: childH.Position},
		})
	}

	// recursively validate children headings
	for _, tmplChild := range tmplHeading.Children {
		validateHeading(tmplChild, childH.Children, diags)
	}
}

func findMatchingHeading(headings []*ast.Heading, target *ast.Heading) *ast.Heading {
	for _, h := range headings {
		if matchesHeading(h, target) {
			return h
		}
	}
	return nil
}

func matchesHeading(h, target *ast.Heading) bool {
	// if target has @name, match only by @name (authoritative)
	if target.Name != "" {
		return h.Name == target.Name
	}
	// match by slug
	if target.Slug != "" && h.Slug == target.Slug {
		return true
	}
	// match by text
	return h.Text == target.Text
}

// InheritVariables returns variables from template that should be inherited by child.
// Public and protected variables are inherited.
// Private variables are excluded.
// Child variables with the same name override inherited ones.
func InheritVariables(child, tmpl *ast.Document) []ast.Variable {
	// collect child variable names for override detection
	childVars := make(map[string]bool)
	for _, v := range child.Variables {
		childVars[v.Name] = true
	}

	var inherited []ast.Variable
	for _, v := range tmpl.Variables {
		// skip private variables
		if v.Access == ast.AccessPrivate {
			continue
		}
		// skip if child overrides
		if childVars[v.Name] {
			continue
		}
		inherited = append(inherited, v)
	}

	// return child's own vars + inherited (child first = higher priority in scope)
	result := make([]ast.Variable, 0, len(child.Variables)+len(inherited))
	result = append(result, child.Variables...)
	result = append(result, inherited...)
	return result
}

// MergeHeadings merges template headings with child headings.
// Child headings override template headings. Template @default headings
// are used when child doesn't provide them.
func MergeHeadings(child, tmpl *ast.Document) []*ast.Heading {
	var result []*ast.Heading

	for _, th := range tmpl.Headings {
		ch := findMatchingHeading(child.Headings, th)
		if ch != nil {
			// child provides this heading — use child's version
			merged := *ch
			merged.Children = mergeChildHeadings(ch.Children, th.Children)
			result = append(result, &merged)
		} else if th.Annotation == ast.AnnotationDefault {
			// template default — use template's version
			result = append(result, th)
		}
		// if required and missing, ValidateContract already reported E070
	}

	// add any child headings not in template (extra sections)
	for _, ch := range child.Headings {
		if findMatchingHeading(tmpl.Headings, ch) == nil {
			result = append(result, ch)
		}
	}

	return result
}

func mergeChildHeadings(childHeadings, tmplHeadings []*ast.Heading) []*ast.Heading {
	if len(tmplHeadings) == 0 {
		// defensive copy to avoid aliasing original slice
		if len(childHeadings) == 0 {
			return nil
		}
		cp := make([]*ast.Heading, len(childHeadings))
		copy(cp, childHeadings)
		return cp
	}

	var result []*ast.Heading

	for _, th := range tmplHeadings {
		ch := findMatchingHeading(childHeadings, th)
		if ch != nil {
			merged := *ch
			merged.Children = mergeChildHeadings(ch.Children, th.Children)
			result = append(result, &merged)
		} else if th.Annotation == ast.AnnotationDefault {
			result = append(result, th)
		}
	}

	// add extra child headings not in template
	for _, ch := range childHeadings {
		if findMatchingHeading(tmplHeadings, ch) == nil {
			result = append(result, ch)
		}
	}

	return result
}
