package shell

import (
	"testing"
	"time"

	"github.com/peggco/pegg/internal/vfs"
)

func setupTestFS(t *testing.T) *vfs.VFS {
	t.Helper()
	dir := t.TempDir()
	fs, err := vfs.New(dir, vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestBashToolEcho(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	out, err := tool.Execute(t.Context(), `{"command": "echo hello world", "description": "echo hello"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "hello world") {
		t.Errorf("expected 'hello world', got:\n%s", out)
	}
	if !contains(t, out, "Exit code: 0") {
		t.Errorf("expected exit code 0, got:\n%s", out)
	}
}

func TestBashToolExitCode(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	out, err := tool.Execute(t.Context(), `{"command": "exit 42", "description": "exit with code 42"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "Exit code: 42") {
		t.Errorf("expected exit code 42, got:\n%s", out)
	}
}

func TestBashToolStderr(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	out, err := tool.Execute(t.Context(), `{"command": "echo error message >&2", "description": "echo to stderr"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "error message") {
		t.Errorf("expected 'error message' in stderr, got:\n%s", out)
	}
}

func TestBashToolMissingCommand(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	_, err := tool.Execute(t.Context(), `{}`)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBashToolPipe(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	out, err := tool.Execute(t.Context(), `{"command": "echo 'one two three' | wc -w", "description": "count words"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "3") {
		t.Errorf("expected word count 3, got:\n%s", out)
	}
}

func TestBashToolStdoutAndStderr(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	out, err := tool.Execute(t.Context(), `{"command": "echo stdout; echo stderr >&2", "description": "echo stdout and stderr"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "stdout") {
		t.Errorf("expected stdout, got:\n%s", out)
	}
	if !contains(t, out, "stderr") {
		t.Errorf("expected stderr, got:\n%s", out)
	}
}

func TestBashToolTimeout(t *testing.T) {
	fs := setupTestFS(t)
	tool := bashTool(fs)

	start := time.Now()
	out, err := tool.Execute(t.Context(), `{"command": "sleep 5", "description": "test timeout", "timeout": 1}`)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("command ran too long: %v", elapsed)
	}
	if !contains(t, out, "timed out after 1s") {
		t.Errorf("expected timeout message, got:\n%s", out)
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
