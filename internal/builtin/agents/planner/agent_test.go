package planner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func newTestFS(t *testing.T) *vfs.VFS {
	t.Helper()
	root := t.TempDir()
	fs, err := vfs.New(root, vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Write("main.go", []byte("package main\n")); err != nil {
		t.Fatal(err)
	}
	return fs
}

func execTool(t *testing.T, t2 tool.Tool, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return t2.Execute(context.Background(), string(raw))
}

func plannerTools(t *testing.T) map[string]tool.Tool {
	t.Helper()
	fs := newTestFS(t)
	a, err := newPlannerAgent(fs)
	if err != nil {
		t.Fatal(err)
	}
	reg := make(map[string]tool.Tool)
	for _, name := range []string{"read", "write", "edit", "list", "glob", "grep"} {
		tk, ok := a.Tools.Get(name)
		if !ok {
			t.Fatalf("planner missing file tool %q", name)
		}
		reg[name] = tk
	}
	return reg
}

func TestPlannerStartsInPlanMode(t *testing.T) {
	fs := newTestFS(t)
	a, err := newPlannerAgent(fs)
	if err != nil {
		t.Fatal(err)
	}
	r := a.StandingReminder()
	if r == nil {
		t.Fatal("planner must have a standing plan-mode reminder")
	}
	if r.Tag != "plan_mode" {
		t.Fatalf("tag = %q, want plan_mode", r.Tag)
	}
	if !strings.Contains(r.Content, "READ-ONLY") {
		t.Fatalf("reminder content = %q, want read-only constraint", r.Content)
	}
}

func TestPlannerReadsFullCodebase(t *testing.T) {
	reg := plannerTools(t)

	out, err := execTool(t, reg["read"], map[string]any{"filePath": "main.go"})
	if err != nil {
		t.Fatalf("read codebase file: %v", err)
	}
	if !strings.Contains(out, "package main") {
		t.Fatalf("read returned %q", out)
	}

	out, err = execTool(t, reg["list"], map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("list root: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("list missing codebase file:\n%s", out)
	}

	out, err = execTool(t, reg["glob"], map[string]any{"pattern": "**/*.go"})
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("glob missing codebase file:\n%s", out)
	}

	out, err = execTool(t, reg["grep"], map[string]any{"pattern": "package main"})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Fatalf("grep missing codebase file:\n%s", out)
	}
}

func TestPlannerWritesOnlyToPlans(t *testing.T) {
	fs := newTestFS(t)
	a, err := newPlannerAgent(fs)
	if err != nil {
		t.Fatal(err)
	}
	reg := make(map[string]tool.Tool)
	for _, name := range []string{"read", "write", "edit", "list", "glob", "grep"} {
		tk, ok := a.Tools.Get(name)
		if !ok {
			t.Fatalf("planner missing file tool %q", name)
		}
		reg[name] = tk
	}

	scope := ".pegg/plans"
	if _, err := reg["write"].Execute(context.Background(), `{"filePath": "`+scope+`/spec-1.md", "content": "step 1"}`); err != nil {
		t.Fatalf("write into plans: %v", err)
	}
	if out, err := execTool(t, reg["read"], map[string]any{"filePath": scope + "/spec-1.md"}); err != nil {
		t.Fatalf("read back plan file: %v", err)
	} else if !strings.Contains(out, "step 1") {
		t.Fatalf("read returned %q", out)
	}

	if _, err := execTool(t, reg["read"], map[string]any{"filePath": "main.go"}); err != nil {
		t.Fatalf("read codebase file: %v", err)
	}
	for _, target := range []string{"main.go", "README.md"} {
		if _, err := execTool(t, reg["write"], map[string]any{"filePath": target, "content": "x"}); err == nil {
			t.Fatalf("write to %s must fail", target)
		}
		if _, err := execTool(t, reg["edit"], map[string]any{"filePath": target, "oldString": "package", "newString": "package2"}); err == nil {
			t.Fatalf("edit to %s must fail", target)
		}
	}

	if data, err := os.ReadFile(filepath.Join(fs.Root(), "main.go")); err != nil || !strings.Contains(string(data), "package main") {
		t.Fatalf("main.go was modified: %q, %v", string(data), err)
	}
	phys := filepath.Join(fs.Root(), scope, "spec-1.md")
	if _, err := os.Stat(phys); err != nil {
		t.Fatalf("plan file not created on disk: %v", err)
	}
}

func TestPlannerReadsPlansDespiteGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".pegg/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := vfs.New(root, vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Write("main.go", []byte("package main\n")); err != nil {
		t.Fatal(err)
	}

	a, err := newPlannerAgent(fs)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := a.Tools.Get("read")
	write, _ := a.Tools.Get("write")

	scope := ".pegg/plans"
	if _, err := write.Execute(context.Background(), `{"filePath": "`+scope+`/spec-1.md", "content": "plan body"}`); err != nil {
		t.Fatalf("write into gitignored plans dir: %v", err)
	}
	if out, err := read.Execute(context.Background(), `{"filePath": "`+scope+`/spec-1.md"}`); err != nil {
		t.Fatalf("read plan file from gitignored dir: %v", err)
	} else if !strings.Contains(out, "plan body") {
		t.Fatalf("read returned %q", out)
	}
}
