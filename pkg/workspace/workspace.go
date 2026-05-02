package workspace

import (
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/greyfolk99/siba/pkg/ast"
	"github.com/greyfolk99/siba/pkg/parser"
)

// ModuleConfig represents module.toml
type ModuleConfig struct {
	Module       ModuleInfo           `toml:"module"`
	Dependencies map[string]string    `toml:"dependencies"`
	Scripts      map[string]string    `toml:"scripts"`
	Render       RenderConfig         `toml:"render"`
}

type ModuleInfo struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

type RenderConfig struct {
	Formats []string `toml:"formats"`
}

// Workspace represents a siba workspace
type Workspace struct {
	Root       string
	Config     *ModuleConfig
	Documents  map[string]*ast.Document // keyed by @doc name
	DocsByPath map[string]*ast.Document // keyed by file path
	Templates  map[string]*ast.Document // keyed by @template name
}

// LoadWorkspace loads a workspace from a root directory
func LoadWorkspace(root string) (*Workspace, error) {
	w := &Workspace{
		Root:       root,
		Documents:  make(map[string]*ast.Document),
		DocsByPath: make(map[string]*ast.Document),
		Templates:  make(map[string]*ast.Document),
	}

	// Parse module.toml if exists
	configPath := filepath.Join(root, "module.toml")
	if _, err := os.Stat(configPath); err == nil {
		config, err := ParseModuleToml(configPath)
		if err != nil {
			return nil, err
		}
		w.Config = config
	}

	// Discover and parse all .md files (supports multiple docs per file)
	paths := DiscoverDocuments(root)
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(root, path)
		docs := parser.ParseDocuments(relPath, string(source))
		for i, doc := range docs {
			if i == 0 {
				w.DocsByPath[relPath] = doc // first doc for path lookup
			}
			if doc.Name != "" {
				if doc.IsTemplate {
					w.Templates[doc.Name] = doc
				} else {
					w.Documents[doc.Name] = doc
				}
			}
		}
	}

	return w, nil
}

// DiscoverDocuments finds all .md files recursively, skipping _export and .siba dirs
func DiscoverDocuments(root string) []string {
	var paths []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// skip hidden dirs, _export, node_modules (but never skip the root itself)
		name := info.Name()
		if info.IsDir() && path != root && (strings.HasPrefix(name, ".") || name == "_export" || name == "node_modules") {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(name, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	return paths
}

// GetDocument returns a document by @doc name
func (w *Workspace) GetDocument(name string) *ast.Document {
	return w.Documents[name]
}

// GetDocumentByPath returns a document by file path
func (w *Workspace) GetDocumentByPath(path string) *ast.Document {
	return w.DocsByPath[path]
}

// ResolveImportDoc resolves an import path relative to fromPath's directory.
// Falls back to workspace-relative or @doc name if not found relative to fromPath.
// importPath may be a relative path (./foo.md), with/without .md, or a @doc name.
// fromPath is the absolute path of the file performing the import; pass "" if unknown.
func (w *Workspace) ResolveImportDoc(importPath, fromPath string) *ast.Document {
	tryPaths := []string{}

	// Resolve relative to fromPath's directory if available
	if fromPath != "" && (strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") || !filepath.IsAbs(importPath)) {
		baseDir := filepath.Dir(fromPath)
		joined := filepath.Clean(filepath.Join(baseDir, importPath))
		tryPaths = append(tryPaths, joined, joined+".md")
	}

	// Fallback: workspace-relative (legacy behavior)
	clean := strings.TrimPrefix(importPath, "./")
	tryPaths = append(tryPaths, clean, clean+".md")

	for _, p := range tryPaths {
		if doc := w.GetDocumentByPath(p); doc != nil {
			return doc
		}
	}

	// Final fallback: @doc name
	if doc := w.GetDocument(importPath); doc != nil {
		return doc
	}
	return nil
}

// GetTemplate returns a template document by its @template name
func (w *Workspace) GetTemplate(name string) *ast.Document {
	if doc, ok := w.Templates[name]; ok {
		return doc
	}
	return nil
}

// GetVersion returns the module version or "0.0.0" if not set
func (w *Workspace) GetVersion() string {
	if w.Config != nil && w.Config.Module.Version != "" {
		return w.Config.Module.Version
	}
	return "0.0.0"
}

// RefreshDocument re-parses a single document, cleaning up stale entries
func (w *Workspace) RefreshDocument(path string, source string) {
	// Remove old entries for this path
	if old, ok := w.DocsByPath[path]; ok && old.Name != "" {
		delete(w.Documents, old.Name)
		delete(w.Templates, old.Name)
	}

	doc := parser.ParseDocument(path, source)
	w.DocsByPath[path] = doc
	if doc.Name != "" {
		if doc.IsTemplate {
			w.Templates[doc.Name] = doc
		} else {
			w.Documents[doc.Name] = doc
		}
	}
}

// ParseModuleToml parses a module.toml file
func ParseModuleToml(path string) (*ModuleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ModuleConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// WriteModuleToml writes a module.toml file
func WriteModuleToml(path string, config *ModuleConfig) error {
	data, err := toml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// InitModuleToml creates a new module.toml with defaults
func InitModuleToml(root string, name string) error {
	config := &ModuleConfig{
		Module: ModuleInfo{
			Name:    name,
			Version: "0.1.0",
		},
		Dependencies: make(map[string]string),
		Scripts:      make(map[string]string),
	}
	return WriteModuleToml(filepath.Join(root, "module.toml"), config)
}
