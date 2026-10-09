package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestFreshInstallGoplsAutoInstall(t *testing.T) {
	if os.Getenv("PEGG_LSP_INSTALL_E2E") != "1" {
		t.Skip("set PEGG_LSP_INSTALL_E2E=1 to run (network + compile)")
	}

	Register("gopls", config.LanguageServerConfig{
		Command:     "gopls",
		Args:        []string{"serve"},
		FileTypes:   []string{"go"},
		RootMarkers: []string{"go.mod"},
		Install:     "go install golang.org/x/tools/gopls@latest",
	})
	home := t.TempDir()
	gopath := filepath.Join(os.TempDir(), "pegg-fresh-gopath")
	_ = os.RemoveAll(gopath)
	t.Cleanup(func() { _ = os.RemoveAll(gopath) })
	if err := os.MkdirAll(gopath, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("GOPATH", gopath)
	t.Setenv("GOBIN", "")

	cfg := Registered()["gopls"]
	bin, err := ResolveBinary("gopls", cfg)
	if err != nil {
		t.Skipf("auto-install failed (may be offline): %v", err)
	}
	want := filepath.Join(home, ".pegg", "lsps", "gopls")
	if bin != want {
		t.Fatalf("binary = %q, want cache at %q", bin, want)
	}
	if !isExecutableFile(bin) {
		t.Fatalf("cached binary not executable: %q", bin)
	}
	cmd := exec.Command(bin, "help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cached gopls not runnable: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("gopls produced no output")
	}
}
