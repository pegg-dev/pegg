package loadskill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/utils/query"
)

var (
	testBus = event.New()
	testMgr *session.Manager
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{Choices: []llm.Choice{{
		Index: 0,
		Message: func() *llm.Message {
			m := llm.AssistantMessage("sub answer")
			return &m
		}(),
		FinishReason: func() *llm.FinishReason { fr := llm.FinishReasonStop; return &fr }(),
	}}}, nil
}
func (stubProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error { return nil }
func (stubProvider) ListModels(context.Context) ([]llm.Model, error)                   { return nil, nil }

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "loadskill-test-")
	if err != nil {
		os.Exit(1)
	}
	if err := os.Chdir(tmp); err != nil {
		os.Exit(1)
	}
	store, err := session.NewJSONStoreAt(".")
	if err != nil {
		os.Exit(1)
	}
	testMgr = session.NewManager(store, testBus, logger.New(logger.LevelDebug, discardHandler{}))
	subagent.SubAgentTools(testMgr)
	LoadSkillTool(testMgr)
	os.Exit(m.Run())
}

func setupTestSkill(t *testing.T, name, content string) {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, skill.SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := skill.LoadDir(root, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkillRegistered(t *testing.T) {
	if _, ok := tools.Get("loadskill"); !ok {
		t.Error("loadskill tool not registered")
	}
}

func TestLoadSkillBasic(t *testing.T) {
	setupTestSkill(t, "test-skill", `---
name: test-skill
description: A test skill
---
# Test Skill
This is a test.
`)

	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(context.Background(), `{"name": "test-skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "<skill:test-skill>") {
		t.Errorf("expected skill tag, got:\n%s", out)
	}
	if !contains(t, out, "Description: A test skill") {
		t.Errorf("expected description, got:\n%s", out)
	}
	if !contains(t, out, "# Test Skill") {
		t.Errorf("expected skill content, got:\n%s", out)
	}
}

func TestLoadSkillWithArgs(t *testing.T) {
	setupTestSkill(t, "arg-skill", `---
name: arg-skill
description: Skill with arguments
arguments:
  - filename
  - action
---
# $action $filename
Perform $action on $filename.
`)

	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(context.Background(), `{"name": "arg-skill", "arguments": {"filename": "main.go", "action": "refactor"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "# refactor main.go") {
		t.Errorf("expected substituted content, got:\n%s", out)
	}
	if !contains(t, out, "Perform refactor on main.go.") {
		t.Errorf("expected substituted body, got:\n%s", out)
	}
}

func TestLoadSkillMissingName(t *testing.T) {
	tt, _ := tools.Get("loadskill")
	if _, err := tt.Execute(context.Background(), `{}`); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadSkillNotFound(t *testing.T) {
	tt, _ := tools.Get("loadskill")
	if _, err := tt.Execute(context.Background(), `{"name": "nonexistent"}`); err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestLoadSkillWithWhenToUse(t *testing.T) {
	setupTestSkill(t, "when-skill", `---
name: when-skill
description: Skill with when_to_use
when_to_use: Use when the user asks to deploy
---
# Deploy
Deploy the application.
`)

	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(context.Background(), `{"name": "when-skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "When to use: Use when the user asks to deploy") {
		t.Errorf("expected when_to_use, got:\n%s", out)
	}
}

func TestLoadSkillPartialArgs(t *testing.T) {
	setupTestSkill(t, "partial-skill", `---
name: partial-skill
description: Skill with partial args
arguments:
  - name
  - extra
---
# Hello $name
Extra: $extra
`)

	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(context.Background(), `{"name": "partial-skill", "arguments": {"name": "world"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "# Hello world") {
		t.Errorf("expected substituted name, got:\n%s", out)
	}
	if !contains(t, out, "Extra: $extra") {
		t.Errorf("expected unsubstituted extra, got:\n%s", out)
	}
}

func TestLoadSkillInvalidJSON(t *testing.T) {
	tt, _ := tools.Get("loadskill")
	if _, err := tt.Execute(context.Background(), `not json`); err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestLoadSkillForkContext(t *testing.T) {
	setupTestSkill(t, "fork-skill", `---
name: fork-skill
description: Fork skill
context: fork
---
Do the forked work.
`)

	parent := agent.New("fork-parent")
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	parent.Bus = testBus

	s, err := testMgr.Create(session.CreateOptions{Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testMgr.AppendMessage(s.ID, llm.UserMessage("hello")); err != nil {
		t.Fatal(err)
	}
	testBus.Publish(session.TopicSessionAttached, session.SessionAttached{AgentID: parent.ID, SessionID: s.ID})

	ctx := agent.WithAgent(context.Background(), parent)
	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(ctx, `{"name": "fork-skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "background") {
		t.Errorf("expected background subagent confirmation, got:\n%s", out)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		list, _, err := testMgr.List(query.Query{})
		if err == nil {
			for _, forked := range list {
				if forked.ParentID == s.ID {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("parent session was not forked")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoadSkillForkNoParent(t *testing.T) {
	setupTestSkill(t, "fork-noparent", `---
name: fork-noparent
description: Fork skill without parent
context: fork
---
Do the forked work.
`)

	tt, _ := tools.Get("loadskill")
	if _, err := tt.Execute(context.Background(), `{"name": "fork-noparent"}`); err == nil {
		t.Error("expected error without parent agent in context")
	}
}

func TestLoadSkillForkNoSession(t *testing.T) {
	setupTestSkill(t, "fork-nosession", `---
name: fork-nosession
description: Fork skill without session
context: fork
---
Do the forked work.
`)

	parent := agent.New("nosession-parent")
	parent.Provider = stubProvider{}
	parent.Model = llm.Model{ID: "stub-model"}
	parent.Bus = testBus

	ctx := agent.WithAgent(context.Background(), parent)
	tt, _ := tools.Get("loadskill")
	out, err := tt.Execute(ctx, `{"name": "fork-nosession"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(t, out, "background") {
		t.Errorf("expected background subagent confirmation, got:\n%s", out)
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
