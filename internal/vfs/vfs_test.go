package vfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peggco/pegg/internal/lsp/diagnostic"
)

func newTestVFS(t *testing.T, root string) *VFS {
	t.Helper()
	fs, err := New(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestNewRequiresDirectory(t *testing.T) {
	if _, err := New("", Options{}); err == nil {
		t.Fatal("empty root must fail")
	}
	if _, err := New("/nonexistent/xyz", Options{}); err == nil {
		t.Fatal("missing root must fail")
	}
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file, Options{}); err == nil {
		t.Fatal("file root must fail")
	}
}

func TestResolveNormal(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	phys, err := fs.Resolve("a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "a", "b", "c.txt"); phys != want {
		t.Fatalf("Resolve = %q, want %q", phys, want)
	}

	if _, err := fs.Resolve("/a/b/c.txt"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("absolute virtual path must be blocked, got %v", err)
	}

	phys, err = fs.Resolve(".")
	if err != nil {
		t.Fatal(err)
	}
	if phys != root {
		t.Fatalf("Resolve(.) = %q, want %q", phys, root)
	}

	if phys := fs.Virtual(filepath.Join(root, "x", "y.go")); phys != "x/y.go" {
		t.Fatalf("Virtual = %q, want %q", phys, "x/y.go")
	}
}

func TestResolveBlocksTraversal(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	for _, vpath := range []string{
		"..",
		"../..",
		"../../etc/passwd",
		"a/../../etc/passwd",
		"a/../..",
		"a/../../..",
		"",
		"a\x00b",
		"/etc/passwd",
		"/a/b.txt",
		"//etc/passwd",
	} {
		if _, err := fs.Resolve(vpath); !errors.Is(err, ErrOutOfBounds) {
			t.Errorf("Resolve(%q) = %v, want ErrOutOfBounds", vpath, err)
		}
	}
}

func TestResolveDotStaysInRoot(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	phys, err := fs.Resolve("a/..")
	if err != nil {
		t.Fatalf("Resolve(a/..) = %v", err)
	}
	if phys != root {
		t.Fatalf("Resolve(a/..) = %q, want root %q", phys, root)
	}
}

func TestResolveAbsolutePath(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	phys, err := fs.Resolve(filepath.ToSlash(root))
	if err != nil {
		t.Fatalf("root itself must resolve: %v", err)
	}
	if phys != root {
		t.Fatalf("Resolve(root) = %q, want %q", phys, root)
	}

	inside := filepath.ToSlash(filepath.Join(root, "src", "main.go"))
	phys, err = fs.Resolve(inside)
	if err != nil {
		t.Fatalf("absolute path inside root must resolve: %v", err)
	}
	if want := filepath.Join(root, "src", "main.go"); phys != want {
		t.Fatalf("Resolve(%q) = %q, want %q", inside, phys, want)
	}

	for _, vpath := range []string{"/etc/passwd", "/a/b.txt", "/etc"} {
		if _, err := fs.Resolve(vpath); !errors.Is(err, ErrOutOfBounds) {
			t.Errorf("Resolve(%q) = %v, want ErrOutOfBounds", vpath, err)
		}
	}
}

func TestResolveHomeTilde(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("in root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("home inside workspace", func(t *testing.T) {
		t.Setenv("HOME", root)
		fs := newTestVFS(t, root)

		phys, err := fs.Resolve("~")
		if err != nil || phys != root {
			t.Fatalf("Resolve(~) = %q, %v; want root", phys, err)
		}
		phys, err = fs.Resolve("~/a.txt")
		if err != nil || phys != filepath.Join(root, "a.txt") {
			t.Fatalf("Resolve(~/a.txt) = %q, %v", phys, err)
		}
		data, err := fs.Read("~/a.txt")
		if err != nil {
			t.Fatalf("Read(~/a.txt): %v", err)
		}
		if !strings.Contains(data, "in root") {
			t.Fatalf("Read(~/a.txt) = %q", data)
		}
		if _, err := fs.Write("~/new.txt", []byte("created")); err != nil {
			t.Fatalf("Write(~/new.txt): %v", err)
		}
	})

	t.Run("home outside workspace", func(t *testing.T) {
		t.Setenv("HOME", outside)
		fs := newTestVFS(t, root)

		for _, vpath := range []string{"~", "~/secret.txt", "~/.config/app"} {
			if _, err := fs.Resolve(vpath); !errors.Is(err, ErrOutOfBounds) {
				t.Errorf("Resolve(%q) = %v, want ErrOutOfBounds", vpath, err)
			}
		}
	})

	t.Run("tilde-user is literal", func(t *testing.T) {
		fs := newTestVFS(t, root)
		phys, err := fs.Resolve("~someone")
		if err != nil {
			t.Fatalf("~someone must stay literal: %v", err)
		}
		if want := filepath.Join(root, "~someone"); phys != want {
			t.Fatalf("Resolve(~someone) = %q, want %q", phys, want)
		}
	})
}

func TestResolveBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "secret-link.txt")); err != nil {
		t.Fatal(err)
	}

	fs := newTestVFS(t, root)
	for _, vpath := range []string{"escape/secret.txt", "secret-link.txt", "escape/../escape/secret.txt"} {
		if _, err := fs.Resolve(vpath); !errors.Is(err, ErrOutOfBounds) {
			t.Errorf("Resolve(%q) = %v, want ErrOutOfBounds", vpath, err)
		}
	}
}

func TestReadSnapshotsHash(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	res, err := fs.Read("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "Lines: 1") {
		t.Fatalf("expected Lines: 1, got %q", res)
	}
	if !strings.Contains(res, "     1: hello world") {
		t.Fatalf("expected numbered line, got %q", res)
	}
	if !strings.Contains(res, "hello world") {
		t.Fatalf("expected 'hello world' in output, got %q", res)
	}

	hash, ok := fs.Snapshot("a.txt")
	if !ok || hash == "" {
		t.Fatal("snapshot should exist after read")
	}
	if hash != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatalf("unexpected hash %q", hash)
	}

	if fs.Exists("missing.txt") {
		t.Fatal("missing file must not exist")
	}
}

func TestReadRange(t *testing.T) {
	root := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	res, err := fs.ReadRange("a.txt", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "Lines: 5") {
		t.Fatalf("expected Lines: 5, got %q", res)
	}
	if !strings.Contains(res, "Range: lines 2-3") {
		t.Fatalf("expected Range: lines 2-3, got %q", res)
	}
	if !strings.Contains(res, "     2: line2") {
		t.Fatalf("expected '     2: line2', got %q", res)
	}
	if !strings.Contains(res, "     3: line3") {
		t.Fatalf("expected '     3: line3', got %q", res)
	}

	res, err = fs.ReadRange("a.txt", 4, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "     4: line4") {
		t.Fatalf("expected '     4: line4', got %q", res)
	}
	if !strings.Contains(res, "     5: line5") {
		t.Fatalf("expected '     5: line5', got %q", res)
	}

	res, err = fs.ReadRange("a.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "     1: line1") {
		t.Fatalf("expected '     1: line1', got %q", res)
	}

	res, err = fs.ReadRange("a.txt", 99, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "Lines: 5") {
		t.Fatalf("offset beyond EOF should return empty window, got %q", res)
	}

	hash, ok := fs.Snapshot("a.txt")
	if !ok || hash == "" {
		t.Fatal("ReadRange must snapshot the content hash")
	}
	if _, err := fs.Edit("a.txt", "line3", "edited", false); err != nil {
		t.Fatalf("edit after ReadRange should be allowed: %v", err)
	}
}

func TestReadIgnoredReturnsDenied(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeIgnoreFiles(t, root, "secret.txt\n", "")
	fs := newTestVFS(t, root)

	if _, err := fs.Read("secret.txt"); !errors.Is(err, ErrIgnored) {
		t.Fatalf("want ErrIgnored, got %v", err)
	}
	if fs.Exists("secret.txt") {
		t.Fatal("ignored file must not exist from agent view")
	}
}

