package orchestrator

import (
	"slices"
	"testing"

	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/builtin/tools/todo"
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

func TestOrchestratorAgentTools(t *testing.T) {
	fs := newTestFS(t)
	todo.TodoTools(fs, nil)
	subagent.SubAgentTools(nil)

	a, err := newOrchestratorAgent(fs)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"task", "taskstatus", "todoread", "todowrite"} {
		if !slices.Contains(a.ToolNames, name) {
			t.Fatalf("orchestrator missing tool name %q", name)
		}
		if _, ok := tools.Get(name); !ok {
			t.Fatalf("tool %q not registered in global registry", name)
		}
	}
}
