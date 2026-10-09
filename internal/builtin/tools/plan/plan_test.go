package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/vfs"
)

func TestEnterPlanModeAttachesReminder(t *testing.T) {
	a := agent.New("test-agent")
	ctx := agent.WithAgent(context.Background(), a)

	out, err := enterplanmodeTool(nil).Execute(ctx, "{}")
	if err != nil {
		t.Fatalf("enterplanmode: %v", err)
	}
	if !strings.Contains(out, "Plan mode enabled") {
		t.Errorf("output = %q, want confirmation", out)
	}

	r := a.StandingReminder()
	if r == nil {
		t.Fatal("no standing reminder attached after enterplanmode")
	}
	if r.Tag != "plan_mode" {
		t.Errorf("tag = %q, want plan_mode", r.Tag)
	}
	if !strings.Contains(r.Content, "READ-ONLY") {
		t.Errorf("content = %q, want read-only constraint", r.Content)
	}
}

func TestExitPlanModeDetachesReminder(t *testing.T) {
	a := agent.New("test-agent")
	ctx := agent.WithAgent(context.Background(), a)

	if _, err := enterplanmodeTool(nil).Execute(ctx, "{}"); err != nil {
		t.Fatalf("enterplanmode: %v", err)
	}
	if _, err := exitplanmodeTool(nil).Execute(ctx, "{}"); err != nil {
		t.Fatalf("exitplanmode: %v", err)
	}
	if r := a.StandingReminder(); r != nil {
		t.Fatalf("standing reminder still attached after exitplanmode: %+v", r)
	}
}

func TestPlanModeToolsRequireAgent(t *testing.T) {
	if _, err := enterplanmodeTool(nil).Execute(context.Background(), "{}"); err == nil {
		t.Fatal("enterplanmode without agent should error")
	}
	if _, err := exitplanmodeTool(nil).Execute(context.Background(), "{}"); err == nil {
		t.Fatal("exitplanmode without agent should error")
	}
}

func TestPlanModeRestrictsWritesToPlans(t *testing.T) {
	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Write("main.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	a := agent.New("test-agent")
	ctx := agent.WithAgent(context.Background(), a)

	if _, err := enterplanmodeTool(fs).Execute(ctx, "{}"); err != nil {
		t.Fatalf("enterplanmode: %v", err)
	}
	if _, err := fs.Write(".pegg/plans/plan.md", []byte("plan")); err != nil {
		t.Fatalf("write to plans while in plan mode: %v", err)
	}
	if _, err := fs.Write("main.go", []byte("x")); err == nil {
		t.Fatal("write outside plans must fail while in plan mode")
	}
	if _, err := fs.Read("main.go"); err != nil {
		t.Fatalf("reads must stay allowed while in plan mode: %v", err)
	}

	if _, err := exitplanmodeTool(fs).Execute(ctx, "{}"); err != nil {
		t.Fatalf("exitplanmode: %v", err)
	}
	if _, err := fs.Write("main.go", []byte("package main2")); err != nil {
		t.Fatalf("write after exiting plan mode: %v", err)
	}
}