func TestWriteAndEdit(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	if _, err := fs.Write("src/main.go", []byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "src", "main.go"))
	if err != nil || string(data) != "package main\n" {
		t.Fatalf("write failed: data=%q err=%v", data, err)
	}

	if _, err := fs.Edit("src/main.go", "package main", "package app", false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "src", "main.go"))
	if string(data) != "package app\n" {
		t.Fatalf("edit not applied: %q", data)
	}

	if _, err := fs.Edit("src/main.go", "missing", "x", false); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("want ErrNoMatch, got %v", err)
	}
}

func TestEditRequiresRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	if _, err := fs.Edit("a.txt", "hello", "bye", false); !errors.Is(err, ErrReadRequired) {
		t.Fatalf("want ErrReadRequired, got %v", err)
	}
}

func TestEditDetectsExternalChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	if _, err := fs.Read("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("modified externally"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Edit("a.txt", "original", "new", false); !errors.Is(err, ErrFileChanged) {
		t.Fatalf("want ErrFileChanged, got %v", err)
	}

	if _, err := fs.Read("a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Edit("a.txt", "modified externally", "new", false); err != nil {
		t.Fatalf("re-read should allow edit: %v", err)
	}
}

func TestEditReplaceAll(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x x x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)
	if _, err := fs.Read("a.txt"); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Edit("a.txt", "x x", "y y", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(data) != "y y x" {
		t.Fatalf("first-only replace = %q", data)
	}

	if _, err := fs.Edit("a.txt", "x", "y", true); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "a.txt"))
	if string(data) != "y y y" {
		t.Fatalf("replace all = %q", data)
	}
}

func TestAtomicWriteLeavesNoTemp(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	if _, err := fs.Write("a.txt", []byte("data")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "a.txt" {
			t.Fatalf("unexpected leftover entry %q", e.Name())
		}
	}
}

func TestDelete(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)
	if _, err := fs.Write("a.txt", []byte("data")); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Read("a.txt"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Delete("a.txt"); err != nil {
		t.Fatal(err)
	}
	if fs.Exists("a.txt") {
		t.Fatal("file should be gone")
	}
	if _, ok := fs.Snapshot("a.txt"); ok {
		t.Fatal("snapshot should be dropped after delete")
	}
	if err := fs.Delete("a.txt"); err == nil {
		t.Fatal("second delete should fail")
	}
}

