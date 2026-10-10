package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/builtin/agents/orchestrator"
	"github.com/peggco/pegg/internal/builtin/middlewares"
	"github.com/peggco/pegg/internal/builtin/middlewares/compaction"
	"github.com/peggco/pegg/internal/builtin/tools/ask"
	"github.com/peggco/pegg/internal/builtin/tools/loadskill"
	"github.com/peggco/pegg/internal/builtin/tools/plan"
	"github.com/peggco/pegg/internal/builtin/tools/shell"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/builtin/tools/todo"
	"github.com/peggco/pegg/internal/builtin/tools/web"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/router"
	"github.com/peggco/pegg/internal/schedule"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
	"github.com/peggco/pegg/internal/vfs"
)

func TestRunRendererEvents(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", true, true)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	r.banner("p1", "m1")

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID: "main-1", AgentName: "orchestrator",
		Model: llm.Model{ID: "m1"}, Provider: &cliTestProvider{name: "p1"},
	})
	bus.Publish(agent.TopicAgentInput, agent.AgentInput{AgentID: "main-1", AgentName: "orchestrator", Input: "hello"})
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: "main-1", AgentName: "orchestrator", Reasoning: "let me think"})
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: "main-1", AgentName: "orchestrator", Content: "Hello world"})
	bus.Publish(agent.TopicAgentToolCall, agent.AgentToolCall{
		AgentID: "main-1", AgentName: "orchestrator",
		Call: llm.ToolCall{ID: "c1", Function: llm.Function{Name: "read", Arguments: `{"path":"main.go"}`}},
	})
	bus.Publish(agent.TopicAgentToolResult, agent.AgentToolResult{
		AgentID: "main-1", AgentName: "orchestrator", ToolName: "read", Output: "package main",
	})
	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: "main-1", AgentName: "orchestrator", Message: llm.AssistantMessage("Hello world"),
	})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{
		AgentID: "main-1", AgentName: "orchestrator", Iterations: 2,
		Usage: llm.Usage{TotalTokens: 100, Cost: 0.001},
	})

	out := buf.String()
	for _, want := range []string{
		"Running orchestrator (p1/m1)",
		"orchestrator started",
		"> hello",
		"Thinking:",
		"Hello world",
		"tool read",
		`"path":"main.go"`,
		"orchestrator finished",
		"2 iterations",
		"100 tokens",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRunRendererHidesThinking(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", false, false)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	r.banner("p1", "m1")
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: "main-1", AgentName: "orchestrator", Reasoning: "secret reasoning"})
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: "main-1", AgentName: "orchestrator", Content: "Hello world"})

	out := buf.String()
	for _, want := range []string{"Thinking...", "Thinking\n", "Hello world"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "secret reasoning") {
		t.Fatalf("reasoning must be hidden without --show-thinking: %q", out)
	}
}

func TestRunRendererHidesSubagent(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", false, false)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID: "sub-1", AgentName: "explorer",
		Model: llm.Model{ID: "m1"}, Provider: &cliTestProvider{name: "p1"},
	})
	bus.Publish(agent.TopicAgentToolCall, agent.AgentToolCall{
		AgentID: "sub-1", AgentName: "explorer",
		Call: llm.ToolCall{ID: "c1", Function: llm.Function{Name: "glob", Arguments: `{"pattern":"**/*.go"}`}},
	})
	bus.Publish(agent.TopicAgentToolResult, agent.AgentToolResult{
		AgentID: "sub-1", AgentName: "explorer", ToolName: "glob", Output: "main.go",
	})
	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: "sub-1", AgentName: "explorer", Message: llm.AssistantMessage("explorer result"),
	})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "sub-1", AgentName: "explorer", Iterations: 1})

	out := buf.String()
	for _, want := range []string{"Subagent explorer...", "Subagent explorer\n", "explorer started"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	for _, hidden := range []string{"explorer result", "tool glob", "result glob", `"pattern"`} {
		if strings.Contains(out, hidden) {
			t.Fatalf("subagent %q must be hidden without --show-subagent: %q", hidden, out)
		}
	}
}

