package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestLoadProjectServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectConfigFileName)
	content := `{
		"languageServers": {
			"gopls": {
				"command": "gopls",
				"args": ["serve"],
				"filetypes": ["go"],
				"rootMarkers": ["go.mod"]
			},
			"pyright": {
				"command": "pyright-langserver",
				"args": ["--stdio"],
				"filetypes": ["py"],
				"rootMarkers": ["pyproject.toml", "requirements.txt"]
			}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	servers, err := LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %+v", servers)
	}
	gopls := servers["gopls"]
	if gopls.Command != "gopls" || len(gopls.Args) != 1 || len(gopls.FileTypes) != 1 || gopls.FileTypes[0] != "go" {
		t.Fatalf("gopls = %+v", gopls)
	}
	pyright := servers["pyright"]
	if len(pyright.RootMarkers) != 2 || pyright.RootMarkers[1] != "requirements.txt" {
		t.Fatalf("pyright = %+v", pyright)
	}
}

func TestLoadProjectServersMissingFile(t *testing.T) {
	servers, err := LoadProjectServers(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %+v", servers)
	}
}

func TestLoadProjectServersInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectConfigFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectServers(dir); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMergeServersProjectOverridesGlobal(t *testing.T) {
	global := map[string]config.LanguageServerConfig{
		"shared":      {Command: "global-cmd", FileTypes: []string{"go"}},
		"only-global": {Command: "g"},
	}
	project := map[string]config.LanguageServerConfig{
		"shared":       {Command: "project-cmd", FileTypes: []string{"go"}},
		"only-project": {Command: "p", FileTypes: []string{"py"}},
	}
	merged := MergeServers(global, project)
	if len(merged) != 3 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged["shared"].Command != "project-cmd" {
		t.Fatalf("shared = %+v", merged["shared"])
	}
	if merged["only-global"].Command != "g" || merged["only-project"].Command != "p" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestMergeServersDisables(t *testing.T) {
	base := map[string]config.LanguageServerConfig{
		"gopls": {Command: "gopls", FileTypes: []string{"go"}},
	}
	over := map[string]config.LanguageServerConfig{
		"gopls": {Command: "gopls", FileTypes: []string{"go"}, Enabled: &[]bool{false}[0]},
	}
	merged := MergeServers(base, over)
	if len(merged) != 0 {
		t.Fatalf("disabled server must be dropped, got %+v", merged)
	}
}

func TestSaveAndUpsertProject(t *testing.T) {
	dir := t.TempDir()
	server := config.LanguageServerConfig{
		Command:     "gopls",
		Args:        []string{"serve"},
		FileTypes:   []string{"go"},
		RootMarkers: []string{"go.mod"},
	}
	if err := UpsertProjectServer(dir, "gopls", server); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded["gopls"].Command != "gopls" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if err := RemoveProjectServer(dir, "gopls"); err != nil {
		t.Fatal(err)
	}
	loaded, _ = LoadProjectServers(dir)
	if len(loaded) != 0 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if err := RemoveProjectServer(dir, "missing"); err == nil {
		t.Fatal("expected error removing missing server")
	}
}
