package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/parser"
	"github.com/greyfolk99/siba/pkg/pkg"
	"github.com/greyfolk99/siba/pkg/render"
	"github.com/greyfolk99/siba/pkg/scripts"
	"github.com/greyfolk99/siba/pkg/validate"
	"github.com/greyfolk99/siba/pkg/workspace"
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
		os.Exit(2)
	}

	jsonMode := hasFlag("--json")

	rawMode := hasFlag("--raw")
	args := argsWithout(2, "--json", "--raw")

	switch os.Args[1] {
	// Read commands (render + streaming)
	case "cat":
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "usage: siba cat <file.md[#symbol]>")
			os.Exit(2)
		}
		runCat(args[0], rawMode)
	case "head":
		n := 10
		c := 0
		file := ""
		for i, a := range args {
			if a == "-n" && i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &n)
			} else if a == "-c" && i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &c)
			} else if !strings.HasPrefix(a, "-") {
				file = a
			}
		}
		if file == "" {
			fmt.Fprintln(os.Stderr, "usage: siba head [-n N|-c N] <file.md[#symbol]>")
			os.Exit(2)
		}
		if c > 0 {
			runHeadBytes(file, c, rawMode)
		} else {
			runHead(file, n, rawMode)
		}
	case "tail":
		n := 10
		file := ""
		for i, a := range args {
			if a == "-n" && i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &n)
			} else if !strings.HasPrefix(a, "-") {
				file = a
			}
		}
		if file == "" {
			fmt.Fprintln(os.Stderr, "usage: siba tail [-n N] <file.md[#symbol]>")
			os.Exit(2)
		}
		runTail(file, n, rawMode)

	// Search commands (no render, index only)
	case "ls":
		if len(args) == 0 {
			runLs("", jsonMode)
		} else {
			runLs(args[0], jsonMode)
		}
	case "tree":
		depsMode := hasFlag("--deps")
		dotMode := hasFlag("--dot")
		treeArgs := argsWithout(2, "--json", "--deps", "--dot")
		if depsMode {
			runTreeDeps(jsonMode, dotMode)
		} else if len(treeArgs) > 0 {
			runTreeHeadings(treeArgs[0], jsonMode)
		} else {
			runTreeHeadings("", jsonMode)
		}
	case "find":
		findArgs := argsWithout(2, "--json", "--heading", "--variable")
		headingMode := hasFlag("--heading")
		variableMode := hasFlag("--variable")
		if len(findArgs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: siba find [--heading|--variable] <query>")
			os.Exit(2)
		}
		runFind(findArgs[0], headingMode, variableMode, jsonMode)

	// Build commands
	case "export":
		runExport(jsonMode)
	case "check":
		checkArgs := argsWithout(2, "--json")
		if len(checkArgs) == 0 {
			runCheckWorkspace(jsonMode)
		} else {
			runCheck(checkArgs[0], jsonMode)
		}

	// Management
	case "init":
		runInit()
	case "get":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: siba get <package-url> [version]")
			os.Exit(2)
		}
		version := "main"
		if len(args) >= 2 {
			version = args[1]
		}
		runGet(args[0], version)
	case "tidy":
		runTidy()
	case "run":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: siba run <script-name>")
			os.Exit(2)
		}
		runScript(args[0])
	case "help":
		if len(args) > 0 {
			runHelp(args[0])
		} else {
			runHelp("")
		}
	case "version":
		fmt.Println("siba v0.2.0")
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("SIBA — Structured Ink for Building Archives")
	fmt.Println()
	fmt.Println("Read:")
	fmt.Println("  siba cat <file[#symbol]>        Render and stream to stdout")
	fmt.Println("  siba head [-n N] <file[#sym]>   First N lines (default 10)")
	fmt.Println("  siba tail [-n N] <file[#sym]>   Last N lines (default 10)")
	fmt.Println()
	fmt.Println("Search:")
	fmt.Println("  siba ls [file]                  List documents or symbols")
	fmt.Println("  siba tree [file]                Heading tree")
	fmt.Println("  siba tree --deps                Dependency tree")
	fmt.Println("  siba find <query>               Search workspace")
	fmt.Println()
	fmt.Println("Build:")
	fmt.Println("  siba export [-o dir]            Export to _export/{version}/")
	fmt.Println("  siba check [file]               Validate documents")
	fmt.Println()
	fmt.Println("Manage:")
	fmt.Println("  siba init                       Initialize project")
	fmt.Println("  siba get <pkg> [version]        Add dependency")
	fmt.Println("  siba tidy                       Remove unused dependencies")
	fmt.Println("  siba run <script>               Run script from module.toml")
	fmt.Println("  siba help [topic]               Show help")
	fmt.Println("  siba version                    Show version")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --json    JSON output")
	fmt.Println("  --raw     Raw source (no render)")
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

