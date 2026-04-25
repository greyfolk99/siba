package pkg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/greyfolk99/siba/internal/workspace"
)

// CacheDir returns the global siba cache directory
func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".siba", "cache")
}

// Manager handles package operations
type Manager struct {
	Root   string
	Config *workspace.ModuleConfig
}

// NewManager creates a package manager for a workspace
func NewManager(root string, config *workspace.ModuleConfig) *Manager {
	return &Manager{Root: root, Config: config}
}

// Add adds a dependency to module.toml and fetches it
func (m *Manager) Add(pkgURL string, version string) error {
	if m.Config == nil {
		return fmt.Errorf("no module.toml found, run 'siba init' first")
	}

	if m.Config.Dependencies == nil {
		m.Config.Dependencies = make(map[string]string)
	}
	m.Config.Dependencies[pkgURL] = version

	// Write updated config
	if err := workspace.WriteModuleToml(filepath.Join(m.Root, "module.toml"), m.Config); err != nil {
		return fmt.Errorf("failed to update module.toml: %w", err)
	}

	// Fetch the package
	return Fetch(pkgURL, version, CacheDir())
}

// Install fetches all dependencies
func (m *Manager) Install() error {
	if m.Config == nil {
		return nil
	}

	for pkgURL, version := range m.Config.Dependencies {
		dest := CachePath(pkgURL, version)
		if _, err := os.Stat(dest); err == nil {
			continue // already cached
		}
		fmt.Printf("fetching %s@%s...\n", pkgURL, version)
		if err := Fetch(pkgURL, version, CacheDir()); err != nil {
			return fmt.Errorf("failed to fetch %s: %w", pkgURL, err)
		}
	}
	return nil
}

// Tidy removes dependencies not referenced in any document
func (m *Manager) Tidy(usedPkgs map[string]bool) error {
	if m.Config == nil {
		return nil
	}

	changed := false
	for pkgURL := range m.Config.Dependencies {
		if !usedPkgs[pkgURL] {
			delete(m.Config.Dependencies, pkgURL)
			changed = true
			fmt.Printf("removed unused dependency: %s\n", pkgURL)
		}
	}

	if changed {
		return workspace.WriteModuleToml(filepath.Join(m.Root, "module.toml"), m.Config)
	}
	return nil
}

// CachePath returns the cache directory for a specific package version
func CachePath(pkgURL string, version string) string {
	safeName := strings.ReplaceAll(pkgURL, "/", "_")
	return filepath.Join(CacheDir(), fmt.Sprintf("%s@%s", safeName, version))
}

// Fetch downloads a package via git clone
func Fetch(pkgURL string, version string, cacheDir string) error {
	dest := CachePath(pkgURL, version)

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	// git clone
	gitURL := "https://" + pkgURL + ".git"
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", version, gitURL, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// try without .git suffix
		gitURL = "https://" + pkgURL
		cmd = exec.Command("git", "clone", "--depth", "1", "--branch", version, gitURL, dest)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
	}

	return nil
}
