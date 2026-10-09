package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func testLogger() *logger.Logger {
	return logger.New(logger.LevelDebug, discardHandler{})
}

func testCache(t *testing.T) cache.Cache {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	c, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

type scriptedProvider struct {
	name string
}

func (p *scriptedProvider) Name() string { return p.name }

func (p *scriptedProvider) Chat(_ context.Context, req *llm.Request) (*llm.Response, error) {
	var last string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if last = llm.MessageText(req.Messages[i]); last != "" {
			break
		}
	}
	content := `<observation><type>bugfix</type><title>Fixed auth</title><narrative>Added retry-once on 401.</narrative></observation>`
	switch {
	case strings.Contains(last, "Summarize this finished run"):
		content = `<summary><request>fix auth</request><learned>refresh tokens expire</learned><completed>fixed endpoint</completed><next_steps>add expiry tests</next_steps></summary>`
	case strings.Contains(last, "Consolidate the latest session"):
		content = `<memory_update><active_context>## Active context
- fixed auth refresh, next: expiry tests</active_context><decisions>## Decisions
- retry-once on 401</decisions><lessons>## Lessons
- [global] refresh tokens expire after 24h</lessons><notes>## Notes
- token.go: refresh flow</notes></memory_update>`
	}
	msg := llm.AssistantMessage(content)
	fr := llm.FinishReasonStop
	return &llm.Response{
		Model:   "scripted",
		Choices: []llm.Choice{{Index: 0, Message: &msg, FinishReason: &fr}},
	}, nil
}

func (p *scriptedProvider) ChatStream(_ context.Context, _ *llm.Request, _ llm.StreamHandler) error {
	return nil
}

func (p *scriptedProvider) ListModels(_ context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "scripted-1"}}, nil
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestManagerCaptureFlushAndPanel(t *testing.T) {
	bus := event.New()
	llmMgr := llm.NewManager(bus, testLogger(), testCache(t))
	if err := llmMgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(llmMgr.Shutdown)
	llm.RegisterProvider("memtest-llm", func(cfg config.LLMConfig) (llm.Provider, error) {
		return &scriptedProvider{name: cfg.Provider}, nil
	})
	llmMgr.Sync(context.Background(), []config.LLMConfig{{Provider: "memtest-llm"}})

	cfg := defaultCfg()
	m := newManager(Deps{Config: cfg, LLM: llmMgr, Bus: bus, Log: testLogger()}, t.TempDir())
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{AgentID: "a1", AgentName: "orchestrator"})
	bus.Publish(agent.TopicAgentInput, agent.AgentInput{AgentID: "a1", AgentName: "orchestrator", Input: "fix the auth flow"})
	bus.Publish(agent.TopicAgentToolCall, agent.AgentToolCall{
		AgentID: "a1", AgentName: "orchestrator",
		Call: llm.ToolCall{ID: "c1", Type: "function", Function: llm.Function{Name: "edit", Arguments: `{"file_path":"internal/auth/token.go"}`}},
	})
	bus.Publish(agent.TopicAgentToolResult, agent.AgentToolResult{
		AgentID: "a1", AgentName: "orchestrator", CallID: "c1", ToolName: "edit",
		Output: "file updated",
	})
	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: "a1", AgentName: "orchestrator",
		Message: llm.AssistantMessage("fixed the refresh endpoint"),
	})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{
		AgentID: "a1", AgentName: "orchestrator", Output: "done",
	})

	waitFor(t, 10*time.Second, func() bool {
		count, _, _ := m.Store().Stats()
		return count >= 2
	})

	idx, err := m.Store().ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, e := range idx {
		types[e.Type]++
	}
	if types["bugfix"] != 1 || types["summary"] != 1 {
		t.Fatalf("index types = %v", types)
	}

	if block, ok := m.Store().ReadBlock(idx[0].File, idx[0].ID); ok {
		if !strings.Contains(block, "internal/auth/token.go") {
			t.Fatalf("block missing file evidence: %q", block)
		}
	} else {
		t.Fatal("observation block not found")
	}

	waitFor(t, 10*time.Second, func() bool {
		return strings.Contains(m.Store().ReadCurated(activeCtxFile), "fixed auth")
	})
	if lessons := m.Store().ReadCurated(lessonsFile); !strings.Contains(lessons, "refresh tokens expire after 24h") {
		t.Fatalf("lessons = %q", lessons)
	}

	waitFor(t, 5*time.Second, func() bool {
		data, err := os.ReadFile(m.Store().globalLessonsPath())
		return err == nil && strings.Contains(string(data), "refresh tokens expire after 24h")
	})

	panel := m.BuildPanel()
	if panel == "" {
		t.Fatal("panel is empty")
	}
	if !strings.Contains(panel, "# Project memory") {
		t.Fatalf("panel missing header: %q", panel)
	}
	if !strings.Contains(panel, "Fixed auth") {
		t.Fatalf("panel missing observation: %q", panel)
	}

	in := m.expandInput(agent.MessageInput{Text: "continue"})
	if in.Text != "continue" {
		t.Fatalf("user input must stay untouched, got: %q", in.Text)
	}
	if len(in.SystemReminders) != 1 || !strings.Contains(in.SystemReminders[0], "# Project memory") {
		t.Fatalf("system reminders = %+v", in.SystemReminders)
	}
}

