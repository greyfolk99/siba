package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadWorkspace_EmptyDir verifies that LoadWorkspace succeeds on an empty
// directory (no module.toml, no .md files) and returns an empty workspace.
func TestLoadWorkspace_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ws, err := LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Root != dir {
		t.Errorf("expected root %q, got %q", dir, ws.Root)
	}
	if ws.Config != nil {
		t.Error("expected nil Config for empty dir")
	}
	if len(ws.Documents) != 0 {
		t.Errorf("expected 0 documents, got %d", len(ws.Documents))
	}
	if len(ws.Templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(ws.Templates))
	}
	if len(ws.DocsByPath) != 0 {
		t.Errorf("expected 0 DocsByPath, got %d", len(ws.DocsByPath))
	}
}

// TestLoadWorkspace_WithModuleToml verifies that LoadWorkspace correctly parses
// module.toml and populates Config fields.
func TestLoadWorkspace_WithModuleToml(t *testing.T) {
	dir := t.TempDir()

	tomlContent := `[module]
name = "test-project"
version = "1.2.3"
`
	if err := os.WriteFile(filepath.Join(dir, "module.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Config == nil {
		t.Fatal("expected non-nil Config")
	}
	if ws.Config.Module.Name != "test-project" {
		t.Errorf("expected module name 'test-project', got %q", ws.Config.Module.Name)
	}
	if ws.Config.Module.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", ws.Config.Module.Version)
	}
	if ws.GetVersion() != "1.2.3" {
		t.Errorf("GetVersion() = %q, want '1.2.3'", ws.GetVersion())
	}
}

// TestRefreshDocument verifies that RefreshDocument re-parses a file and updates
// workspace maps, removing stale entries and inserting new ones.
func TestRefreshDocument(t *testing.T) {
	dir := t.TempDir()
	ws, err := LoadWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}

	path := "test.md"

	// First version: doc named "alpha"
	ws.RefreshDocument(path, "<!-- @doc alpha -->\n# Alpha\nHello")
	if _, ok := ws.Documents["alpha"]; !ok {
		t.Fatal("expected document 'alpha' to exist")
	}
	if ws.DocsByPath[path] == nil {
		t.Fatal("expected DocsByPath entry")
	}

	// Update: rename to "beta"
	ws.RefreshDocument(path, "<!-- @doc beta -->\n# Beta\nWorld")
	if _, ok := ws.Documents["alpha"]; ok {
		t.Error("stale document 'alpha' should have been removed")
	}
	if _, ok := ws.Documents["beta"]; !ok {
		t.Fatal("expected document 'beta' to exist")
	}

	// Update: make it a template
	ws.RefreshDocument(path, "<!-- @template gamma -->\n# Gamma\nTemplate")
	if _, ok := ws.Documents["beta"]; ok {
		t.Error("stale document 'beta' should have been removed")
	}
	if _, ok := ws.Templates["gamma"]; !ok {
		t.Fatal("expected template 'gamma' to exist")
	}
}

// TestResolveImportDoc verifies that ResolveImportDoc resolves documents by
// relative path (with and without .md extension) and by @doc name.
func TestResolveImportDoc(t *testing.T) {
	dir := t.TempDir()

	// Create a .md file with a @doc name
	mdContent := "<!-- @doc my-doc -->\n# My Doc\nContent here"
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "page.md"), []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := LoadWorkspace(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resolve by @doc name (no fromPath)
	doc := ws.ResolveImportDoc("my-doc", "")
	if doc == nil {
		t.Fatal("expected to resolve 'my-doc' by name")
	}

	// Resolve by workspace-relative path with .md
	doc = ws.ResolveImportDoc("sub/page.md", "")
	if doc == nil {
		t.Fatal("expected to resolve 'sub/page.md' by path")
	}

	// Resolve by workspace-relative path without .md
	doc = ws.ResolveImportDoc("sub/page", "")
	if doc == nil {
		t.Fatal("expected to resolve 'sub/page' by path without extension")
	}

	// Non-existent should return nil
	doc = ws.ResolveImportDoc("nonexistent", "")
	if doc != nil {
		t.Error("expected nil for non-existent import")
	}

	// Resolve relative to fromPath: sibling file (fromPath is workspace-relative)
	siblingMd := "<!-- @doc sibling -->\n# Sibling"
	if err := os.WriteFile(filepath.Join(dir, "sub", "sibling.md"), []byte(siblingMd), 0644); err != nil {
		t.Fatal(err)
	}
	ws, _ = LoadWorkspace(dir)
	doc = ws.ResolveImportDoc("./sibling.md", filepath.Join("sub", "page.md"))
	if doc == nil {
		t.Fatal("expected to resolve './sibling.md' relative to fromPath")
	}
	if doc.Name != "sibling" {
		t.Errorf("expected sibling doc, got %s", doc.Name)
	}
}
