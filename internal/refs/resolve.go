package refs

import (
	"fmt"
	"strings"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/scope"
	"github.com/hjseo/siba/internal/workspace"
)

// ResolvedKind indicates what a reference resolved to
type ResolvedKind int

const (
	ResolvedVariable ResolvedKind = iota
	ResolvedSection
	ResolvedDocument
)

// ResolvedRef is the result of resolving a reference
type ResolvedRef struct {
	Kind     ResolvedKind
	Value    string        // string representation of resolved value
	Variable *ast.Variable // non-nil for variable references
	Heading  *ast.Heading  // non-nil for section references
	Document *ast.Document // non-nil for document references
}

// ResolveReference resolves a single {{}} reference
func ResolveReference(ref ast.Reference, currentDoc *ast.Document, rootScope *scope.Scope, ws *workspace.Workspace) (*ResolvedRef, *ast.Diagnostic) {
	if ref.IsEscaped {
		return nil, nil
	}

	// Local variable reference (no path, no section, just a name)
	if ref.PathPart != "" && ref.Section == "" && ref.Variable == "" && !strings.Contains(ref.PathPart, "/") {
		// could be local variable or doc reference
		lineScope := scope.FindScopeForLine(rootScope, ref.Position.Line)

		// try local variable first
		if v, ok := lineScope.Resolve(ref.PathPart); ok {
			if v.Value == nil {
				return nil, &ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E050",
					Message:  fmt.Sprintf("variable %q has no value", ref.PathPart),
					Range:    ast.Range{Start: ref.Position, End: ref.Position},
				}
			}
			return &ResolvedRef{
				Kind:     ResolvedVariable,
				Value:    ast.ValueToString(*v.Value),
				Variable: v,
			}, nil
		}

		// try as document reference
		if ws != nil {
			if doc := ws.GetDocument(ref.PathPart); doc != nil {
				return &ResolvedRef{
					Kind:     ResolvedDocument,
					Document: doc,
				}, nil
			}
		}

		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E050",
			Message:  fmt.Sprintf("unresolved reference: %s", ref.PathPart),
			Range:    ast.Range{Start: ref.Position, End: ref.Position},
		}
	}

	// Section reference: #section or doc#section
	if ref.Section != "" {
		targetDoc := currentDoc
		if ref.PathPart != "" {
			if ws == nil {
				return nil, &ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E051",
					Message:  fmt.Sprintf("cannot resolve cross-document reference without workspace: %s", ref.Raw),
					Range:    ast.Range{Start: ref.Position, End: ref.Position},
				}
			}
			targetDoc = resolveDocByNameOrPath(ref.PathPart, ws)
			if targetDoc == nil {
				return nil, &ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E052",
					Message:  fmt.Sprintf("document not found: %s", ref.PathPart),
					Range:    ast.Range{Start: ref.Position, End: ref.Position},
				}
			}
		}

		if targetDoc == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E050",
				Message:  fmt.Sprintf("no document context for section reference: #%s", ref.Section),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		heading := findHeading(targetDoc.Headings, ref.Section)
		if heading == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E053",
				Message:  fmt.Sprintf("section not found: %s#%s", ref.PathPart, ref.Section),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		return &ResolvedRef{
			Kind:    ResolvedSection,
			Heading: heading,
		}, nil
	}

	// Document variable reference: doc.variable or path.variable
	if ref.PathPart != "" && ref.Variable != "" {
		// try local variable with property access first (no workspace needed)
		lineScope := scope.FindScopeForLine(rootScope, ref.Position.Line)
		if v, ok := lineScope.Resolve(ref.PathPart); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
			if prop, ok := v.Value.Object[ref.Variable]; ok {
				return &ResolvedRef{
					Kind:  ResolvedVariable,
					Value: ast.ValueToString(prop),
				}, nil
			}
		}

		if ws == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E051",
				Message:  fmt.Sprintf("cannot resolve cross-document reference without workspace: %s", ref.Raw),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		targetDoc := resolveDocByNameOrPath(ref.PathPart, ws)
		if targetDoc == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E052",
				Message:  fmt.Sprintf("document not found: %s", ref.PathPart),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		// find root-level variable in target doc
		for i, v := range targetDoc.Variables {
			if v.Name == ref.Variable && v.Access == ast.AccessPublic {
				if v.Value == nil {
					return nil, &ast.Diagnostic{
						Severity: ast.SeverityError,
						Code:     "E054",
						Message:  fmt.Sprintf("variable %q has no value: %s.%s", ref.Variable, ref.PathPart, ref.Variable),
						Range:    ast.Range{Start: ref.Position, End: ref.Position},
					}
				}
				return &ResolvedRef{
					Kind:     ResolvedVariable,
					Value:    ast.ValueToString(*v.Value),
					Variable: &targetDoc.Variables[i],
				}, nil
			}
		}

		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E054",
			Message:  fmt.Sprintf("variable not found or not public: %s.%s", ref.PathPart, ref.Variable),
			Range:    ast.Range{Start: ref.Position, End: ref.Position},
		}
	}

	// Path-based document reference
	if ref.PathPart != "" && strings.Contains(ref.PathPart, "/") {
		if ws == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E051",
				Message:  fmt.Sprintf("cannot resolve path reference without workspace: %s", ref.Raw),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		doc := ws.GetDocumentByPath(ref.PathPart)
		if doc == nil {
			// try with .md extension
			doc = ws.GetDocumentByPath(ref.PathPart + ".md")
		}
		if doc == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E055",
				Message:  fmt.Sprintf("document not found at path: %s", ref.PathPart),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		return &ResolvedRef{
			Kind:     ResolvedDocument,
			Document: doc,
		}, nil
	}

	return nil, &ast.Diagnostic{
		Severity: ast.SeverityError,
		Code:     "E050",
		Message:  fmt.Sprintf("unresolved reference: %s", ref.Raw),
		Range:    ast.Range{Start: ref.Position, End: ref.Position},
	}
}

