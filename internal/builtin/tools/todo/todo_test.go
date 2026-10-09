package todo

import (
	"context"
	"os"
	"testing"

	"github.com/peggco/pegg/internal/agent"
)

const testSessionID = "test-session"

func setupTodoTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.sessions = make(map[string]*sessionStore)
	store.agentSessions = make(map[string]string)
	store.mu.Unlock()
}

func sessionContext(t *testing.T) context.Context {
	t.Helper()
	a := &agent.Agent{ID: "test-agent"}
	store.mu.Lock()
	store.agentSessions[a.ID] = testSessionID
	store.mu.Unlock()
	return agent.WithAgent(context.Background(), a)
}

func resetSession(t *testing.T) {
	t.Helper()
	store.mu.Lock()
	delete(store.sessions, testSessionID)
	store.mu.Unlock()
}

func TestTodoreadToolEmpty(t *testing.T) {
	setupTodoTest(t)
	tool := todoreadTool(nil)

	out, err := tool.Execute(sessionContext(t), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "No todos") {
		t.Errorf("expected 'No todos', got:\n%s", out)
	}
}

func TestTodowriteToolSet(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)

	out, err := tool.Execute(sessionContext(t), `{"todos": [
		{"id": "todo-1", "title": "fix bug", "description": "fix the critical bug", "status": "pending", "priority": "high"}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "fix bug") {
		t.Errorf("expected todo title in output, got:\n%s", out)
	}

	all, err := store.all(sessionContext(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(all))
	}
	if all[0].ID != "todo-1" || all[0].Title != "fix bug" || all[0].Priority != "high" {
		t.Errorf("unexpected todo: %+v", all[0])
	}
	if all[0].CreatedAt.IsZero() || all[0].UpdatedAt.IsZero() {
		t.Errorf("expected timestamps to be set: %+v", all[0])
	}
}

func TestTodowriteToolReplace(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)
	ctx := sessionContext(t)

	if _, err := tool.Execute(ctx, `{"todos": [
		{"id": "todo-1", "title": "first", "status": "pending", "priority": "medium"}
	]}`); err != nil {
		t.Fatal(err)
	}

	out, err := tool.Execute(ctx, `{"todos": [
		{"id": "todo-1", "title": "first updated", "status": "completed", "priority": "high"},
		{"id": "todo-2", "title": "second", "status": "pending", "priority": "low"}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "first updated") {
		t.Errorf("expected 'first updated' in output, got:\n%s", out)
	}
	if !contains(t, out, "second") {
		t.Errorf("expected 'second' in output, got:\n%s", out)
	}

	all, err := store.all(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(all))
	}
}

func TestTodowriteToolClear(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)
	ctx := sessionContext(t)

	if _, err := tool.Execute(ctx, `{"todos": [
		{"id": "todo-1", "title": "first", "status": "pending", "priority": "medium"}
	]}`); err != nil {
		t.Fatal(err)
	}

	if _, err := tool.Execute(ctx, `{"todos": []}`); err != nil {
		t.Fatal(err)
	}

	all, err := store.all(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("expected no todos, got %d", len(all))
	}
}

func TestTodowriteToolInvalidJSON(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)

	if _, err := tool.Execute(sessionContext(t), `not json`); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestTodowriteToolWithDependsOn(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)
	ctx := sessionContext(t)

	_, err := tool.Execute(ctx, `{"todos": [
		{"id": "todo-1", "title": "first", "status": "completed", "priority": "medium"},
		{"id": "todo-2", "title": "second", "status": "pending", "priority": "medium", "dependsOn": ["todo-1"]}
	]}`)
	if err != nil {
		t.Fatal(err)
	}

	listTool := todoreadTool(nil)
	listOut, err := listTool.Execute(ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, listOut, "depends on: todo-1") {
		t.Errorf("expected dependency info, got:\n%s", listOut)
	}
}

func TestTodoPersistence(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)
	ctx := sessionContext(t)

	if _, err := tool.Execute(ctx, `{"todos": [
		{"id": "todo-1", "title": "persistent task", "status": "pending", "priority": "medium"}
	]}`); err != nil {
		t.Fatal(err)
	}

	resetSession(t)

	listTool := todoreadTool(nil)
	out, err := listTool.Execute(ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "persistent task") {
		t.Errorf("expected todo to persist, got:\n%s", out)
	}
}

func TestTodoSessionIsolation(t *testing.T) {
	setupTodoTest(t)
	tool := todowriteTool(nil)

	ctxA := sessionContext(t)
	ctxB := agent.WithAgent(context.Background(), &agent.Agent{ID: "other-agent"})
	store.mu.Lock()
	store.agentSessions["other-agent"] = "other-session"
	store.mu.Unlock()

	if _, err := tool.Execute(ctxA, `{"todos": [
		{"id": "todo-1", "title": "session a task", "status": "pending", "priority": "medium"}
	]}`); err != nil {
		t.Fatal(err)
	}

	listA, err := store.all(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 {
		t.Fatalf("expected 1 todo in session A, got %d", len(listA))
	}

	listB, err := store.all(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	if len(listB) != 0 {
		t.Fatalf("expected no todos in session B, got %d", len(listB))
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct{ status, expected string }{
		{"completed", "[x]"},
		{"in_progress", "[~]"},
		{"cancelled", "[-]"},
		{"pending", "[ ]"},
		{"unknown", "[ ]"},
	}
	for _, tt := range tests {
		got := statusIcon(tt.status)
		if got != tt.expected {
			t.Errorf("statusIcon(%q) = %q, want %q", tt.status, got, tt.expected)
		}
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
