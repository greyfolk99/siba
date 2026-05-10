package refs

import (
	"fmt"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/workspace"
)

// IncrementalGraph maintains a forward + reverse adjacency view of the
// workspace's reference graph. Updating a single doc costs O(deg(node))
// rather than the O(N) full rescan that BuildDependencyGraphs does.
//
// Used by long-running consumers (siba-lsp, siba-wiki-web daemon) that
// receive fsnotify events: on file change, call UpdateDoc with the freshly
// parsed *ast.Document and only the affected edges are touched.
type IncrementalGraph struct {
	Forward map[string]map[string]bool // source → set of targets
	Reverse map[string]map[string]bool // target → set of sources
}

// NewIncrementalGraph returns an empty graph.
func NewIncrementalGraph() *IncrementalGraph {
	return &IncrementalGraph{
		Forward: make(map[string]map[string]bool),
		Reverse: make(map[string]map[string]bool),
	}
}

// AddEdge records source → target.
func (g *IncrementalGraph) AddEdge(source, target string) {
	if _, ok := g.Forward[source]; !ok {
		g.Forward[source] = make(map[string]bool)
	}
	if _, ok := g.Reverse[target]; !ok {
		g.Reverse[target] = make(map[string]bool)
	}
	g.Forward[source][target] = true
	g.Reverse[target][source] = true
}

// RemoveEdge removes source → target if present.
func (g *IncrementalGraph) RemoveEdge(source, target string) {
	if m, ok := g.Forward[source]; ok {
		delete(m, target)
		if len(m) == 0 {
			delete(g.Forward, source)
		}
	}
	if m, ok := g.Reverse[target]; ok {
		delete(m, source)
		if len(m) == 0 {
			delete(g.Reverse, target)
		}
	}
}

// UpdateOutgoing replaces the set of targets that source points to. Any old
// edges that aren't in newTargets are removed; new ones are added. This is
// the canonical "doc just changed" update.
func (g *IncrementalGraph) UpdateOutgoing(source string, newTargets []string) {
	old := g.Forward[source]
	target := make(map[string]bool, len(newTargets))
	for _, t := range newTargets {
		target[t] = true
	}
	// remove edges that disappeared
	for t := range old {
		if !target[t] {
			g.RemoveEdge(source, t)
		}
	}
	// add new edges
	for t := range target {
		if !old[t] {
			g.AddEdge(source, t)
		}
	}
}

// RemoveNode drops every edge involving node (both directions). Use when a
// document is deleted from the workspace.
func (g *IncrementalGraph) RemoveNode(node string) {
	for t := range g.Forward[node] {
		if m, ok := g.Reverse[t]; ok {
			delete(m, node)
			if len(m) == 0 {
				delete(g.Reverse, t)
			}
		}
	}
	delete(g.Forward, node)
	for s := range g.Reverse[node] {
		if m, ok := g.Forward[s]; ok {
			delete(m, node)
			if len(m) == 0 {
				delete(g.Forward, s)
			}
		}
	}
	delete(g.Reverse, node)
}

// Backlinks returns the set of sources that reference target. O(1).
func (g *IncrementalGraph) Backlinks(target string) []string {
	if m, ok := g.Reverse[target]; ok {
		out := make([]string, 0, len(m))
		for s := range m {
			out = append(out, s)
		}
		return out
	}
	return nil
}

// Targets returns the set of targets that source points to. O(1).
func (g *IncrementalGraph) Targets(source string) []string {
	if m, ok := g.Forward[source]; ok {
		out := make([]string, 0, len(m))
		for t := range m {
			out = append(out, t)
		}
		return out
	}
	return nil
}

