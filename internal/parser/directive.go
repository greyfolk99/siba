package parser

import (
	"regexp"
	"strings"

	"github.com/hjseo/siba/internal/ast"
)

var directiveRe = regexp.MustCompile(`<!--\s*@(\w+)\s*(.*?)\s*-->`)
var multilineDirectiveRe = regexp.MustCompile(`(?s)<!--\s*@(\w+)\s*(.*?)\s*-->`)

// ParseDirectives extracts all <!-- @... --> directives from source
func ParseDirectives(source string) []ast.Directive {
	var directives []ast.Directive
	lines := strings.Split(source, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		matches := directiveRe.FindStringSubmatch(trimmed)
		if matches == nil {
			// handle multi-line directives (e.g., @const with arrays/objects)
			// check if line starts a directive that spans multiple lines
			if strings.Contains(trimmed, "<!--") && strings.Contains(trimmed, "@") && !strings.Contains(trimmed, "-->") {
				// collect until -->
				full := trimmed
				for j := i + 1; j < len(lines); j++ {
					full += "\n" + lines[j]
					if strings.Contains(lines[j], "-->") {
						break
					}
				}
				matches = multilineDirectiveRe.FindStringSubmatch(full)
				if matches != nil {
					kind, args := parseDirectiveInner(matches[1], matches[2])
					directives = append(directives, ast.Directive{
						Kind:     kind,
						Raw:      full,
						Args:     args,
						Position: ast.Position{Line: i + 1, Column: 1},
					})
				}
			}
			continue
		}

		kind, args := parseDirectiveInner(matches[1], matches[2])
		directives = append(directives, ast.Directive{
			Kind:     kind,
			Raw:      trimmed,
			Args:     args,
			Position: ast.Position{Line: i + 1, Column: 1},
		})
	}

	return directives
}

func parseDirectiveInner(keyword, args string) (ast.DirectiveKind, string) {
	args = strings.TrimSpace(args)
	switch keyword {
	case "doc":
		return ast.DirectiveDoc, args
	case "extends":
		return ast.DirectiveExtends, args
	case "template":
		return ast.DirectiveTemplate, args
	case "name":
		return ast.DirectiveName, args
	case "default":
		return ast.DirectiveDefault, args
	case "import":
		return ast.DirectiveImport, args
	case "const":
		return ast.DirectiveConst, args
	case "let":
		return ast.DirectiveLet, args
	case "if":
		return ast.DirectiveIf, args
	case "endif":
		return ast.DirectiveEndif, args
	case "for":
		return ast.DirectiveFor, args
	case "endfor":
		return ast.DirectiveEndfor, args
	default:
		return ast.DirectiveDoc, args // fallback
	}
}

// IsDirectiveLine checks if a line is a directive
func IsDirectiveLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return directiveRe.MatchString(trimmed)
}

// ExtractAnnotation finds the annotation from directives immediately above a heading
func ExtractAnnotation(directives []ast.Directive, headingLine int) ast.Annotation {
	for _, d := range directives {
		if d.Position.Line == headingLine-1 {
			switch d.Kind {
			case ast.DirectiveDefault:
				return ast.AnnotationDefault
			}
		}
	}
	return ast.AnnotationRequired
}
