package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peggco/pegg/internal/vfs"
)

func setupTestVFS(t *testing.T, files map[string]string) *vfs.VFS {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := vfs.New(dir, vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestReadTool(t *testing.T) {
	fs := setupTestVFS(t, map[string]string{
		"hello.txt": "line one\nline two\nline three\n",
	})
	tool := readTool(fs)

	out, err := tool.Execute(t.Context(), `{"filePath": "hello.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "<file>") {
		t.Errorf("expected output to contain '<file>', got:\n%s", out)
	}
	if !contains(t, out, "</file>") {
		t.Errorf("expected output to contain '</file>', got:\n%s", out)
	}
	if !contains(t, out, "line one") {
		t.Errorf("expected output to contain 'line one', got:\n%s", out)
	}
	if !contains(t, out, "line two") {
		t.Errorf("expected output to contain 'line two', got:\n%s", out)
	}
	if !contains(t, out, "line three") {
		t.Errorf("expected output to contain 'line three', got:\n%s", out)
	}
	if contains(t, out, "File has more lines") {
		t.Errorf("expected no truncation message for complete file, got:\n%s", out)
	}
}

func TestReadToolRange(t *testing.T) {
	fs := setupTestVFS(t, map[string]string{
		"hello.txt": "line one\nline two\nline three\nline four\n",
	})
	tool := readTool(fs)

	out, err := tool.Execute(t.Context(), `{"filePath": "hello.txt", "offset": 2, "limit": 2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "line two") {
		t.Errorf("expected output to contain 'line two', got:\n%s", out)
	}
	if !contains(t, out, "line three") {
		t.Errorf("expected output to contain 'line three', got:\n%s", out)
	}
	if contains(t, out, "line one") {
		t.Errorf("expected output NOT to contain 'line one', got:\n%s", out)
	}
	if !contains(t, out, "File has more lines") {
		t.Errorf("expected truncation message, got:\n%s", out)
	}
	if !contains(t, out, "offset") {
		t.Errorf("expected truncation message to mention 'offset', got:\n%s", out)
	}
}

func TestReadToolNotFound(t *testing.T) {
	fs := setupTestVFS(t, map[string]string{})
	tool := readTool(fs)

	_, err := tool.Execute(t.Context(), `{"filePath": "nonexistent.txt"}`)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadToolMissingPath(t *testing.T) {
	fs := setupTestVFS(t, map[string]string{})
	tool := readTool(fs)

	_, err := tool.Execute(t.Context(), `{}`)
	if err == nil {
		t.Fatal("expected error for missing filePath")
	}
}

func contains(t *testing.T, s, substr string) bool {
	t.Helper()
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
