package lsp_test

import (
	"strings"
	"testing"

	_ "github.com/peggco/pegg/internal/builtin/lsps"
	"github.com/peggco/pegg/internal/lsp"
)

func TestDefaultServersHaveFileTypes(t *testing.T) {
	servers := lsp.Registered()
	if len(servers) == 0 {
		t.Fatal("no default servers")
	}
	for name, cfg := range servers {
		if len(cfg.FileTypes) == 0 {
			t.Errorf("server %q has no filetypes", name)
			continue
		}
		for _, ext := range cfg.FileTypes {
			if ext == "" || ext == "." {
				t.Errorf("server %q has invalid filetype %q", name, ext)
			}
		}
	}
}

func TestDefaultServersHaveResolutionStrategy(t *testing.T) {
	servers := lsp.Registered()
	for name, cfg := range servers {
		if cfg.Command == "" {
			t.Errorf("server %q has no command", name)
		}
		if len(cfg.RootMarkers) == 0 {
			t.Errorf("server %q has no root markers", name)
		}
	}
}

func TestDefaultServersKnownLanguagesCovered(t *testing.T) {
	servers := lsp.Registered()
	gotExt := map[string]bool{}
	for _, cfg := range servers {
		for _, ext := range cfg.FileTypes {
			gotExt[ext] = true
		}
	}
	for _, ext := range []string{
		"astro", "sh", "c", "h", "cs", "clj", "dart", "ex", "fs", "gleam",
		"go", "hs", "java", "jl", "kt", "lua", "nix", "ml", "php", "prisma",
		"py", "razor", "rb", "rs", "swift", "svelte", "tf", "typ", "ts", "vue",
		"yaml", "zig", "jsx", "tsx", "js",
	} {
		if !gotExt[ext] {
			t.Errorf("no server covers .%s", ext)
		}
	}
}

func TestDefaultServerInstallsAreShellSafe(t *testing.T) {
	for name, cfg := range lsp.Registered() {
		if cfg.Install == "" {
			continue
		}
		if len(cfg.Install) > 2000 {
			t.Errorf("server %q install command unreasonably long", name)
		}
		if strings.Contains(cfg.Install, "~/.pegg/lsps/") && !strings.Contains(cfg.Install, `"$HOME/.pegg/lsps/`) && !strings.Contains(cfg.Install, `"$HOME/.pegg/lsps")`) {
			t.Errorf("server %q install writes to cache without quoting $HOME", name)
		}
	}
}
