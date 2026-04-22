package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/parser"
	"github.com/hjseo/siba/internal/pkg"
	"github.com/hjseo/siba/internal/render"
	"github.com/hjseo/siba/internal/scripts"
	"github.com/hjseo/siba/internal/validate"
	"github.com/hjseo/siba/internal/workspace"
)

// hasFlag checks if a flag is present in os.Args
func hasFlag(flag string) bool {
	for _, arg := range os.Args {
		if arg == flag {
			return true
		}
	}
	return false
}

// argsWithout returns os.Args[start:] with specified flags removed
func argsWithout(start int, flags ...string) []string {
	skip := make(map[string]bool)
	for _, f := range flags {
		skip[f] = true
	}
	var result []string
	for _, arg := range os.Args[start:] {
		if !skip[arg] {
			result = append(result, arg)
		}
	}
	return result
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	jsonMode := hasFlag("--json")

	switch os.Args[1] {
	case "init":
		runInit()
	case "render":
		runRender(jsonMode)
	case "check":
		args := argsWithout(2, "--json")
		if len(args) == 0 {
			runCheckWorkspace(jsonMode)
		} else {
			runCheck(args[0], jsonMode)
		}
	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: siba get <package-url> [version]")
			os.Exit(1)
		}
		version := "main"
		if len(os.Args) >= 4 {
			version = os.Args[3]
		}
		runGet(os.Args[2], version)
	case "tidy":
		runTidy()
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: siba run <script-name>")
			os.Exit(1)
		}
		runScript(os.Args[2])
	case "graph":
		runGraph(jsonMode)
	case "version":
		fmt.Println("siba v0.1.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SIBA — Structured Ink for Building Archives")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  siba init                      Initialize a new project")
	fmt.Println("  siba render [file.md]           Render documents")
	fmt.Println("  siba render -o <dir>            Render to custom output directory")
	fmt.Println("  siba check [file.md]            Check document(s) for errors")
	fmt.Println("  siba get <pkg> [version]        Add a dependency")
	fmt.Println("  siba tidy                       Remove unused dependencies")
	fmt.Println("  siba run <script>               Run a script from module.toml")
	fmt.Println("  siba graph                      Show dependency/reference graph")
	fmt.Println("  siba version                    Show version")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --json                          Output in JSON format")
}

// --- JSON output types ---

// JSONDiagnostic is a diagnostic in JSON output
type JSONDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	EndLine  int    `json:"end_line"`
	EndCol   int    `json:"end_column"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// JSONCheckResult is the result of siba check --json
type JSONCheckResult struct {
	File        string           `json:"file,omitempty"`
	DocName     string           `json:"doc_name,omitempty"`
	ExtendsName string           `json:"extends,omitempty"`
	IsTemplate  bool             `json:"is_template"`
	Variables   int              `json:"variables"`
	References  int              `json:"references"`
	Headings    int              `json:"headings"`
	Errors      int              `json:"errors"`
	Warnings    int              `json:"warnings"`
	Diagnostics []JSONDiagnostic `json:"diagnostics"`
}

// JSONCheckWorkspaceResult is the result of workspace-wide check
type JSONCheckWorkspaceResult struct {
	Root        string            `json:"root"`
	Version     string            `json:"version"`
	Documents   int               `json:"documents"`
	Templates   int               `json:"templates"`
	TotalErrors int               `json:"total_errors"`
	TotalWarns  int               `json:"total_warnings"`
	Files       []JSONCheckResult `json:"files"`
	Workspace   []JSONDiagnostic  `json:"workspace_diagnostics"`
}

// JSONRenderResult is the result of siba render --json
type JSONRenderResult struct {
	File    string `json:"file"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