func TestRunRendererShowsCompaction(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", false, false)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	bus.Publish(compaction.TopicCompactionFinished, compaction.Event{
		AgentID:   "main-1",
		AgentName: "orchestrator",
		Strategy:  "sliding-window",
		Messages:  30,
		Tokens:    8000,
	})

	out := buf.String()
	for _, want := range []string{"Context compacted", "sliding-window", "30 messages", "8000 tokens"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRunRendererSubagentMessage(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", true, true)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{
		AgentID: "sub-1", AgentName: "explorer",
		Model: llm.Model{ID: "m1"}, Provider: &cliTestProvider{name: "p1"},
	})
	bus.Publish(agent.TopicAgentMessage, agent.AgentMessage{
		AgentID: "sub-1", AgentName: "explorer", Message: llm.AssistantMessage("explorer result"),
	})
	bus.Publish(agent.TopicAgentFinished, agent.AgentFinished{AgentID: "sub-1", AgentName: "explorer", Iterations: 1})

	out := buf.String()
	if !strings.Contains(out, "explorer result") {
		t.Fatalf("subagent message not rendered: %q", out)
	}
	if !strings.Contains(out, "explorer finished") {
		t.Fatalf("subagent finished not rendered: %q", out)
	}
}

func TestRunRendererToolError(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", true, true)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	bus.Publish(agent.TopicAgentToolResult, agent.AgentToolResult{
		AgentID: "sub-1", AgentName: "developer", ToolName: "bash", Output: "boom", Err: context.Canceled,
	})
	out := buf.String()
	if !strings.Contains(out, "error [developer] bash: boom") {
		t.Fatalf("tool error not rendered: %q", out)
	}
}

func TestRunRendererMainErrorSuppressed(t *testing.T) {
	bus := event.New()
	var buf bytes.Buffer
	r := newRunRenderer(&buf, nil, "main-1", true, true)
	if err := r.subscribe(bus); err != nil {
		t.Fatal(err)
	}
	defer r.unsubscribe(bus)

	bus.Publish(agent.TopicAgentError, agent.AgentError{AgentID: "main-1", AgentName: "orchestrator", Err: context.Canceled})
	if buf.Len() != 0 {
		t.Fatalf("main agent error should be suppressed (returned by runRun): %q", buf.String())
	}
}

type runStreamProvider struct {
	name    string
	models  []llm.Model
	lastReq *llm.Request
}

func (p *runStreamProvider) Name() string { return p.name }
func (p *runStreamProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{
		Choices: []llm.Choice{{Index: 0, Message: &llm.Message{Role: llm.RoleAssistant, Content: "hello from chat"}}},
	}, nil
}
func (p *runStreamProvider) ChatStream(_ context.Context, req *llm.Request, handler llm.StreamHandler) error {
	p.lastReq = req
	if err := handler(llm.StreamChunk{Reasoning: "thinking about it"}); err != nil {
		return err
	}
	if err := handler(llm.StreamChunk{Content: "Hello "}); err != nil {
		return err
	}
	if err := handler(llm.StreamChunk{Content: "world"}); err != nil {
		return err
	}
	return handler(llm.StreamChunk{FinishReason: llm.FinishReasonStop})
}
func (p *runStreamProvider) ListModels(context.Context) ([]llm.Model, error) { return p.models, nil }

func newRunTestCLI(t *testing.T) (*CLI, *config.Config, *llm.Manager) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	bus := event.New()
	log := testLogger()

	cacheStore, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cacheStore.Close() })

	mgr := llm.NewManager(bus, log, cacheStore)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)

	cfg := config.DefaultConfig()
	sess, err := session.SessionModule(cfg.Session, bus, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	rec := session.NewRecorder(sess, log)
	if err := rec.Start(bus); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rec.Stop(bus) })

	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	todo.TodoTools(fs, bus)
	shell.ShellTools(fs)
	subagent.SubAgentTools(sess)
	ask.AskTool()
	web.WebTools(fs)
	loadskill.LoadSkillTool(sess)
	plan.PlanTools(fs)
	schedule.ScheduleTools(nil)
	_ = memory.RegisterTools(nil)
	middlewares.Create(fs, middlewares.Deps{})
	orchestrator.Register(fs)
	if _, err := agents.New("orchestrator"); err != nil {
		t.Fatal(err)
	}

	addRunProvider(t, cfg, mgr, "runprov", "m1")
	return New(bus, cfg, log, fs, sess, mgr, nil, nil, nil, nil, nil, nil, nil), cfg, mgr
}

func addRunProvider(t *testing.T, cfg *config.Config, mgr *llm.Manager, name string, modelIDs ...string) {
	t.Helper()
	models := make([]llm.Model, len(modelIDs))
	for i, id := range modelIDs {
		models[i] = llm.Model{ID: id}
	}
	llm.RegisterProvider(name, func(config.LLMConfig) (llm.Provider, error) {
		return &runStreamProvider{name: name, models: models}, nil
	})
	cfg.Providers = append(cfg.Providers, config.LLMConfig{Provider: name})
	mgr.Sync(context.Background(), cfg.Providers)
}

