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
//
// Reference patterns:
//   {{variable}}        — local variable (scope chain)
//   {{obj.prop}}        — local object property
//   {{#symbol}}         — current file symbol (template/doc/section)
//   {{#parent/child}}   — current file nested symbol
//   {{alias#symbol}}    — imported file symbol (@import required)
func ResolveReference(ref ast.Reference, currentDoc *ast.Document, rootScope *scope.Scope, ws *workspace.Workspace) (*ResolvedRef, *ast.Diagnostic) {
	if ref.IsEscaped {
		return nil, nil
	}

	// Case 1: # symbol reference (alias#symbol or #symbol)
	if ref.Section != "" {
		return resolveSymbolRef(ref, currentDoc, ws)
	}

	// Case 2: obj.prop — local object property OR @import alias.var
	if ref.PathPart != "" && ref.Variable != "" {
		lineScope := scope.FindScopeForLine(rootScope, ref.Position.Line)
		// Try local object property
		if v, ok := lineScope.Resolve(ref.PathPart); ok && v.Value != nil && v.Value.Kind == ast.TypeObject {
			if prop, ok := v.Value.Object[ref.Variable]; ok {
				return &ResolvedRef{
					Kind:  ResolvedVariable,
					Value: ast.ValueToString(prop),
				}, nil
			}
		}
		// Try @import alias.variable (module-level variable)
		if currentDoc != nil && ws != nil {
			for _, imp := range currentDoc.Imports {
				if imp.Alias == ref.PathPart {
					targetDoc := resolveImportPath(imp.Path, ws)
					if targetDoc != nil {
						for i, tv := range targetDoc.Variables {
							if tv.Name == ref.Variable && tv.Access != ast.AccessPrivate && tv.Value != nil {
								return &ResolvedRef{
									Kind:     ResolvedVariable,
									Value:    ast.ValueToString(*tv.Value),
									Variable: &targetDoc.Variables[i],
								}, nil
							}
						}
					}
				}
			}
		}
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E050",
			Message:  fmt.Sprintf("unresolved property: %s.%s", ref.PathPart, ref.Variable),
			Range:    ast.Range{Start: ref.Position, End: ref.Position},
		}
	}

	// Case 3: simple name — local variable or current-file doc/template
	if ref.PathPart != "" && ref.Section == "" && ref.Variable == "" {
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

		// try as document reference (by @doc name)
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

	return nil, &ast.Diagnostic{
		Severity: ast.SeverityError,
		Code:     "E050",
		Message:  fmt.Sprintf("unresolved reference: %s", ref.Raw),
		Range:    ast.Range{Start: ref.Position, End: ref.Position},
	}
}

// resolveSymbolRef handles # references: #symbol, alias#symbol, #parent/child
func resolveSymbolRef(ref ast.Reference, currentDoc *ast.Document, ws *workspace.Workspace) (*ResolvedRef, *ast.Diagnostic) {
	alias := ref.PathPart  // before #, empty = current file
	symbol := ref.Section  // after #, may contain / for nesting

	var targetDoc *ast.Document

	if alias == "" {
		// #symbol — resolve in current file
		targetDoc = currentDoc
	} else {
		// alias#symbol — resolve via @import
		if currentDoc == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E051",
				Message:  fmt.Sprintf("no document context for import reference: %s", ref.Raw),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		// find import alias
		importPath := ""
		for _, imp := range currentDoc.Imports {
			if imp.Alias == alias {
				importPath = imp.Path
				break
			}
		}
		if importPath == "" {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E052",
				Message:  fmt.Sprintf("import alias not found: %s", alias),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}

		// resolve import path to document via workspace
		if ws != nil {
			targetDoc = resolveImportPath(importPath, ws)
		}
		if targetDoc == nil {
			return nil, &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E052",
				Message:  fmt.Sprintf("imported file not found: %s (alias %s)", importPath, alias),
				Range:    ast.Range{Start: ref.Position, End: ref.Position},
			}
		}
	}

	if targetDoc == nil {
		return nil, &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E050",
			Message:  fmt.Sprintf("no document context for symbol reference: #%s", symbol),
			Range:    ast.Range{Start: ref.Position, End: ref.Position},
		}
	}

	// resolve symbol within the target document
	// First try heading by name/slug
	heading := findHeading(targetDoc.Headings, symbol)
	if heading != nil {
		return &ResolvedRef{
			Kind:    ResolvedSection,
			Heading: heading,
		}, nil
	}

	// For nested symbols with /, try the first segment
	if strings.Contains(symbol, "/") {
		parts := strings.SplitN(symbol, "/", 2)
		heading = findHeading(targetDoc.Headings, parts[0])
		if heading != nil && len(parts) > 1 {
			// recursive search in children
			nested := findHeading(heading.Children, parts[1])
			if nested != nil {
				return &ResolvedRef{
					Kind:    ResolvedSection,
					Heading: nested,
				}, nil
			}
		}
	}

	return nil, &ast.Diagnostic{
		Severity: ast.SeverityError,
		Code:     "E053",
		Message:  fmt.Sprintf("symbol not found: %s#%s", alias, symbol),
		Range:    ast.Range{Start: ref.Position, End: ref.Position},
	}
}

// resolveImportPath resolves an import path (relative or absolute) to a document
func resolveImportPath(importPath string, ws *workspace.Workspace) *ast.Document {
	// strip leading ./
	clean := strings.TrimPrefix(importPath, "./")

	// try by path
	if doc := ws.GetDocumentByPath(clean); doc != nil {
		return doc
	}
	// try with .md
	if doc := ws.GetDocumentByPath(clean + ".md"); doc != nil {
		return doc
	}
	// try by @doc name (for package references)
	if doc := ws.GetDocument(importPath); doc != nil {
		return doc
	}
	return nil
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