type mockDecision struct {
	name    string
	answers map[string]float64
}

func (d *mockDecision) Name() string { return d.name }

func (d *mockDecision) Decide(_ context.Context, req *decisionapi.Request) (*decisionapi.Response, error) {
	answers := make(map[string]decisionapi.Answer, len(req.Questions))
	for id := range req.Questions {
		p := d.answers[id]
		answers[id] = decisionapi.Answer{Type: decisionapi.QuestionNoul, Noul: &p}
	}
	return &decisionapi.Response{Model: req.Model, Provider: d.name, Answers: answers}, nil
}

func TestManagerGateHitsWithDecision(t *testing.T) {
	bus := event.New()
	decMgr := decisionapi.NewManager(bus, testLogger())
	if err := decMgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(decMgr.Shutdown)

	decisionapi.RegisterProvider("memtest-dec", func(cfg config.LLMConfig) (decisionapi.Provider, error) {
		return &mockDecision{name: cfg.Provider, answers: map[string]float64{"rel_0": 0.9, "rel_1": 0.1}}, nil
	})
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: "memtest-dec", APIKey: "k"}},
	})

	cfg := defaultCfg()
	cfg.Provider = "memtest-dec"
	m := newManager(Deps{Config: cfg, Decision: decMgr, Bus: bus}, t.TempDir())

	hits := []SearchHit{
		{ID: "obs-aaaaaa", Title: "Auth token refresh", Text: "refresh", Score: 10},
		{ID: "obs-bbbbbb", Title: "Cache eviction", Text: "cache", Score: 5},
	}
	kept := m.gateHits(context.Background(), "token refresh", hits)
	if len(kept) != 1 || kept[0].ID != "obs-aaaaaa" {
		t.Fatalf("kept = %+v, want only the relevant one", kept)
	}
}

func TestManagerDisabled(t *testing.T) {
	bus := event.New()
	cfg := defaultCfg()
	cfg.Enabled = false
	m := newManager(Deps{Config: cfg, Bus: bus}, t.TempDir())

	m.handleToolResult(agent.AgentToolResult{AgentID: "a1", AgentName: "orch", CallID: "c1", ToolName: "bash", Output: "x"})
	if len(m.turns) != 0 {
		t.Fatal("capture should be skipped when disabled")
	}
	if panel := m.BuildPanel(); panel != "" {
		t.Fatalf("panel should be empty when disabled: %q", panel)
	}
}

func TestSkipTools(t *testing.T) {
	bus := event.New()
	cfg := defaultCfg()
	cfg.SkipTools = []string{"bash"}
	m := newManager(Deps{Config: cfg, Bus: bus}, t.TempDir())

	m.handleToolResult(agent.AgentToolResult{AgentID: "a1", AgentName: "orch", CallID: "c1", ToolName: "bash", Output: "x"})
	m.handleToolResult(agent.AgentToolResult{AgentID: "a1", AgentName: "orch", CallID: "c2", ToolName: "read", Output: "y"})
	m.mu.Lock()
	defer m.mu.Unlock()
	turn := m.turns["a1"]
	if turn == nil || len(turn.toolUses) != 1 || turn.toolUses[0].Name != "read" {
		t.Fatalf("toolUses = %+v", turn)
	}
}
