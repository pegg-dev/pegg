package schedule

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func newTestTool(t *testing.T) *testEnv {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Scheduler.StoreDir = t.TempDir()
	mgr, err := New(Deps{Config: cfg})
	if err != nil {
		t.Fatalf("New manager: %v", err)
	}
	manager = mgr
	return &testEnv{tool: scheduleTool()}
}

type testEnv struct {
	tool interface {
		Execute(ctx context.Context, args string) (string, error)
	}
}

func (e *testEnv) exec(t *testing.T, args string) string {
	t.Helper()
	out, err := e.tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute(%s): %v", args, err)
	}
	return out
}

func TestScheduleToolLifecycle(t *testing.T) {
	env := newTestTool(t)

	out := env.exec(t, `{"action":"create","name":"daily","cron":"0 9 * * *","prompt":"do work","provider":"p","model":"m"}`)
	var created Schedule
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("create output: %v (%s)", err, out)
	}
	if created.ID == "" || created.Name != "daily" {
		t.Fatalf("unexpected create output: %s", out)
	}

	out = env.exec(t, `{"action":"list"}`)
	if !strings.Contains(out, created.ID) {
		t.Fatalf("list did not contain created schedule: %s", out)
	}

	out = env.exec(t, `{"action":"get","id":"`+created.ID+`"}`)
	if !strings.Contains(out, "daily") {
		t.Fatalf("get output: %s", out)
	}

	out = env.exec(t, `{"action":"update","id":"`+created.ID+`","enabled":false,"name":"renamed"}`)
	var updated Schedule
	if err := json.Unmarshal([]byte(out), &updated); err != nil {
		t.Fatalf("update output: %v", err)
	}
	if updated.Enabled || updated.Name != "renamed" {
		t.Fatalf("unexpected update: %+v", updated)
	}

	out = env.exec(t, `{"action":"delete","id":"`+created.ID+`"}`)
	if !strings.Contains(out, "deleted") {
		t.Fatalf("delete output: %s", out)
	}
	if len(manager.List()) != 0 {
		t.Fatal("expected no schedules after delete")
	}
}

func TestScheduleToolValidation(t *testing.T) {
	env := newTestTool(t)

	if _, err := env.tool.Execute(context.Background(), `{"action":"create","name":"x"}`); err == nil {
		t.Fatal("expected error when cron and prompt missing")
	}
	if _, err := env.tool.Execute(context.Background(), `{"action":"get"}`); err == nil {
		t.Fatal("expected error when id missing")
	}
	if _, err := env.tool.Execute(context.Background(), `{"action":"bogus"}`); err == nil {
		t.Fatal("expected error for unknown action")
	}
	if _, err := env.tool.Execute(context.Background(), `{"action":"create","name":"x","cron":"not a cron","prompt":"p"}`); err == nil {
		t.Fatal("expected error for invalid cron")
	}
}
