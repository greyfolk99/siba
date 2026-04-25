package parser

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/greyfolk99/siba/internal/ast"
)

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// ParseHeadings extracts a flat list of headings from source
func ParseHeadings(source string) []*ast.Heading {
	var headings []*ast.Heading
	lines := strings.Split(source, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		matches := headingRe.FindStringSubmatch(trimmed)
		if matches == nil {
			continue
		}

		level := len(matches[1])
		text := strings.TrimSpace(matches[2])

		headings = append(headings, &ast.Heading{
			Level:    level,
			Text:     text,
			Slug:     GenerateSlug(text),
			Position: ast.Position{Line: i + 1, Column: 1},
		})
	}

	return headings
}

// BuildHeadingTree builds a parent-child tree from flat headings based on level
func BuildHeadingTree(headings []*ast.Heading) []*ast.Heading {
	if len(headings) == 0 {
		return nil
	}

	var roots []*ast.Heading
	var stack []*ast.Heading

	for _, h := range headings {
		// pop stack until we find a parent with lower level
		for len(stack) > 0 && stack[len(stack)-1].Level >= h.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			roots = append(roots, h)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, h)
		}

		stack = append(stack, h)
	}

	return roots
}

// GenerateSlug converts heading text to a GitHub-style slug
// Keeps Korean characters as-is
func GenerateSlug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('-')
		}
	}

	slug := b.String()
	// collapse multiple dashes
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	if slug == "" {
		slug = "heading"
	}
	return slug
}

// DeduplicateSlug adds numbering to duplicate slugs
func DeduplicateSlug(slug string, existing map[string]int) string {
	count, ok := existing[slug]
	if !ok {
		existing[slug] = 1
		return slug
	}
	existing[slug] = count + 1
	newSlug := fmt.Sprintf("%s-%d", slug, count)
	return newSlug
}
