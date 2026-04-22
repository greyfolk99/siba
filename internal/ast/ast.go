package ast

import (
	"fmt"
	"strings"
)

// Position represents a line/column position in source
type Position struct {
	Line   int
	Column int
}

// Range represents a start-end range in source
type Range struct {
	Start Position
	End   Position
}

// Severity for diagnostics
type Severity int

const (
	SeverityError   Severity = iota
	SeverityWarning
	SeverityInfo
	SeverityHint
)

// Diagnostic represents a diagnostic message
type Diagnostic struct {
	File     string
	Range    Range
	Severity Severity
	Code     string
	Message  string
}

// DirectiveKind enumerates directive types
type DirectiveKind int

const (
	DirectiveDoc DirectiveKind = iota
	DirectiveExtends
	DirectiveTemplate
	DirectiveName
	DirectiveDefault
	DirectiveConst
	DirectiveLet
	DirectiveImport
	DirectiveIf
	DirectiveEndif
	DirectiveFor
	DirectiveEndfor
)

// Directive represents a parsed <!-- @... --> directive
type Directive struct {
	Kind     DirectiveKind
	Raw      string
	Args     string
	Position Position
}

// Annotation for headings
type Annotation int

const (
	AnnotationRequired Annotation = iota // default
	AnnotationDefault
)

// AccessLevel for variables
type AccessLevel int

const (
	AccessDefault AccessLevel = iota // visible to extends children + same file
	AccessPrivate                    // not visible to extends children
)

// Mutability for variables
type Mutability int

const (
	MutConst Mutability = iota
	MutLet
)

// TypeKind enumerates type kinds
type TypeKind int

const (
	TypeString TypeKind = iota
	TypeNumber
	TypeBoolean
	TypeArray
	TypeObject
	TypeUnion
	TypeNull
	TypeAny
)

// TypeExpr represents a type expression
type TypeExpr struct {
	Kind         TypeKind
	ElementType  *TypeExpr            // for arrays
	Fields       map[string]*TypeExpr // for objects
	UnionMembers []*TypeExpr          // for unions
}

// Value represents a literal value
type Value struct {
	Kind     TypeKind
	Str      string
	Num      float64
	Bool     bool
	Array    []Value
	Object   map[string]Value
	IsNull   bool
	Raw      string
}

// Variable represents a variable declaration
type Variable struct {
	Name       string
	Type       *TypeExpr
	Value      *Value
	Mutability Mutability
	Access     AccessLevel
	Position   Position
}

// Heading represents a heading node in the document tree
type Heading struct {
	Level      int
	Text       string
	Slug       string
	Name       string // from @name directive, empty if none
	Annotation Annotation
	Children   []*Heading
	Content    Range // content range after heading until next heading
	Position   Position
}

// Reference represents a {{}} reference
type Reference struct {
	Raw       string
	PathPart  string // before #
	Section   string // after #, before .
	Variable  string // after .
	IsEscaped bool   // \{{ prefix
	Position  Position
}

// ControlBlock represents an @if or @for block
type ControlBlock struct {
	Kind      DirectiveKind // DirectiveIf or DirectiveFor
	Condition string        // for @if
	Iterator  string        // for @for: variable name
	Collection string       // for @for: collection name
	Start     Position
	End       Position
}

// Import represents an @import directive
type Import struct {
	Alias    string   // short name
	Path     string   // file path or package URL
	Position Position
}

// Document represents a fully parsed document
type Document struct {
	Path          string
	Name          string // from @doc or @template
	ExtendsName   string // from @extends (can include #symbol)
	IsTemplate    bool   // has @template
	Imports       []Import
	Headings      []*Heading
	Directives    []Directive
	Variables     []Variable
	References    []Reference
	ControlBlocks []ControlBlock
	Source        string
	Diagnostics   []Diagnostic
}

// TypeEquals checks type equality
func TypeEquals(a, b *TypeExpr) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case TypeArray:
		return TypeEquals(a.ElementType, b.ElementType)
	case TypeObject:
		if len(a.Fields) != len(b.Fields) {
			return false
		}
		for k, v := range a.Fields {
			bv, ok := b.Fields[k]
			if !ok || !TypeEquals(v, bv) {
				return false
			}
		}
		return true
	case TypeUnion:
		if len(a.UnionMembers) != len(b.UnionMembers) {
			return false
		}
		for i, m := range a.UnionMembers {
			if !TypeEquals(m, b.UnionMembers[i]) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// IsAssignable checks if source is assignable to target
func IsAssignable(target, source *TypeExpr) bool {
	if target == nil || target.Kind == TypeAny {
		return true
	}
	if source == nil {
		return false
	}
	if TypeEquals(target, source) {
		return true
	}
	if target.Kind == TypeUnion {
		for _, m := range target.UnionMembers {
			if IsAssignable(m, source) {
				return true
			}
		}
	}
	return false
}

// TypeToString converts a type to its string representation
func TypeToString(t *TypeExpr) string {
	if t == nil {
		return "any"
	}
	switch t.Kind {
	case TypeString:
		return "string"
	case TypeNumber:
		return "number"
	case TypeBoolean:
		return "boolean"
	case TypeNull:
		return "null"
	case TypeAny:
		return "any"
	case TypeArray:
		return TypeToString(t.ElementType) + "[]"
	case TypeObject:
		s := "{ "
		first := true
		for k, v := range t.Fields {
			if !first {
				s += ", "
			}
			s += k + ": " + TypeToString(v)
			first = false
		}
		return s + " }"
	case TypeUnion:
		s := ""
		for i, m := range t.UnionMembers {
			if i > 0 {
				s += " | "
			}
			s += TypeToString(m)
		}
		return s
	default:
		return "unknown"
	}
}

// ValueToString converts a Value to its display string
func ValueToString(v Value) string {
	switch v.Kind {
	case TypeString:
		return v.Str
	case TypeNumber:
		if v.Raw != "" {
			// only trim trailing zeros after decimal point
			if strings.Contains(v.Raw, ".") {
				return strings.TrimRight(strings.TrimRight(v.Raw, "0"), ".")
			}
			return v.Raw
		}
		// fallback when Raw is empty
		if v.Num == float64(int64(v.Num)) {
			return fmt.Sprintf("%d", int64(v.Num))
		}
		return fmt.Sprintf("%g", v.Num)
	case TypeBoolean:
		if v.Bool {
			return "true"
		}
		return "false"
	case TypeNull:
		return "null"
	default:
		if v.Raw != "" {
			return v.Raw
		}
		return ""
	}
}