// JSONExportResult is the result of siba render --json
type JSONExportResult struct {
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

// JSONEnvelope is the common JSON output wrapper
type JSONEnvelope struct {
	OK     bool        `json:"ok"`
	Data   interface{} `json:"data"`
	Errors interface{} `json:"errors"`
}

func writeJSON(v interface{}) {
	env := JSONEnvelope{OK: true, Data: v, Errors: []interface{}{}}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(env)
}

func writeJSONError(v interface{}, errors interface{}) {
	env := JSONEnvelope{OK: false, Data: v, Errors: errors}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(env)
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

func runExport(jsonMode bool) {
	// find -o flag value
	outputDir := ""
	for i, arg := range os.Args {
		if arg == "-o" && i+1 < len(os.Args) {
			outputDir = os.Args[i+1]
		}
	}

	// Workspace render
	cwd, _ := os.Getwd()
	w, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		if jsonMode {
			writeJSON(JSONExportResult{Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "error loading workspace: %v\n", err)
		}
		os.Exit(1)
	}

	if err := scripts.RunPreexport(w.Config); err != nil {
		fmt.Fprintf(os.Stderr, "preexport failed: %v\n", err)
		os.Exit(1)
	}

	version := w.GetVersion()
	if !jsonMode {
		fmt.Printf("exporting v%s...\n", version)
	}

	if err := render.RenderWorkspace(w, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}

	if err := scripts.RunPostexport(w.Config); err != nil {
		fmt.Fprintf(os.Stderr, "postexport failed: %v\n", err)
		os.Exit(1)
	}

	if !jsonMode {
		fmt.Printf("export complete: v%s\n", version)
	}
}

func renderSingleFile(path string, jsonMode bool) {
	source, err := os.ReadFile(path)
	if err != nil {
		if jsonMode {
			writeJSON(JSONExportResult{File: path, Error: err.Error()})
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
		writeJSON(JSONExportResult{File: path, Error: "document has errors"})
		os.Exit(1)
	}
	if hasErrors {
		os.Exit(1)
	}

	// Load workspace for cross-doc refs and @default inheritance
	cwd, _ := os.Getwd()
	ws, _ := workspace.LoadWorkspace(cwd)

	var buf bytes.Buffer
	if err := render.StreamRender(doc, &buf, ws); err != nil {
		if jsonMode {
			writeJSON(JSONExportResult{File: path, Error: err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
		}
		os.Exit(1)
	}
	output := buf.String()

	if jsonMode {
		writeJSON(JSONExportResult{File: path, Content: output})
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

// --- Read commands ---

// parseFileArg splits "file.md#symbol" into path and symbol parts
func parseFileArg(fileArg string) (string, string) {
	if idx := strings.Index(fileArg, "#"); idx >= 0 {
		return fileArg[:idx], fileArg[idx+1:]
	}
	return fileArg, ""
}

func loadAndParse(fileArg string) (*ast.Document, string) {
	path, symbol := parseFileArg(fileArg)
	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	doc := parser.ParseDocument(path, string(source))
	for _, d := range doc.Diagnostics {
		if d.Severity == ast.SeverityError {
			fmt.Fprintf(os.Stderr, "%s:%d: error: %s\n", path, d.Range.Start.Line, d.Message)
		}
	}
	return doc, symbol
}

func renderOrRaw(doc *ast.Document, symbol string, rawMode bool) string {
	var output string
	if rawMode {
		if symbol != "" {
			output = extractSection(doc, symbol)
		} else {
			output = doc.Source
		}
	} else {
		cwd, _ := os.Getwd()
		ws, _ := workspace.LoadWorkspace(cwd)
		var buf bytes.Buffer
		if err := render.StreamRender(doc, &buf, ws); err != nil {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
			os.Exit(1)
		}
		rendered := buf.String()
		if symbol != "" {
			// re-parse rendered output to extract section
			tmpDoc := parser.ParseDocument(doc.Path, rendered)
			output = extractSection(tmpDoc, symbol)
		} else {
			output = rendered
		}
	}
	return output
}

func extractSection(doc *ast.Document, symbol string) string {
	lines := strings.Split(doc.Source, "\n")
	h := findHeading(doc.Headings, symbol)
	if h == nil {
		fmt.Fprintf(os.Stderr, "error: symbol %q not found\n", symbol)
		os.Exit(1)
	}
	start := h.Position.Line - 1
	end := h.Content.End.Line
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n") + "\n"
}

func findHeading(headings []*ast.Heading, symbol string) *ast.Heading {
	for _, h := range headings {
		if h.Slug == symbol || h.Name == symbol || h.Text == symbol {
			return h
		}
		if found := findHeading(h.Children, symbol); found != nil {
			return found
		}
	}
	return nil
}

func runCat(fileArg string, rawMode bool) {
	if rawMode {
		doc, symbol := loadAndParse(fileArg)
		output := renderOrRaw(doc, symbol, true)
		fmt.Print(output)
		return
	}

	doc, symbol := loadAndParse(fileArg)
	if symbol != "" {
		// symbol routing — render then extract section
		output := renderOrRaw(doc, symbol, false)
		fmt.Print(output)
		return
	}

	// streaming render to stdout
	cwd, _ := os.Getwd()
	ws, _ := workspace.LoadWorkspace(cwd)
	if err := render.StreamRender(doc, os.Stdout, ws); err != nil {
		fmt.Fprintf(os.Stderr, "render error: %v\n", err)
		os.Exit(1)
	}
}

func runHead(fileArg string, n int, rawMode bool) {
	doc, symbol := loadAndParse(fileArg)
	if !rawMode && symbol == "" {
		// streaming with line limit
		cwd, _ := os.Getwd()
		ws, _ := workspace.LoadWorkspace(cwd)
		lw := &lineWriter{w: os.Stdout, limit: n}
		render.StreamRender(doc, lw, ws)
		return
	}
	output := renderOrRaw(doc, symbol, rawMode)
	lines := strings.Split(output, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	fmt.Println(strings.Join(lines, "\n"))
}

func runHeadBytes(fileArg string, c int, rawMode bool) {
	doc, symbol := loadAndParse(fileArg)
	if !rawMode && symbol == "" {
		cwd, _ := os.Getwd()
		ws, _ := workspace.LoadWorkspace(cwd)
		bw := &byteWriter{w: os.Stdout, limit: c}
		render.StreamRender(doc, bw, ws)
		return
	}
	output := renderOrRaw(doc, symbol, rawMode)
	if len(output) > c {
		output = output[:c]
	}
	fmt.Print(output)
}

// byteWriter wraps an io.Writer and stops after N bytes
type byteWriter struct {
	w     io.Writer
	limit int
	count int
}

func (bw *byteWriter) Write(p []byte) (int, error) {
	remaining := bw.limit - bw.count
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := bw.w.Write(p)
	bw.count += n
	return n, err
}

func runTail(fileArg string, n int, rawMode bool) {
	doc, symbol := loadAndParse(fileArg)
	if !rawMode && symbol == "" {
		// streaming with ring buffer
		cwd, _ := os.Getwd()
		ws, _ := workspace.LoadWorkspace(cwd)
		tw := &tailWriter{limit: n}
		render.StreamRender(doc, tw, ws)
		for _, line := range tw.Lines() {
			fmt.Println(line)
		}
		return
	}
	output := renderOrRaw(doc, symbol, rawMode)
	lines := strings.Split(output, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	fmt.Println(strings.Join(lines, "\n"))
}

// tailWriter collects lines in a ring buffer, keeping only the last N
type tailWriter struct {
	limit int
	buf   []string
	sb    strings.Builder
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			tw.buf = append(tw.buf, tw.sb.String())
			tw.sb.Reset()
			if len(tw.buf) > tw.limit {
				tw.buf = tw.buf[1:]
			}
		} else {
			tw.sb.WriteByte(b)
		}
	}
	return len(p), nil
}

func (tw *tailWriter) Lines() []string {
	if tw.sb.Len() > 0 {
		tw.buf = append(tw.buf, tw.sb.String())
		tw.sb.Reset()
		if len(tw.buf) > tw.limit {
			tw.buf = tw.buf[1:]
		}
	}
	return tw.buf
}

// lineWriter wraps an io.Writer and stops after N lines
type lineWriter struct {
	w     io.Writer
	limit int
	count int
}

func (lw *lineWriter) Write(p []byte) (int, error) {
	if lw.count >= lw.limit {
		return len(p), nil // discard
	}
	for _, b := range p {
		if b == '\n' {
			lw.count++
			if lw.count >= lw.limit {
				lw.w.Write([]byte{'\n'})
				return len(p), nil
			}
		}
	}
	return lw.w.Write(p)
}

// --- Search commands ---

// JSONDocInfo represents a document in ls output
type JSONDocInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsTemplate bool   `json:"is_template"`
}

// JSONSymbolInfo represents a symbol in ls output
type JSONSymbolInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "heading", "variable", "reference"
	Line int    `json:"line"`
}

func runLs(fileArg string, jsonMode bool) {
	if fileArg == "" {
		// list all documents in workspace
		cwd, _ := os.Getwd()
		ws, err := workspace.LoadWorkspace(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if jsonMode {
			var docs []JSONDocInfo
			for name, doc := range ws.Documents {
				docs = append(docs, JSONDocInfo{Name: name, Path: doc.Path, IsTemplate: false})
			}
			for name, doc := range ws.Templates {
				docs = append(docs, JSONDocInfo{Name: name, Path: doc.Path, IsTemplate: true})
			}
			writeJSON(docs)
			return
		}

		for name, doc := range ws.Templates {
			fmt.Printf("%-6s %-30s %s\n", "tmpl", name, doc.Path)
		}
		for name, doc := range ws.Documents {
			fmt.Printf("%-6s %-30s %s\n", "doc", name, doc.Path)
		}
		return
	}

	// list symbols in a specific file
	doc, _ := loadAndParse(fileArg)

	if jsonMode {
		var symbols []JSONSymbolInfo
		collectHeadingSymbols(doc.Headings, &symbols)
		for _, v := range doc.Variables {
			symbols = append(symbols, JSONSymbolInfo{
				Name: v.Name,
				Kind: "variable",
				Line: v.Position.Line,
			})
		}
		for _, r := range doc.References {
			if !r.IsEscaped {
				symbols = append(symbols, JSONSymbolInfo{
					Name: r.Raw,
					Kind: "reference",
					Line: r.Position.Line,
				})
			}
		}
		writeJSON(symbols)
		return
	}

	fmt.Printf("# %s\n", fileArg)
	if len(doc.Headings) > 0 {
		fmt.Println("\nHeadings:")
		printHeadingList(doc.Headings, "  ")
	}
	if len(doc.Variables) > 0 {
		fmt.Println("\nVariables:")
		for _, v := range doc.Variables {
			mut := "const"
			if v.Mutability == ast.MutLet {
				mut = "let"
			}
			fmt.Printf("  %s %s\n", mut, v.Name)
		}
	}
	if len(doc.References) > 0 {
		fmt.Println("\nReferences:")
		for _, r := range doc.References {
			if !r.IsEscaped {
				fmt.Printf("  {{%s}}\n", r.Raw)
			}
		}
	}
}

func collectHeadingSymbols(headings []*ast.Heading, symbols *[]JSONSymbolInfo) {
	for _, h := range headings {
		*symbols = append(*symbols, JSONSymbolInfo{
			Name: h.Text,
			Kind: "heading",
			Line: h.Position.Line,
		})
		collectHeadingSymbols(h.Children, symbols)
	}
}

func printHeadingList(headings []*ast.Heading, indent string) {
	for _, h := range headings {
		fmt.Printf("%s%s %s\n", indent, strings.Repeat("#", h.Level), h.Text)
		printHeadingList(h.Children, indent+"  ")
	}
}

// JSONHeadingTree represents a heading node in tree output
type JSONHeadingTree struct {
	Text     string            `json:"text"`
	Slug     string            `json:"slug"`
	Level    int               `json:"level"`
	Children []JSONHeadingTree `json:"children,omitempty"`
}

func runTreeHeadings(fileArg string, jsonMode bool) {
	if fileArg != "" {
		doc, _ := loadAndParse(fileArg)

		if jsonMode {
			tree := buildHeadingTree(doc.Headings)
			writeJSON(tree)
			return
		}

		fmt.Printf("# %s\n", fileArg)
		printHeadingTree(doc.Headings, "")
		return
	}

	// workspace-wide heading overview
	cwd, _ := os.Getwd()
	ws, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if jsonMode {
		result := make(map[string][]JSONHeadingTree)
		for path, doc := range ws.DocsByPath {
			result[path] = buildHeadingTree(doc.Headings)
		}
		writeJSON(result)
		return
	}

	for path, doc := range ws.DocsByPath {
		name := doc.Name
		if name == "" {
			name = path
		}
		fmt.Printf("== %s ==\n", name)
		printHeadingTree(doc.Headings, "  ")
		fmt.Println()
	}
}

func buildHeadingTree(headings []*ast.Heading) []JSONHeadingTree {
	var tree []JSONHeadingTree
	for _, h := range headings {
		node := JSONHeadingTree{
			Text:     h.Text,
			Slug:     h.Slug,
			Level:    h.Level,
			Children: buildHeadingTree(h.Children),
		}
		tree = append(tree, node)
	}
	return tree
}

func printHeadingTree(headings []*ast.Heading, indent string) {
	for _, h := range headings {
		fmt.Printf("%s%s %s\n", indent, strings.Repeat("#", h.Level), h.Text)
		printHeadingTree(h.Children, indent+"  ")
	}
}

func runTreeDeps(jsonMode bool, dotMode bool) {
	cwd, _ := os.Getwd()
	ws, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Build nodes and edges (reuse runGraph logic)
	var nodes []JSONGraphNode
	docIDs := make(map[string]string)

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

	var edges []JSONGraphEdge
	for path, doc := range ws.DocsByPath {
		sourceID := docIDs[path]

		if doc.ExtendsName != "" {
			edges = append(edges, JSONGraphEdge{
				Source: sourceID,
				Target: doc.ExtendsName,
				Type:   "extends",
			})
		}

		for _, ref := range doc.References {
			if ref.IsEscaped {
				continue
			}
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

	result := JSONGraphResult{Nodes: nodes, Edges: edges}

	if jsonMode {
		writeJSON(result)
		return
	}

	if dotMode {
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
		return
	}

	// text tree output (default)
	// Build adjacency: source → targets
	children := make(map[string][]string)
	hasParent := make(map[string]bool)
	for _, e := range edges {
		children[e.Source] = append(children[e.Source], e.Target+" ("+e.Type+")")
		hasParent[e.Target] = true
	}

	// Print roots first
	for _, n := range nodes {
		if !hasParent[n.ID] {
			fmt.Println(n.ID)
			printDepTree(children, n.ID, "  ", make(map[string]bool))
		}
	}
}

func printDepTree(children map[string][]string, node string, indent string, visited map[string]bool) {
	if visited[node] {
		return
	}
	visited[node] = true
	for _, child := range children[node] {
		fmt.Printf("%s→ %s\n", indent, child)
		// extract actual node name (strip type suffix)
		parts := strings.SplitN(child, " (", 2)
		if len(parts) > 0 {
			printDepTree(children, parts[0], indent+"  ", visited)
		}
	}
}

// JSONFindResult represents a search match
type JSONFindResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Kind    string `json:"kind,omitempty"` // "heading", "variable", "content"
}

func runFind(query string, headingOnly bool, variableOnly bool, jsonMode bool) {
	cwd, _ := os.Getwd()
	ws, err := workspace.LoadWorkspace(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var results []JSONFindResult
	queryLower := strings.ToLower(query)

	for path, doc := range ws.DocsByPath {
		if headingOnly {
			searchHeadings(doc.Headings, path, queryLower, &results)
			continue
		}
		if variableOnly {
			for _, v := range doc.Variables {
				if strings.Contains(strings.ToLower(v.Name), queryLower) {
					results = append(results, JSONFindResult{
						File: path,
						Line: v.Position.Line,
						Text: v.Name,
						Kind: "variable",
					})
				}
			}
			continue
		}
		// grep rendered text (streaming)
		gw := &grepWriter{query: queryLower, file: path}
		render.StreamRender(doc, gw, ws)
		for _, r := range gw.results {
			results = append(results, r)
		}
	}

	if jsonMode {
		if results == nil {
			results = []JSONFindResult{}
		}
		writeJSON(results)
		return
	}

	if len(results) == 0 {
		fmt.Println("no matches found")
		return
	}
	for _, r := range results {
		fmt.Printf("%s:%d: %s\n", r.File, r.Line, r.Text)
	}
}

// grepWriter collects lines matching a query during streaming render
type grepWriter struct {
	query   string
	file    string
	line    int
	sb      strings.Builder
	results []JSONFindResult
}

func (gw *grepWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			gw.line++
			text := gw.sb.String()
			if strings.Contains(strings.ToLower(text), gw.query) {
				gw.results = append(gw.results, JSONFindResult{
					File: gw.file,
					Line: gw.line,
					Text: strings.TrimSpace(text),
					Kind: "content",
				})
			}
			gw.sb.Reset()
		} else {
			gw.sb.WriteByte(b)
		}
	}
	return len(p), nil
}

func searchHeadings(headings []*ast.Heading, path string, query string, results *[]JSONFindResult) {
	for _, h := range headings {
		if strings.Contains(strings.ToLower(h.Text), query) {
			*results = append(*results, JSONFindResult{
				File: path,
				Line: h.Position.Line,
				Text: h.Text,
				Kind: "heading",
			})
		}
		searchHeadings(h.Children, path, query, results)
	}
}

func runHelp(topic string) {
	switch topic {
	case "directives":
		fmt.Println("SIBA Directives")
		fmt.Println("================")
		fmt.Println()
		fmt.Println("  @doc <name>           Declare document name")
		fmt.Println("  @template <name>      Declare as template")
		fmt.Println("  @extends <name>       Inherit from template")
		fmt.Println("  @name <name>          Name a heading section")
		fmt.Println("  @default              Mark heading as default content")
		fmt.Println("  @const <name> = val   Declare constant variable")
		fmt.Println("  @let <name> = val     Declare mutable variable")
		fmt.Println("  @import <alias> from <path>  Import document")
		fmt.Println()
		fmt.Println("All directives use HTML comment syntax: <!-- @directive ... -->")
	case "variables":
		fmt.Println("SIBA Variables")
		fmt.Println("===============")
		fmt.Println()
		fmt.Println("  @const name = \"value\"     Immutable string")
		fmt.Println("  @const count = 42         Immutable number")
		fmt.Println("  @const flag = true        Immutable boolean")
		fmt.Println("  @let name = \"default\"     Mutable (overridable by child)")
		fmt.Println()
		fmt.Println("Types: string, number, boolean, null, array, object, union")
		fmt.Println("Access: default (visible to children), private (hidden)")
	case "templates":
		fmt.Println("SIBA Templates")
		fmt.Println("===============")
		fmt.Println()
		fmt.Println("  <!-- @template my-template -->   Declare template")
		fmt.Println("  <!-- @extends my-template -->    Use template")
		fmt.Println()
		fmt.Println("Templates define structure with @default sections.")
		fmt.Println("Children override @default content with their own headings.")
		fmt.Println("Variables from templates are inherited by children.")
	case "references":
		fmt.Println("SIBA References")
		fmt.Println("================")
		fmt.Println()
		fmt.Println("  {{doc-name}}              Insert document content")
		fmt.Println("  {{doc-name#section}}       Insert specific section")
		fmt.Println("  {{doc-name.variable}}      Insert variable value")
		fmt.Println("  {{.variable}}              Insert local variable")
		fmt.Println("  \\{{escaped}}               Literal (not expanded)")
	case "control":
		fmt.Println("SIBA Control Flow")
		fmt.Println("==================")
		fmt.Println()
		fmt.Println("  <!-- @if condition -->")
		fmt.Println("    content")
		fmt.Println("  <!-- @endif -->")
		fmt.Println()
		fmt.Println("  <!-- @for item in collection -->")
		fmt.Println("    content with {{.item}}")
		fmt.Println("  <!-- @endfor -->")
	case "packages":
		fmt.Println("SIBA Packages")
		fmt.Println("==============")
		fmt.Println()
		fmt.Println("  siba get <url> [version]    Add dependency")
		fmt.Println("  siba tidy                   Remove unused")
		fmt.Println()
		fmt.Println("Packages are declared in module.toml [dependencies].")
		fmt.Println("Use @import to bring package documents into scope.")
	default:
		fmt.Println("SIBA Help Topics")
		fmt.Println("=================")
		fmt.Println()
		fmt.Println("  siba help directives    @doc, @extends, @template, etc.")
		fmt.Println("  siba help variables     @const, @let, types")
		fmt.Println("  siba help templates     @template, @extends, @default")
		fmt.Println("  siba help references    {{doc}}, {{doc.var}}, {{doc#sec}}")
		fmt.Println("  siba help control       @if/@endif, @for/@endfor")
		fmt.Println("  siba help packages      siba get, siba tidy, module.toml")
		fmt.Println()
		fmt.Println("Run 'siba help <topic>' for details.")
	}
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