func toJSONDiagnostic(d ast.Diagnostic) JSONDiagnostic {
	sev := "error"
	switch d.Severity {
	case ast.SeverityWarning:
		sev = "warning"
	case ast.SeverityInfo:
		sev = "info"
	case ast.SeverityHint:
		sev = "hint"
	}
	return JSONDiagnostic{
		File:     d.File,
		Line:     d.Range.Start.Line,
		Column:   d.Range.Start.Column,
		EndLine:  d.Range.End.Line,
		EndCol:   d.Range.End.Column,
		Severity: sev,
		Code:     d.Code,
		Message:  d.Message,
	}
}

// JSONGraphResult is the result of siba graph --json
type JSONGraphResult struct {
	Nodes []JSONGraphNode `json:"nodes"`
	Edges []JSONGraphEdge `json:"edges"`
}

// JSONGraphNode represents a document in the graph
type JSONGraphNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsTemplate bool   `json:"is_template"`
	Variables  int    `json:"variables"`
	Headings   int    `json:"headings"`
}

// JSONGraphEdge represents a relationship between documents
type JSONGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "extends", "ref", "variable"
}

func writeJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func runInit() {
	cwd, _ := os.Getwd()
	name := filepath.Base(cwd)

	if _, err := os.Stat(filepath.Join(cwd, "module.toml")); err == nil {
		fmt.Println("module.toml already exists")
		return
	}

	if err := workspace.InitModuleToml(cwd, name); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("initialized siba project: %s\n", name)
	fmt.Println("created module.toml")
}

func runRender(jsonMode bool) {
	args := argsWithout(2, "--json", "-o")

	// find -o flag value
	outputDir := ""
	for i, arg := range os.Args {
		if arg == "-o" && i+1 < len(os.Args) {
			outputDir = os.Args[i+1]
		}
	}

	// Single file render
	if len(args) > 0 && args[0] != "-o" {
		renderSingleFile(args[0], jsonMode)
		return
	}

	// Workspace render
	cwd, _ := os.Getwd()
	w, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		if jsonMode {
			writeJSON(JSONRenderResult{Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "error loading workspace: %v\n", err)
		}
		os.Exit(1)
	}

	if err := scripts.RunPrerender(w.Config); err != nil {
		fmt.Fprintf(os.Stderr, "prerender failed: %v\n", err)
		os.Exit(1)
	}

	version := w.GetVersion()
	if !jsonMode {
		fmt.Printf("rendering v%s...\n", version)
	}

	if err := render.RenderWorkspace(w, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "render failed: %v\n", err)
		os.Exit(1)
	}

	if err := scripts.RunPostrender(w.Config); err != nil {
		fmt.Fprintf(os.Stderr, "postrender failed: %v\n", err)
		os.Exit(1)
	}

	if !jsonMode {
		fmt.Printf("render complete: v%s\n", version)
	}
}