func TestHooks(t *testing.T) {
	root := t.TempDir()
	fs, err := New(root, Options{})
	if err != nil {
		t.Fatal(err)
	}

	var readPaths, writePaths, deletePaths []string
	fs.OnBeforeRead(func(path, content string) string {
		readPaths = append(readPaths, path)
		return content
	})
	fs.OnAfterWrite(func(path, content string) string {
		writePaths = append(writePaths, path)
		return content
	})
	fs.OnFileDelete(func(e FileDelete) FileDelete {
		deletePaths = append(deletePaths, e.Path)
		return e
	})

	if _, err := fs.Write("a.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Read("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete("a.txt"); err != nil {
		t.Fatal(err)
	}

	if len(readPaths) != 1 || readPaths[0] != "a.txt" {
		t.Fatalf("before-read hooks = %v", readPaths)
	}
	if len(writePaths) != 1 || writePaths[0] != "a.txt" {
		t.Fatalf("after-write hooks = %v", writePaths)
	}
	if len(deletePaths) != 1 || deletePaths[0] != "a.txt" {
		t.Fatalf("on-delete hooks = %v", deletePaths)
	}
}

func TestHooksCarryDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := New(root, Options{})
	if err != nil {
		t.Fatal(err)
	}

	diag := diagnostic.Diagnostic{
		Severity: 1,
		Message:  "boom",
		Range: diagnostic.Range{
			Start: diagnostic.Position{Line: 0, Character: 0},
			End:   diagnostic.Position{Line: 0, Character: 1},
		},
	}
	fs.OnBeforeRead(func(path, content string) string {
		content += fmt.Sprintf("\n---\nDiagnostics:\n  %s:%d:%d: %s [severity=%d]\n", path, diag.Line(), diag.Column(), diag.Message, diag.Severity)
		return content
	})
	fs.OnAfterWrite(func(path, content string) string {
		content += fmt.Sprintf("\nDiagnostics:\n  %s:%d:%d: %s [severity=%d]\n", path, diag.Line(), diag.Column(), diag.Message, diag.Severity)
		return content
	})

	res, err := fs.Read("a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "boom") {
		t.Fatalf("read output must contain diagnostic 'boom', got %q", res)
	}

	wr, err := fs.Write("b.go", []byte("package b\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wr, "boom") {
		t.Fatalf("write output must contain diagnostic 'boom', got %q", wr)
	}
}

func TestScopedAccess(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)
	if _, err := fs.Write("main.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Write("docs/readme.md", []byte("docs")); err != nil {
		t.Fatal(err)
	}

	plans, err := fs.Scoped(".pegg/plans")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := plans.Write(".pegg/plans/plan.md", []byte("plan")); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Read(".pegg/plans/plan.md"); err != nil {
		t.Fatalf("read inside scope: %v", err)
	}

	if _, err := plans.Read("main.go"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("read outside scope: got %v, want ErrOutOfBounds", err)
	}
	if _, err := plans.Write(".pegg/plans/../plan.md", []byte("x")); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("traversal write: got %v, want ErrOutOfBounds", err)
	}
	if _, err := plans.Read("/etc/hostname"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("absolute read: got %v, want ErrOutOfBounds", err)
	}
	if _, err := plans.Read("~/.bashrc"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("home read: got %v, want ErrOutOfBounds", err)
	}

	dot, err := plans.Resolve(".")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".pegg", "plans")
	if dot != want {
		t.Fatalf("Resolve(.) = %q, want %q", dot, want)
	}

	virtual := plans.Virtual(filepath.Join(root, ".pegg", "plans", "plan.md"))
	if virtual != ".pegg/plans/plan.md" {
		t.Fatalf("Virtual = %q", virtual)
	}

	unscoped, err := fs.Scoped("")
	if err != nil {
		t.Fatal(err)
	}
	if unscoped != fs {
		t.Fatal("Scoped(\"\") must return the receiver")
	}
}

func TestScopedInvalid(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)
	for _, scope := range []string{"..", "../x", "/abs"} {
		if _, err := fs.Scoped(scope); err == nil {
			t.Fatalf("Scoped(%q) must fail", scope)
		}
	}
}