func TestRunCommandStreamsOutput(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)

	if err := c.Execute([]string{"run", "hello"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"Running orchestrator (runprov/m1)",
		"> hello",
		"Thinking",
		"Hello world",
		"orchestrator finished",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "thinking about it") {
		t.Fatalf("reasoning must be hidden by default: %q", out)
	}

	buf.Reset()
	if err := c.Execute([]string{"run", "--show-thinking", "hello"}); err != nil {
		t.Fatal(err)
	}
	out = buf.String()
	if !strings.Contains(out, "thinking about it") {
		t.Fatalf("reasoning must be shown with --show-thinking: %q", out)
	}
}

func TestRunCommandChatMode(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("second message\nthird message\nexit\n"))

	if err := c.Execute([]string{"run", "--chat", "hello"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"chat mode",
		"> hello",
		"> second message",
		"> third message",
		"orchestrator finished",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if n := strings.Count(out, "Hello world"); n != 3 {
		t.Fatalf("expected 3 responses (initial + 2 chat turns), got %d: %q", n, out)
	}
	if strings.Contains(out, "> exit") {
		t.Fatalf("exit command should not be sent to the model: %q", out)
	}
}

func TestRunCommandNoMessageStartsChat(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("first message\nsecond message\nexit\n"))

	if err := c.Execute([]string{"run"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"chat mode", "> first message", "> second message"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
	if n := strings.Count(out, "Hello world"); n != 2 {
		t.Fatalf("expected 2 responses (both from chat turns, no initial run), got %d: %q", n, out)
	}
	if !strings.Contains(out, "> first message") || strings.Contains(out, "> hello") {
		t.Fatalf("no initial message should be sent: %q", out)
	}
}

func TestRunCommandSelectModel(t *testing.T) {
	router.ModelOptionsHook.Reset()
	c, cfg, mgr := newRunTestCLI(t)
	addRunProvider(t, cfg, mgr, "runprov2", "m1")

	var buf bytes.Buffer
	c.root.SetOut(&buf)

	var items []string
	selected := 1
	c.picker = func(list []string, _ string) (int, error) {
		items = list
		return selected, nil
	}

	if err := c.Execute([]string{"run", "--select-model", "hello"}); err != nil {
		t.Fatal(err)
	}

	want := []string{"runprov/m1", "runprov2/m1"}
	if len(items) != len(want) {
		t.Fatalf("selector items = %v, want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("selector items = %v, want %v", items, want)
		}
	}
	if !strings.Contains(buf.String(), "Running orchestrator (runprov2/m1)") {
		t.Fatalf("selected model not used: %q", buf.String())
	}
}

func TestRunCommandModelAutoSelectsProvider(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)

	called := false
	c.picker = func(list []string, _ string) (int, error) {
		called = true
		return 0, nil
	}

	if err := c.Execute([]string{"run", "--model", "m1", "hello"}); err != nil {
		t.Fatal(err)
	}

	if called {
		t.Fatal("selector should not be called when only one provider has the model")
	}
	if !strings.Contains(buf.String(), "Running orchestrator (runprov/m1)") {
		t.Fatalf("auto-selected provider not used: %q", buf.String())
	}
}

func TestRunCommandModelMultipleProviders(t *testing.T) {
	c, cfg, mgr := newRunTestCLI(t)
	addRunProvider(t, cfg, mgr, "runprov2", "m1")

	var buf bytes.Buffer
	c.root.SetOut(&buf)

	var items []string
	c.picker = func(list []string, _ string) (int, error) {
		items = list
		return 1, nil
	}

	if err := c.Execute([]string{"run", "--model", "m1", "hello"}); err != nil {
		t.Fatal(err)
	}

	want := []string{"runprov/m1", "runprov2/m1"}
	if len(items) != len(want) {
		t.Fatalf("selector items = %v, want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("selector items = %v, want %v", items, want)
		}
	}
	if !strings.Contains(buf.String(), "Running orchestrator (runprov2/m1)") {
		t.Fatalf("selected provider not used: %q", buf.String())
	}
}

func TestRunCommandProviderSelectsModel(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)

	var items []string
	c.picker = func(list []string, _ string) (int, error) {
		items = list
		return 0, nil
	}

	if err := c.Execute([]string{"run", "--provider", "runprov", "hello"}); err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 || items[0] != "m1" {
		t.Fatalf("selector items = %v, want [m1]", items)
	}
	if !strings.Contains(buf.String(), "Running orchestrator (runprov/m1)") {
		t.Fatalf("selected model not used: %q", buf.String())
	}
}