// HasPath checks whether there's a directed path from source to target.
// Used for incremental cycle detection: before adding edge (u → v), call
// HasPath(v, u) — if true the new edge would create a cycle.
//
// O(reachable from source) using DFS.
func (g *IncrementalGraph) HasPath(source, target string) bool {
	if source == target {
		return true
	}
	visited := make(map[string]bool)
	var dfs func(node string) bool
	dfs = func(node string) bool {
		if node == target {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		for next := range g.Forward[node] {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(source)
}

// IncrementalGraphs bundles the three graph kinds (extends, embed, link).
// Each is independent — adding an embed edge doesn't touch extends.
type IncrementalGraphs struct {
	Extends *IncrementalGraph
	Embed   *IncrementalGraph
	Link    *IncrementalGraph
}

// NewIncrementalGraphs returns an empty bundle.
func NewIncrementalGraphs() *IncrementalGraphs {
	return &IncrementalGraphs{
		Extends: NewIncrementalGraph(),
		Embed:   NewIncrementalGraph(),
		Link:    NewIncrementalGraph(),
	}
}

// BuildIncrementalGraphs walks every document once and populates all three
// graphs. Equivalent to the existing BuildDependencyGraphs but with reverse
// indexes for O(1) backlink queries and O(deg) updates.
//
// A nil workspace returns an empty graph rather than panicking.
func BuildIncrementalGraphs(ws *workspace.Workspace) *IncrementalGraphs {
	if ws == nil {
		return NewIncrementalGraphs()
	}
	g := NewIncrementalGraphs()
	for _, doc := range ws.DocsByPath {
		g.UpdateDoc(ws, doc)
	}
	return g
}

// UpdateDoc refreshes every edge originating at doc. Call this from a file
// watcher (or LSP didSave) after re-parsing the document. No-op if either
// argument is nil.
func (g *IncrementalGraphs) UpdateDoc(ws *workspace.Workspace, doc *ast.Document) {
	if ws == nil || doc == nil {
		return
	}
	id := docID(doc)

	// extends — single edge max
	var extTargets []string
	if doc.ExtendsName != "" {
		extTargets = []string{doc.ExtendsName}
	}
	g.Extends.UpdateOutgoing(id, extTargets)

	// embed + link — walk references
	var embTargets, linkTargets []string
	seen := make(map[string]bool) // dedupe per kind
	seenLink := make(map[string]bool)
	for _, ref := range doc.References {
		if ref.IsEscaped || ref.PathPart == "" {
			continue
		}
		target := resolveRefTarget(ws, doc, ref)
		if target == "" {
			continue
		}
		if ref.IsLink {
			if !seenLink[target] {
				seenLink[target] = true
				linkTargets = append(linkTargets, target)
			}
		} else {
			if !seen[target] {
				seen[target] = true
				embTargets = append(embTargets, target)
			}
		}
	}
	g.Embed.UpdateOutgoing(id, embTargets)
	g.Link.UpdateOutgoing(id, linkTargets)
}

// RemoveDoc drops every trace of doc from all three graphs. No-op if doc nil.
func (g *IncrementalGraphs) RemoveDoc(doc *ast.Document) {
	if doc == nil {
		return
	}
	id := docID(doc)
	g.Extends.RemoveNode(id)
	g.Embed.RemoveNode(id)
	g.Link.RemoveNode(id)
}

func docID(doc *ast.Document) string {
	if doc.Name != "" {
		return doc.Name
	}
	return doc.Path
}

func resolveRefTarget(ws *workspace.Workspace, doc *ast.Document, ref ast.Reference) string {
	// alias-prefixed: {{alias}} / {{alias#sec}} / [[alias]]
	for _, imp := range doc.Imports {
		if imp.Alias == ref.PathPart {
			target := ws.ResolveImportDoc(imp.Path, doc.Path)
			if target != nil {
				return docID(target)
			}
		}
	}
	// plain {{doc-name}}
	if !strings.Contains(ref.PathPart, "/") {
		if d := ws.GetDocument(ref.PathPart); d != nil {
			return ref.PathPart
		}
	}
	return ""
}

// DetectCyclesIncremental returns cycles present in g, using DFS. Same output
// shape as DetectExtendsCycles / DetectEmbedCycles, but works on the
// IncrementalGraph. Code is the diagnostic code to emit (E060 / E022 / etc).
func DetectCyclesIncremental(g *IncrementalGraph, code, label string) []ast.Diagnostic {
	var diags []ast.Diagnostic
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		inStack[node] = true
		for next := range g.Forward[node] {
			if !visited[next] {
				dfs(next, append(append([]string{}, path...), next))
			} else if inStack[next] {
				// trim path to the cycle proper — drop nodes before next reappears
				cycleStart := 0
				for i, n := range path {
					if n == next {
						cycleStart = i
						break
					}
				}
				cycle := append(append([]string{}, path[cycleStart:]...), next)
				diags = append(diags, ast.Diagnostic{
					Severity: ast.SeverityError,
					Code:     code,
					Message:  fmt.Sprintf("%s: %s", label, strings.Join(cycle, " → ")),
				})
			}
		}
		inStack[node] = false
	}
	for n := range g.Forward {
		if !visited[n] {
			dfs(n, []string{n})
		}
	}
	return diags
}