// ValidateReferences validates all references in a document
func ValidateReferences(doc *ast.Document, rootScope *scope.Scope, ws *workspace.Workspace) []ast.Diagnostic {
	var diags []ast.Diagnostic
	for _, ref := range doc.References {
		if ref.IsEscaped {
			continue
		}
		_, d := ResolveReference(ref, doc, rootScope, ws)
		if d != nil {
			diags = append(diags, *d)
		}
	}
	return diags
}

func resolveDocByNameOrPath(name string, ws *workspace.Workspace) *ast.Document {
	// try by @doc name
	if doc := ws.GetDocument(name); doc != nil {
		return doc
	}
	// try by path
	if doc := ws.GetDocumentByPath(name); doc != nil {
		return doc
	}
	// try by path with .md
	if doc := ws.GetDocumentByPath(name + ".md"); doc != nil {
		return doc
	}
	return nil
}

func findHeading(headings []*ast.Heading, nameOrSlug string) *ast.Heading {
	for _, h := range headings {
		if h.Name == nameOrSlug || h.Slug == nameOrSlug {
			return h
		}
		// search children
		if found := findHeading(h.Children, nameOrSlug); found != nil {
			return found
		}
	}
	return nil
}

// DependencyGraph tracks document dependencies for cycle detection
type DependencyGraph struct {
	Edges map[string][]string // doc name → list of referenced doc names
}

// BuildDependencyGraph builds a dependency graph from a workspace
func BuildDependencyGraph(ws *workspace.Workspace) DependencyGraph {
	g := DependencyGraph{
		Edges: make(map[string][]string),
	}

	for _, doc := range ws.DocsByPath {
		docID := doc.Path
		if doc.Name != "" {
			docID = doc.Name
		}

		var deps []string

		// @extends creates a dependency
		if doc.ExtendsName != "" {
			deps = append(deps, doc.ExtendsName)
		}

		// {{doc-name}} references create dependencies
		for _, ref := range doc.References {
			if ref.IsEscaped {
				continue
			}
			if ref.PathPart != "" && !strings.Contains(ref.PathPart, "/") {
				// could be a doc reference
				if ws.GetDocument(ref.PathPart) != nil {
					deps = append(deps, ref.PathPart)
				}
			}
		}

		if len(deps) > 0 {
			g.Edges[docID] = deps
		}
	}

	return g
}

// DetectCycles finds circular references in the dependency graph
func DetectCycles(g DependencyGraph) []ast.Diagnostic {
	var diags []ast.Diagnostic

	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		inStack[node] = true

		for _, dep := range g.Edges[node] {
			if !visited[dep] {
				// copy path to avoid slice aliasing
				newPath := make([]string, len(path)+1)
				copy(newPath, path)
				newPath[len(path)] = dep
				dfs(dep, newPath)
			} else if inStack[dep] {
				// cycle found — copy path for safe append
				cyclePath := make([]string, len(path)+1)
				copy(cyclePath, path)
				cyclePath[len(path)] = dep
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     "E060",
					Message:  fmt.Sprintf("circular reference detected: %s", strings.Join(cyclePath, " → ")),
				})
			}
		}

		inStack[node] = false
	}

	for node := range g.Edges {
		if !visited[node] {
			dfs(node, []string{node})
		}
	}

	return diags
}