func TestRunCommandModelNotFound(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	if err := c.Execute([]string{"run", "--model", "nope", "hello"}); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestRunCommandPreferredSessionModel(t *testing.T) {
	c, cfg, mgr := newRunTestCLI(t)
	addRunProvider(t, cfg, mgr, "prefprov", "p1")

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.Create(session.CreateOptions{Title: "last run", Provider: "prefprov", Model: "p1", ProjectDir: dir}); err != nil {
		t.Fatal(err)
	}

	mgr.SetSessionResolver(func() (string, string, bool) {
		q := query.Query{
			Page: query.Page{Number: 1, Size: 20},
			Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
			Filters: []query.Filter{
				{Column: "project_dir", Operator: query.OpEqual, Value: dir},
			},
		}
		sessions, _, err := c.sessions.List(q)
		if err != nil {
			return "", "", false
		}
		for _, s := range sessions {
			if subagent.IsSubagentSession(s.ID) {
				continue
			}
			if s.Provider == "" || s.Model == "" {
				continue
			}
			return s.Provider, s.Model, true
		}
		return "", "", false
	})

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"run", "hello"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Running orchestrator (prefprov/p1)") {
		t.Fatalf("preferred session model not used: %q", buf.String())
	}
}

func TestRunCommandSessionResume(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.sessions.Create(session.CreateOptions{Title: "old chat", Provider: "runprov", Model: "m1", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.UserMessage("old question")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.AssistantMessage("old answer")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"run", "--session", s.ID, "new question"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"Running orchestrator (runprov/m1)",
		"--- session history ---",
		"> old question",
		"old answer",
		"> new question",
		"Hello world",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}

	msgs, err := c.sessions.Messages(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected session to grow to 4 messages, got %d", len(msgs))
	}
}

func TestRunCommandSessionChat(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.sessions.Create(session.CreateOptions{Title: "old chat", Provider: "runprov", Model: "m1", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.UserMessage("old question")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.AssistantMessage("old answer")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("chat turn\nexit\n"))
	if err := c.Execute([]string{"run", "--session", s.ID}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"--- session history ---", "> old question", "chat mode", "> chat turn", "Hello world"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}

	msgs, err := c.sessions.Messages(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected session to grow to 4 messages, got %d", len(msgs))
	}
}

func TestRunCommandSelectSession(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.Create(session.CreateOptions{Title: "alpha session", Provider: "runprov", Model: "m1", ProjectDir: dir}); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.sessions.Create(session.CreateOptions{Title: "beta session", Provider: "runprov", Model: "m1", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)

	var items []string
	c.picker = func(list []string, label string) (int, error) {
		items = list
		if label != "Select session" {
			t.Fatalf("unexpected label %q", label)
		}
		return 0, nil
	}

	if err := c.Execute([]string{"run", "--select-session", "hello"}); err != nil {
		t.Fatal(err)
	}

	if len(items) != 2 || !strings.Contains(items[0], "beta session") || !strings.Contains(items[1], "alpha session") {
		t.Fatalf("picker items = %v, want the two sessions (most recent first)", items)
	}
	if !strings.Contains(buf.String(), "Hello world") {
		t.Fatalf("selected session not resumed: %q", buf.String())
	}
	msgs, err := c.sessions.Messages(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected resumed session to gain 2 messages, got %d", len(msgs))
	}
}

func TestRunCommandSelectSessionPagination(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 12)
	for i := range ids {
		s, err := c.sessions.Create(session.CreateOptions{Title: fmt.Sprintf("session %02d", i), Provider: "runprov", Model: "m1", ProjectDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = s.ID
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)

	calls := 0
	c.picker = func(items []string, label string) (int, error) {
		calls++
		if calls == 1 {
			for i, it := range items {
				if strings.Contains(it, "next page") {
					return i, nil
				}
			}
			t.Fatalf("no next page item in %v", items)
		}
		return 0, nil
	}

	if err := c.Execute([]string{"run", "--select-session", "hello"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 picker calls (page 1 then page 2), got %d", calls)
	}
	msgs, err := c.sessions.Messages(ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected session %s (page 2) to be resumed, got %d messages", ids[1], len(msgs))
	}
}

func TestRunCommandFileAttachment(t *testing.T) {
	c, cfg, mgr := newRunTestCLI(t)
	prov := &runStreamProvider{name: "attachprov", models: []llm.Model{{ID: "am1"}}}
	llm.RegisterProvider("attachprov", func(config.LLMConfig) (llm.Provider, error) {
		return prov, nil
	})
	cfg.Providers = append(cfg.Providers, config.LLMConfig{Provider: "attachprov"})
	mgr.Sync(context.Background(), cfg.Providers)

	dir := t.TempDir()
	img := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(img, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"run", "--model", "am1", "--file", img, "--file", txt, "hello"}); err != nil {
		t.Fatal(err)
	}

	if prov.lastReq == nil {
		t.Fatal("provider did not receive a request")
	}
	content, ok := prov.lastReq.Messages[1].Content.(llm.Content)
	if !ok {
		t.Fatalf("user message content = %T, want llm.Content", prov.lastReq.Messages[1].Content)
	}
	if content.Text != "hello" {
		t.Fatalf("content text = %q, want hello", content.Text)
	}
	if len(content.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(content.Attachments))
	}
	imgAtt, txtAtt := content.Attachments[0], content.Attachments[1]
	if imgAtt.Type != llm.AttachmentTypeImage || imgAtt.MediaType != "image/png" || imgAtt.FileName != "pic.png" {
		t.Fatalf("image attachment = %+v, want image/png pic.png", imgAtt)
	}
	if txtAtt.Type != llm.AttachmentTypeFile || txtAtt.MediaType != "text/plain" || txtAtt.FileName != "notes.txt" {
		t.Fatalf("file attachment = %+v, want text/plain notes.txt", txtAtt)
	}
	if imgAtt.Data == "" || txtAtt.Data == "" {
		t.Fatalf("attachments must carry base64 data")
	}

	out := buf.String()
	if !strings.Contains(out, "attached") || !strings.Contains(out, "pic.png") || !strings.Contains(out, "notes.txt") {
		t.Fatalf("attached line missing: %q", out)
	}
}

func TestRunCommandFileAttachmentMissing(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	if err := c.Execute([]string{"run", "--file", "/nonexistent/file.txt", "hello"}); err == nil {
		t.Fatal("expected error for missing attachment file")
	}
}

func TestLoadAttachmentsMediaTypes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		ext     string
		attType llm.AttachmentType
		media   string
	}{
		{"photo", ".jpg", llm.AttachmentTypeImage, "image/jpeg"},
		{"diagram", ".png", llm.AttachmentTypeImage, "image/png"},
		{"doc", ".pdf", llm.AttachmentTypeFile, "application/pdf"},
		{"notes", ".txt", llm.AttachmentTypeFile, "text/plain"},
		{"data", ".json", llm.AttachmentTypeFile, "application/json"},
		{"binary", ".bin", llm.AttachmentTypeFile, "application/octet-stream"},
	}
	paths := make([]string, len(cases))
	for i, tc := range cases {
		p := filepath.Join(dir, tc.name+tc.ext)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[i] = p
	}
	atts, err := loadAttachments(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != len(cases) {
		t.Fatalf("attachments = %d, want %d", len(atts), len(cases))
	}
	for i, tc := range cases {
		if atts[i].Type != tc.attType || atts[i].MediaType != tc.media || atts[i].FileName != tc.name+tc.ext {
			t.Fatalf("attachment %d = %+v, want %s %s", i, atts[i], tc.attType, tc.media)
		}
	}
}

func TestRunCommandStdinMessage(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("piped input\n"))

	if err := c.Execute([]string{"run"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "> piped input") {
		t.Fatalf("stdin message not used: %q", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Fatalf("no response: %q", out)
	}
}

func TestRunCommandStdinChatMode(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("first message\nsecond message\nexit\n"))

	if err := c.Execute([]string{"run", "--chat"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"chat mode", "> first message", "> second message"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestRunCommandArgsTakePrecedenceOverEmptyStdin(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader(""))

	if err := c.Execute([]string{"run", "explicit message"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "> explicit message") {
		t.Fatalf("args message not used: %q", out)
	}
}

func TestRootCommandPipedInputCombined(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("file contents here"))

	if err := c.Execute([]string{"check this"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "check this") {
		t.Fatalf("args not in output: %q", out)
	}
	if !strings.Contains(out, "file contents here") {
		t.Fatalf("stdin not in output: %q", out)
	}
}

func TestRootCommandPipedInputArgsOnly(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader(""))

	if err := c.Execute([]string{"just args"}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "> just args") {
		t.Fatalf("args message not used: %q", out)
	}
}

func TestRootCommandPipedInputStdinOnly(t *testing.T) {
	c, _, _ := newRunTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	c.root.SetIn(strings.NewReader("just stdin"))

	if err := c.Execute([]string{}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "> just stdin") {
		t.Fatalf("stdin message not used: %q", out)
	}
}
