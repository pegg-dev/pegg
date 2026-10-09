package lsp

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
)

type blobServer struct {
	body   []byte
	hits   int
	mu     sync.Mutex
	ln     net.Listener
	closed chan struct{}
}

func newBlobServer(body []byte) (*blobServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &blobServer{body: body, ln: ln, closed: make(chan struct{})}
	go s.serve()
	return s, nil
}

func (s *blobServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *blobServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	head := line
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			return
		}
		head += l
		if l == "\r\n" || l == "\n" {
			break
		}
	}
	_ = head
	s.mu.Lock()
	s.hits++
	s.mu.Unlock()
	body := s.body
	conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"))
	_, _ = conn.Write(body)
}

func (s *blobServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func (s *blobServer) addr() string {
	return "http://" + s.ln.Addr().String()
}

func (s *blobServer) close() {
	_ = s.ln.Close()
	select {
	case <-s.closed:
	default:
	}
}

func (s *blobServer) waitHit(t *testing.T, target int) {
	deadline := time.After(5 * time.Second)
	for {
		if s.requestCount() >= target {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for server hit")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestFindExecutableOnPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "gopls")
	if err := os.WriteFile(bin, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	found := findExecutable("gopls")
	if found != bin {
		t.Fatalf("found = %q, want %q", found, bin)
	}
}

func TestFindExecutableMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if found := findExecutable("no-such-tool-xyz"); found != "" {
		t.Fatalf("found = %q", found)
	}
}

func TestResolveBinaryDownloadAndCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	blob := []byte("#!/bin/sh\nfake gopls\n")
	srv, err := newBlobServer(blob)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	cfg := config.LanguageServerConfig{
		Command:  "gopls-fake",
		Download: srv.addr() + "/gopls",
	}
	binary, err := ResolveBinary("x", cfg)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, ".pegg", "lsps", "gopls-fake")
	if binary != expected {
		t.Fatalf("binary = %q, want %q", binary, expected)
	}
	data, err := os.ReadFile(expected)
	if err != nil || string(data) != string(blob) {
		t.Fatalf("cached content mismatch: %v %q", err, data)
	}
	fi, _ := os.Stat(expected)
	if (fi.Mode().Perm() & os.FileMode(0o111)) == os.FileMode(0) {
		t.Fatal("downloaded binary must be executable")
	}
	if srv.requestCount() != 1 {
		t.Fatalf("hits = %d, want 1", srv.requestCount())
	}

	binary2, err := ResolveBinary("x", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if binary2 != expected {
		t.Fatalf("binary2 = %q", binary2)
	}
	if srv.requestCount() != 1 {
		t.Fatalf("hits = %d after cache reuse, want 1", srv.requestCount())
	}
}

func TestResolveBinaryNoDownloadConfigured(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := ResolveBinary("gopls", config.LanguageServerConfig{Command: "no-such-xyz"}); err == nil {
		t.Fatal("expected ErrNotFound")
	}
}

func TestResolveBinaryInstallFallback(t *testing.T) {
	home := t.TempDir()
	installedDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOBIN", installedDir)

	binary := filepath.Join(installedDir, "gopls-installed")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nfake\n"), 0755); err != nil {
		t.Fatal(err)
	}

	installer := filepath.Join(t.TempDir(), "install.sh")
	script := "#!/bin/sh\ncp " + binary + " " + filepath.Join(installedDir, "gopls-cmd") + "\n"
	if err := os.WriteFile(installer, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.LanguageServerConfig{
		Command: "gopls-cmd",
		Install: installer,
	}
	resolved, err := ResolveBinary("x", cfg)
	if err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(home, ".pegg", "lsps", "gopls-cmd")
	if resolved != cached {
		t.Fatalf("resolved = %q, want %q", resolved, cached)
	}
	if !isExecutableFile(cached) {
		t.Fatal("installed binary should be cached and executable")
	}

	if resolved2, err := ResolveBinary("x", cfg); err != nil {
		t.Fatal(err)
	} else if resolved2 != cached {
		t.Fatalf("resolved2 = %q", resolved2)
	}
}

func TestResolveBinaryInstallMissingInstaller(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := config.LanguageServerConfig{
		Command: "gopls-cmd",
		Install: "no-such-installer-xyz",
	}
	if _, err := ResolveBinary("x", cfg); err == nil {
		t.Fatal("expected error for missing installer")
	}
}

func TestSplitCommand(t *testing.T) {
	args, err := splitCommand(`go install golang.org/x/tools/gopls@latest`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[0] != "go" || args[1] != "install" || args[2] != "golang.org/x/tools/gopls@latest" {
		t.Fatalf("args = %v", args)
	}

	args, err = splitCommand(`echo "hello world" 'single quote'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[1] != "hello world" || args[2] != "single quote" {
		t.Fatalf("args = %v", args)
	}

	if _, err := splitCommand(`echo "unbalanced`); err == nil {
		t.Fatal("expected unbalanced quote error")
	}
	if _, err := splitCommand(`   `); err == nil {
		t.Fatal("expected empty command error")
	}
}

func TestResolveBinaryInstallShellSnippet(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	srcDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOBIN", binDir)
	t.Setenv("PATH", "/usr/bin:/bin")

	src := filepath.Join(srcDir, "astro-ls")
	if err := os.WriteFile(src, []byte("#!/bin/sh\nfake\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.LanguageServerConfig{
		Command: "astro-ls",
		Install: `mkdir -p "` + binDir + `" && cp "` + src + `" "` + filepath.Join(binDir, "astro-ls") + `"`,
	}
	resolved, err := ResolveBinary("x", cfg)
	if err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(home, ".pegg", "lsps", "astro-ls")
	if resolved != cached {
		t.Fatalf("resolved = %q, want %q", resolved, cached)
	}
	if !isExecutableFile(cached) {
		t.Fatalf("cached binary not executable")
	}
}

func TestResolveBinaryFallsBackToDefaultInstall(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	srcDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOBIN", binDir)
	t.Setenv("PATH", "/usr/bin:/bin")

	src := filepath.Join(srcDir, "my-gopls")
	if err := os.WriteFile(src, []byte("#!/bin/sh\nfake\n"), 0755); err != nil {
		t.Fatal(err)
	}

	prev := defaultLookup
	defaultLookup = func(name string) (config.LanguageServerConfig, bool) {
		if name != "gopls" {
			return config.LanguageServerConfig{}, false
		}
		return config.LanguageServerConfig{
			Command: "gopls",
			Install: `mkdir -p "` + binDir + `" && cp "` + src + `" "` + filepath.Join(binDir, "gopls") + `"`,
		}, true
	}
	t.Cleanup(func() { defaultLookup = prev })

	cfg := config.LanguageServerConfig{
		Command:   "gopls",
		FileTypes: []string{"go"},
	}
	resolved, err := ResolveBinary("gopls", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".pegg", "lsps", "gopls")
	if resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
	if !isExecutableFile(want) {
		t.Fatal("fallback install should produce an executable cache")
	}
}

func TestResolveBinaryNoDefaultFallback(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	prev := defaultLookup
	defaultLookup = func(name string) (config.LanguageServerConfig, bool) {
		return config.LanguageServerConfig{}, false
	}
	t.Cleanup(func() { defaultLookup = prev })

	cfg := config.LanguageServerConfig{Command: "totally-custom-lsp"}
	if _, err := ResolveBinary("totally-custom-lsp", cfg); err == nil {
		t.Fatal("expected ErrNotFound for unknown server without install")
	}
}
