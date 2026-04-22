package scope

import (
	"fmt"
	"strings"

	"github.com/hjseo/siba/internal/ast"
)

// ScopeKind distinguishes heading scopes from control block scopes
type ScopeKind int

const (
	ScopeHeading ScopeKind = iota
	ScopeControlBlock
)

// Scope represents a variable scope tied to a heading or control block
type Scope struct {
	Name      string
	Kind      ScopeKind
	Vars      map[string]*ast.Variable
	Parent    *Scope
	Children  []*Scope
	StartLine int // first line of this scope's content
	EndLine   int // last line of this scope's content
}

// NewScope creates a new scope
func NewScope(name string, kind ScopeKind, parent *Scope) *Scope {
	s := &Scope{
		Name:   name,
		Kind:   kind,
		Vars:   make(map[string]*ast.Variable),
		Parent: parent,
	}
	if parent != nil {
		parent.Children = append(parent.Children, s)
	}
	return s
}

// Declare adds a variable to this scope
func (s *Scope) Declare(name string, v ast.Variable) *ast.Diagnostic {
	// check duplicate in same scope
	if _, exists := s.Vars[name]; exists {
		return &ast.Diagnostic{
			Severity: ast.SeverityError,
			Code:     "E020",
			Message:  fmt.Sprintf("duplicate variable declaration: %s", name),
		}
	}

	// check const shadowing — const can never shadow anything
	if v.Mutability == ast.MutConst {
		if existing, _ := s.resolveUp(name); existing != nil {
			return &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E021",
				Message:  fmt.Sprintf("cannot shadow variable with const: %s", name),
			}
		}
	}

	// @let CAN shadow parent variables (including @const) in child scopes.
	// This is by design — @let is the mutable, shadowable keyword.

	vCopy := v
	s.Vars[name] = &vCopy
	return nil
}

// Resolve looks up a variable by walking up the scope chain
func (s *Scope) Resolve(name string) (*ast.Variable, bool) {
	return s.resolveUp(name)
}

func (s *Scope) resolveUp(name string) (*ast.Variable, bool) {
	if v, ok := s.Vars[name]; ok {
		return v, true
	}
	if s.Parent != nil {
		return s.Parent.resolveUp(name)
	}
	return nil, false
}

// BuildScopeTree constructs a scope tree from a document's heading tree
// and assigns variables to the correct heading scope based on position.
// Returns the root scope and any diagnostics from variable declarations.
func BuildScopeTree(doc *ast.Document) (*Scope, []ast.Diagnostic) {
	root := NewScope("__root__", ScopeHeading, nil)
	root.StartLine = 1
	root.EndLine = len(strings.Split(doc.Source, "\n"))

	// build heading scopes
	for _, h := range doc.Headings {
		buildHeadingScope(h, root)
	}

	// assign variables to correct scope based on line position
	var diags []ast.Diagnostic
	for _, v := range doc.Variables {
		target := findScopeForLine(root, v.Position.Line)
		if d := target.Declare(v.Name, v); d != nil {
			d.Range = ast.Range{Start: v.Position, End: v.Position}
			diags = append(diags, *d)
		}
	}

	return root, diags
}

// FindScopeForLine returns the most specific scope containing the given line
func FindScopeForLine(root *Scope, line int) *Scope {
	return findScopeForLine(root, line)
}

func findScopeForLine(s *Scope, line int) *Scope {
	// check children from last to first (deepest match wins)
	for i := len(s.Children) - 1; i >= 0; i-- {
		child := s.Children[i]
		if line >= child.StartLine && line <= child.EndLine {
			return findScopeForLine(child, line)
		}
	}
	return s
}

func buildHeadingScope(h *ast.Heading, parent *Scope) {
	name := h.Name
	if name == "" {
		name = h.Slug
	}
	s := NewScope(name, ScopeHeading, parent)
	s.StartLine = h.Position.Line
	s.EndLine = h.Content.End.Line

	for _, child := range h.Children {
		buildHeadingScope(child, s)
	}

	// Extend parent scope's EndLine to cover all children
	if len(h.Children) > 0 {
		lastChild := h.Children[len(h.Children)-1]
		lastChildEnd := lastChild.Content.End.Line
		// Also check if children have their own children
		for _, ch := range h.Children {
			if ch.Content.End.Line > lastChildEnd {
				lastChildEnd = ch.Content.End.Line
			}
		}
		if lastChildEnd > s.EndLine {
			s.EndLine = lastChildEnd
		}
	}
}