func TestScopedBypassesIgnoreInsideScope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".pegg/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	plans, err := fs.Scoped(".pegg/plans")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Write(".pegg/plans/plan.md", []byte("plan")); err != nil {
		t.Fatalf("write in gitignored scope: %v", err)
	}
	if _, err := plans.Read(".pegg/plans/plan.md"); err != nil {
		t.Fatalf("read in gitignored scope: %v", err)
	}

	if data, err := fs.Read(".pegg/plans/plan.md"); err != nil {
		t.Fatalf("plans dir must be accessible from root despite ignore: %v", err)
	} else if !strings.Contains(data, "plan") {
		t.Fatalf("plans read = %q, want plan content", data)
	}

	if err := os.MkdirAll(filepath.Join(root, ".pegg", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pegg", "sessions", "secret.txt"), []byte("session-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := fs.Read(".pegg/sessions/secret.txt"); err != nil {
		t.Fatalf("all .pegg paths must be readable: %v", err)
	} else if !strings.Contains(data, "session-data") {
		t.Fatalf("sessions read = %q", data)
	}
	if _, err := fs.Write(".pegg/sessions/other.txt", []byte("x")); !errors.Is(err, ErrIgnored) {
		t.Fatalf("writing outside plans must be blocked: got %v, want ErrIgnored", err)
	}
}

func TestWriteScopeReadsWholeRoot(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)
	if _, err := fs.Write("main.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	plans, err := fs.WriteScope(".pegg/plans")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := plans.Read("main.go"); err != nil {
		t.Fatalf("read outside write scope: %v", err)
	}
	if _, err := plans.List("."); err != nil {
		t.Fatalf("list root: %v", err)
	}
	if got, err := plans.Glob("**/*.go", ""); err != nil || len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("glob: %v, %v", got, err)
	}
	if res, err := plans.Grep("package", "", nil, GrepModeFilesWithMatches, 0); err != nil || len(res) != 1 || res[0].Path != "main.go" {
		t.Fatalf("grep: %v, %v", res, err)
	}

	if _, err := plans.Write(".pegg/plans/spec-1.md", []byte("spec")); err != nil {
		t.Fatalf("write into write scope: %v", err)
	}
	if _, err := plans.Read(".pegg/plans/spec-1.md"); err != nil {
		t.Fatalf("read back from write scope: %v", err)
	}

	if _, err := plans.Write("main.go", []byte("x")); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("write outside write scope: got %v, want ErrOutOfBounds", err)
	}
	if _, err := plans.Write(".pegg/plans/../main.go", []byte("x")); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("traversal write: got %v, want ErrOutOfBounds", err)
	}
	if err := plans.Delete(".pegg/plans/spec-1.md"); err != nil {
		t.Fatalf("delete inside write scope: %v", err)
	}
	if err := plans.Delete("main.go"); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("delete outside write scope: got %v, want ErrOutOfBounds", err)
	}

	if data, err := os.ReadFile(filepath.Join(root, "main.go")); err != nil || string(data) != "package main" {
		t.Fatalf("main.go modified: %q, %v", string(data), err)
	}

	unscoped, err := fs.WriteScope("")
	if err != nil {
		t.Fatal(err)
	}
	if unscoped != fs {
		t.Fatal("WriteScope(\"\") must return the receiver")
	}
}

