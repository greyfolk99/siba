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
	VarLines  map[string]int // variable name → declaration line (for TDZ)
	Parent    *Scope
	Children  []*Scope
	StartLine int // first line of this scope's content
	EndLine   int // last line of this scope's content
}

// NewScope creates a new scope
func NewScope(name string, kind ScopeKind, parent *Scope) *Scope {
	s := &Scope{
		Name:     name,
		Kind:     kind,
		Vars:     make(map[string]*ast.Variable),
		VarLines: make(map[string]int),
		Parent:   parent,
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

	// @const can NEVER shadow a parent variable (const is immutable + no shadowing)
	if v.Mutability == ast.MutConst {
		if existing, _ := s.resolveUp(name); existing != nil {
			return &ast.Diagnostic{
				Severity: ast.SeverityError,
				Code:     "E021",
				Message:  fmt.Sprintf("cannot shadow variable with const: %s", name),
			}
		}
	}

	// @let CAN redeclare in child scopes (each child gets its own value).
	// This is standard block scoping — like JS let in nested blocks.

	vCopy := v
	s.Vars[name] = &vCopy
	s.VarLines[name] = v.Position.Line
	return nil
}

// Resolve looks up a variable by walking up the scope chain (no TDZ check)
func (s *Scope) Resolve(name string) (*ast.Variable, bool) {
	return s.resolveUp(name)
}

// ResolveAt looks up a variable with TDZ check — returns nil if referenced before declaration line
func (s *Scope) ResolveAt(name string, atLine int) (*ast.Variable, bool) {
	// Check this scope first
	if v, ok := s.Vars[name]; ok {
		declLine := s.VarLines[name]
		if atLine < declLine {
			return nil, false // TDZ — referenced before declaration
		}
		return v, true
	}
	// Walk up to parent
	if s.Parent != nil {
		return s.Parent.ResolveAt(name, atLine)
	}
	return nil, false
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

	// Extend EndLine to cover all child scopes (recursively expanded)
	for _, childScope := range s.Children {
		if childScope.EndLine > s.EndLine {
			s.EndLine = childScope.EndLine
		}
	}
}