func renderSingleFile(path string, jsonMode bool) {
	source, err := os.ReadFile(path)
	if err != nil {
		if jsonMode {
			writeJSON(JSONRenderResult{File: path, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	doc := parser.ParseDocument(path, string(source))

	hasErrors := false
	for _, d := range doc.Diagnostics {
		if d.Severity == ast.SeverityError {
			if !jsonMode {
				fmt.Fprintf(os.Stderr, "%s:%d: error: %s\n", path, d.Range.Start.Line, d.Message)
			}
			hasErrors = true
		}
	}

	if hasErrors && jsonMode {
		var jdiags []JSONDiagnostic
		for _, d := range doc.Diagnostics {
			jdiags = append(jdiags, toJSONDiagnostic(d))
		}
		writeJSON(JSONRenderResult{File: path, Error: "document has errors"})
		os.Exit(1)
	}
	if hasErrors {
		os.Exit(1)
	}

	// Load workspace for cross-doc refs and @default inheritance
	cwd, _ := os.Getwd()
	ws, _ := workspace.LoadWorkspace(cwd)

	output, err := render.RenderWithWorkspace(doc, ws)
	if err != nil {
		if jsonMode {
			writeJSON(JSONRenderResult{File: path, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
		}
		os.Exit(1)
	}

	if jsonMode {
		writeJSON(JSONRenderResult{File: path, Content: output})
	} else {
		fmt.Print(output)
	}
}

func runCheck(path string, jsonMode bool) {
	source, err := os.ReadFile(path)
	if err != nil {
		if jsonMode {
			writeJSON(JSONCheckResult{File: path, Diagnostics: []JSONDiagnostic{}})
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	doc := parser.ParseDocument(path, string(source))

	// also run validate with workspace context if available
	cwd, _ := os.Getwd()
	ws, _ := workspace.LoadWorkspace(cwd)
	allDiags := doc.Diagnostics
	allDiags = append(allDiags, validate.ValidateDocument(doc, ws)...)

	errorCount := 0
	warnCount := 0
	var jdiags []JSONDiagnostic

	for _, d := range allDiags {
		jd := toJSONDiagnostic(d)
		if jd.File == "" {
			jd.File = path
		}
		jdiags = append(jdiags, jd)
		switch d.Severity {
		case ast.SeverityError:
			errorCount++
		case ast.SeverityWarning:
			warnCount++
		}
	}

	if jsonMode {
		if jdiags == nil {
			jdiags = []JSONDiagnostic{}
		}
		writeJSON(JSONCheckResult{
			File:        path,
			DocName:     doc.Name,
			ExtendsName: doc.ExtendsName,
			IsTemplate:  doc.IsTemplate,
			Variables:   len(doc.Variables),
			References:  len(doc.References),
			Headings:    countHeadings(doc.Headings),
			Errors:      errorCount,
			Warnings:    warnCount,
			Diagnostics: jdiags,
		})
		if errorCount > 0 {
			os.Exit(1)
		}
		return
	}

	for _, d := range allDiags {
		switch d.Severity {
		case ast.SeverityError:
			fmt.Fprintf(os.Stderr, "\033[31merror\033[0m[%s]: %s (line %d)\n", d.Code, d.Message, d.Range.Start.Line)
		case ast.SeverityWarning:
			fmt.Fprintf(os.Stderr, "\033[33mwarn\033[0m[%s]: %s (line %d)\n", d.Code, d.Message, d.Range.Start.Line)
		}
	}

	if errorCount == 0 && warnCount == 0 {
		fmt.Printf("\033[32m✓\033[0m %s: no issues found\n", path)
		fmt.Printf("  doc: %s\n", doc.Name)
		if doc.ExtendsName != "" {
			fmt.Printf("  extends: %s\n", doc.ExtendsName)
		}
		fmt.Printf("  variables: %d\n", len(doc.Variables))
		fmt.Printf("  references: %d\n", len(doc.References))
		fmt.Printf("  headings: %d\n", countHeadings(doc.Headings))
	} else {
		fmt.Printf("\n%d error(s), %d warning(s)\n", errorCount, warnCount)
		if errorCount > 0 {
			os.Exit(1)
		}
	}
}

func runCheckWorkspace(jsonMode bool) {
	cwd, _ := os.Getwd()
	ws, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		if jsonMode {
			writeJSON(JSONCheckWorkspaceResult{Root: cwd})
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	fileDiags, wsDiags := validate.ValidateWorkspace(ws)
	allFileDiags := validate.AllDiagnostics(fileDiags, nil)

	totalErrors := 0
	totalWarns := 0

	var files []JSONCheckResult
	for path, doc := range ws.DocsByPath {
		diags := fileDiags[path]
		// include parser diagnostics
		diags = append(diags, doc.Diagnostics...)

		errCount := 0
		warnCount := 0
		var jdiags []JSONDiagnostic
		for _, d := range diags {
			jd := toJSONDiagnostic(d)
			if jd.File == "" {
				jd.File = path
			}
			jdiags = append(jdiags, jd)
			switch d.Severity {
			case ast.SeverityError:
				errCount++
			case ast.SeverityWarning:
				warnCount++
			}
		}
		totalErrors += errCount
		totalWarns += warnCount

		if jdiags == nil {
			jdiags = []JSONDiagnostic{}
		}
		files = append(files, JSONCheckResult{
			File:        path,
			DocName:     doc.Name,
			ExtendsName: doc.ExtendsName,
			IsTemplate:  doc.IsTemplate,
			Variables:   len(doc.Variables),
			References:  len(doc.References),
			Headings:    countHeadings(doc.Headings),
			Errors:      errCount,
			Warnings:    warnCount,
			Diagnostics: jdiags,
		})
	}

	// workspace-level diagnostics
	var wsJDiags []JSONDiagnostic
	for _, d := range wsDiags {
		wsJDiags = append(wsJDiags, toJSONDiagnostic(d))
		if d.Severity == ast.SeverityError {
			totalErrors++
		}
	}
	if wsJDiags == nil {
		wsJDiags = []JSONDiagnostic{}
	}

	if jsonMode {
		writeJSON(JSONCheckWorkspaceResult{
			Root:        cwd,
			Version:     ws.GetVersion(),
			Documents:   len(ws.DocsByPath),
			Templates:   len(ws.Templates),
			TotalErrors: totalErrors,
			TotalWarns:  totalWarns,
			Files:       files,
			Workspace:   wsJDiags,
		})
		if totalErrors > 0 {
			os.Exit(1)
		}
		return
	}

	// text output
	for _, d := range allFileDiags {
		switch d.Severity {
		case ast.SeverityError:
			fmt.Fprintf(os.Stderr, "\033[31merror\033[0m[%s] %s: %s (line %d)\n", d.Code, d.File, d.Message, d.Range.Start.Line)
		case ast.SeverityWarning:
			fmt.Fprintf(os.Stderr, "\033[33mwarn\033[0m[%s] %s: %s (line %d)\n", d.Code, d.File, d.Message, d.Range.Start.Line)
		}
	}
	for _, d := range wsDiags {
		fmt.Fprintf(os.Stderr, "\033[31merror\033[0m[%s]: %s\n", d.Code, d.Message)
	}

	fmt.Printf("\n%d document(s), %d template(s)\n", len(ws.DocsByPath), len(ws.Templates))
	fmt.Printf("%d error(s), %d warning(s)\n", totalErrors, totalWarns)
	if totalErrors > 0 {
		os.Exit(1)
	}
}

func runGet(pkgURL string, version string) {
	cwd, _ := os.Getwd()
	configPath := filepath.Join(cwd, "module.toml")
	config, err := workspace.ParseModuleToml(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v (run 'siba init' first)\n", err)
		os.Exit(1)
	}

	mgr := pkg.NewManager(cwd, config)
	if err := mgr.Add(pkgURL, version); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("added %s@%s\n", pkgURL, version)
}

func runTidy() {
	cwd, _ := os.Getwd()
	w, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	usedPkgs := make(map[string]bool)
	for _, doc := range w.DocsByPath {
		if doc.ExtendsName != "" && isPackageRef(doc.ExtendsName) {
			usedPkgs[extractPackageName(doc.ExtendsName)] = true
		}
		for _, ref := range doc.References {
			if isPackageRef(ref.PathPart) {
				usedPkgs[extractPackageName(ref.PathPart)] = true
			}
		}
	}

	mgr := pkg.NewManager(cwd, w.Config)
	if err := mgr.Tidy(usedPkgs); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("tidy complete")
}

func runScript(name string) {
	cwd, _ := os.Getwd()
	configPath := filepath.Join(cwd, "module.toml")
	config, err := workspace.ParseModuleToml(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := scripts.RunScript(name, config); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func countHeadings(headings []*ast.Heading) int {
	count := len(headings)
	for _, h := range headings {
		count += countHeadings(h.Children)
	}
	return count
}

func isPackageRef(s string) bool {
	parts := 0
	for _, c := range s {
		if c == '/' {
			parts++
		}
	}
	return parts >= 2
}

func extractPackageName(s string) string {
	parts := splitPath(s)
	if len(parts) >= 3 {
		return parts[0] + "/" + parts[1] + "/" + parts[2]
	}
	return s
}

func splitPath(s string) []string {
	var parts []string
	current := ""
	for _, c := range s {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func runGraph(jsonMode bool) {
	cwd, _ := os.Getwd()
	ws, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Build nodes
	var nodes []JSONGraphNode
	docIDs := make(map[string]string) // path → id

	for path, doc := range ws.DocsByPath {
		id := path
		if doc.Name != "" {
			id = doc.Name
		}
		docIDs[path] = id

		nodes = append(nodes, JSONGraphNode{
			ID:         id,
			Name:       doc.Name,
			Path:       path,
			IsTemplate: doc.IsTemplate,
			Variables:  len(doc.Variables),
			Headings:   countHeadings(doc.Headings),
		})
	}

	// Build edges
	var edges []JSONGraphEdge

	for path, doc := range ws.DocsByPath {
		sourceID := docIDs[path]

		// @extends → "extends" edge
		if doc.ExtendsName != "" {
			edges = append(edges, JSONGraphEdge{
				Source: sourceID,
				Target: doc.ExtendsName,
				Type:   "extends",
			})
		}

		// {{doc-name}} references
		for _, ref := range doc.References {
			if ref.IsEscaped {
				continue
			}

			// document content insertion: {{doc-name}} or {{doc-name#section}}
			if ref.PathPart != "" && ref.Variable == "" {
				if targetDoc := ws.GetDocument(ref.PathPart); targetDoc != nil {
					edgeType := "ref"
					if ref.Section != "" {
						edgeType = "section_ref"
					}
					edges = append(edges, JSONGraphEdge{
						Source: sourceID,
						Target: ref.PathPart,
						Type:   edgeType,
					})
				}
			}

			// variable reference: {{doc-name.variable}}
			if ref.PathPart != "" && ref.Variable != "" {
				if targetDoc := ws.GetDocument(ref.PathPart); targetDoc != nil {
					edges = append(edges, JSONGraphEdge{
						Source: sourceID,
						Target: ref.PathPart,
						Type:   "variable_ref",
					})
				}
			}
		}
	}

	result := JSONGraphResult{
		Nodes: nodes,
		Edges: edges,
	}

	if jsonMode {
		writeJSON(result)
		return
	}

	// DOT output (default)
	fmt.Println("digraph siba {")
	fmt.Println("  rankdir=LR;")
	fmt.Println("  node [shape=box, style=rounded, fontname=\"sans-serif\"];")
	fmt.Println()

	for _, n := range nodes {
		shape := "box"
		if n.IsTemplate {
			shape = "diamond"
		}
		label := n.ID
		if n.Name != "" && n.Name != n.Path {
			label = n.Name
		}
		fmt.Printf("  %q [label=%q, shape=%s];\n", n.ID, label, shape)
	}

	fmt.Println()

	for _, e := range edges {
		style := "solid"
		color := "black"
		label := ""
		switch e.Type {
		case "extends":
			style = "bold"
			color = "blue"
			label = "extends"
		case "ref":
			color = "gray40"
		case "section_ref":
			color = "gray60"
			style = "dashed"
			label = "#"
		case "variable_ref":
			color = "orange"
			style = "dotted"
			label = "."
		}
		attrs := fmt.Sprintf("style=%s, color=%s", style, color)
		if label != "" {
			attrs += fmt.Sprintf(", label=%q", label)
		}
		fmt.Printf("  %q -> %q [%s];\n", e.Source, e.Target, attrs)
	}

	fmt.Println("}")
}