func TestWriteScopeIgnoresGitignoreInsideScope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".pegg/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	plans, err := fs.WriteScope(".pegg/plans")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Write(".pegg/plans/plan.md", []byte("plan")); err != nil {
		t.Fatalf("write in gitignored scope: %v", err)
	}
	if _, err := plans.Read(".pegg/plans/plan.md"); err != nil {
		t.Fatalf("read in gitignored scope: %v", err)
	}

	if data, err := fs.Read(".pegg/plans/plan.md"); err != nil {
		t.Fatalf("plans dir must be accessible from root despite ignore: %v", err)
	} else if !strings.Contains(data, "plan") {
		t.Fatalf("plans read = %q, want plan content", data)
	}

	if err := os.MkdirAll(filepath.Join(root, ".pegg", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pegg", "sessions", "secret.txt"), []byte("session-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := fs.Read(".pegg/sessions/secret.txt"); err != nil {
		t.Fatalf("all .pegg paths must be readable: %v", err)
	} else if !strings.Contains(data, "session-data") {
		t.Fatalf("sessions read = %q", data)
	}
	if _, err := fs.Write(".pegg/sessions/other.txt", []byte("x")); !errors.Is(err, ErrIgnored) {
		t.Fatalf("writing outside plans must be blocked: got %v, want ErrIgnored", err)
	}
}

func TestPeggReadableButOnlyPlansRulesAndSkillsWritable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".pegg/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".pegg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pegg", "permissions.json"), []byte(`{"allowed":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newTestVFS(t, root)

	if data, err := fs.Read(".pegg/permissions.json"); err != nil {
		t.Fatalf("read .pegg file: %v", err)
	} else if !strings.Contains(data, "allowed") {
		t.Fatalf("read .pegg file = %q", data)
	}
	if res, err := fs.List(".pegg"); err != nil || len(res.Entries) == 0 {
		t.Fatalf("list .pegg: %v %+v", err, res.Entries)
	}
	if got, err := fs.Grep("allowed", ".pegg", nil, GrepModeFilesWithMatches, 0); err != nil || len(got) != 1 || got[0].Path != ".pegg/permissions.json" {
		t.Fatalf("grep .pegg: %v %v", got, err)
	}

	if _, err := fs.Write(".pegg/plans/spec-1.md", []byte("spec")); err != nil {
		t.Fatalf("write into plans: %v", err)
	}
	if data, err := fs.Read(".pegg/plans/spec-1.md"); err != nil {
		t.Fatalf("read plans: %v", err)
	} else if !strings.Contains(data, "spec") {
		t.Fatalf("read plans = %q, want spec content", data)
	}

	if _, err := fs.Write(".pegg/rules/01-testing.md", []byte("# Testing\n")); err != nil {
		t.Fatalf("write into rules: %v", err)
	}
	if data, err := fs.Read(".pegg/rules/01-testing.md"); err != nil {
		t.Fatalf("read rules: %v", err)
	} else if !strings.Contains(data, "# Testing") {
		t.Fatalf("read rules = %q, want rule content", data)
	}

	if _, err := fs.Write(".pegg/skills/demo/SKILL.md", []byte("# Demo\n")); err != nil {
		t.Fatalf("write into skills: %v", err)
	}
	if data, err := fs.Read(".pegg/skills/demo/SKILL.md"); err != nil {
		t.Fatalf("read skills: %v", err)
	} else if !strings.Contains(data, "# Demo") {
		t.Fatalf("read skills = %q, want skill content", data)
	}

	if _, err := fs.Write(".pegg/permissions.json", []byte("{}")); !errors.Is(err, ErrIgnored) {
		t.Fatalf("write outside plans/rules/skills: got %v, want ErrIgnored", err)
	}
	if _, err := fs.Edit(".pegg/permissions.json", "allowed", "denied", false); !errors.Is(err, ErrIgnored) {
		t.Fatalf("edit outside plans/rules/skills: got %v, want ErrIgnored", err)
	}
	if err := fs.Delete(".pegg/permissions.json"); !errors.Is(err, ErrIgnored) {
		t.Fatalf("delete outside plans/rules/skills: got %v, want ErrIgnored", err)
	}
}

func TestWriteOnlyRestriction(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)
	if _, err := fs.Write("main.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	if err := fs.SetWriteOnly(PlansDir); err != nil {
		t.Fatalf("SetWriteOnly: %v", err)
	}
	if _, err := fs.Write(PlansDir+"/plan.md", []byte("plan")); err != nil {
		t.Fatalf("write into write-only scope: %v", err)
	}
	if _, err := fs.Write("main.go", []byte("x")); !errors.Is(err, ErrIgnored) {
		t.Fatalf("write outside write-only scope: got %v, want ErrIgnored", err)
	}
	if _, err := fs.Edit("main.go", "package", "package2", false); !errors.Is(err, ErrIgnored) {
		t.Fatalf("edit outside write-only scope: got %v, want ErrIgnored", err)
	}
	if err := fs.Delete("main.go"); !errors.Is(err, ErrIgnored) {
		t.Fatalf("delete outside write-only scope: got %v, want ErrIgnored", err)
	}
	if data, err := fs.Read("main.go"); err != nil {
		t.Fatalf("reads must stay allowed in write-only mode: %v", err)
	} else if !strings.Contains(data, "package main") {
		t.Fatalf("read = %q", data)
	}

	fs.ClearWriteOnly()
	if _, err := fs.Write("main.go", []byte("package main2")); err != nil {
		t.Fatalf("write after ClearWriteOnly: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	root := t.TempDir()
	fs := newTestVFS(t, root)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := fmt.Sprintf("dir/f%d.txt", i)
			if _, err := fs.Write(p, []byte("hello")); err != nil {
				t.Error(err)
				return
			}
			if _, err := fs.Read(p); err != nil {
				t.Error(err)
				return
			}
			if _, err := fs.Edit(p, "hello", "world", false); err != nil {
				t.Error(err)
				return
			}
			if err := fs.Delete(p); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}
