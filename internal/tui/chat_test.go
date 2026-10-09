package tui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/page/settings"
)

type chatEchoProvider struct {
	chunks  []llm.StreamChunk
	lastReq *llm.Request
}

func (p *chatEchoProvider) Name() string { return "echo" }
func (p *chatEchoProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	p.lastReq = req
	var b string
	for _, c := range p.chunks {
		b += c.Content
	}
	return &llm.Response{Choices: []llm.Choice{{Message: &llm.Message{Role: llm.RoleAssistant, Content: b}}}}, nil
}
func (p *chatEchoProvider) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	p.lastReq = req
	for _, c := range p.chunks {
		if err := handler(c); err != nil {
			return err
		}
	}
	return handler(llm.StreamChunk{IsDone: true, FinishReason: llm.FinishReasonStop})
}
func (p *chatEchoProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func newChatApp(t *testing.T, a *agent.Agent) (*App, event.Bus) {
	t.Helper()
	t.Chdir(t.TempDir())
	bus := event.New()
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{
		bus:         bus,
		deps:        settings.Deps{Agent: a, Bus: bus},
		agent:       a,
		screen:      newTestScreen(t),
		subs:        make(map[string]*agentTranscript),
		subItemByID: make(map[string]*components.ChatItem),
		ctx:         ctx,
		cancel:      cancel,
	}
	t.Cleanup(cancel)
	if err := app.subscribeChat(bus); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { app.unsubscribeChat(bus) })
	app.build()
	return app, bus
}

func TestChatEventPipeline(t *testing.T) {
	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{chunks: []llm.StreamChunk{{Content: "hi there"}}}), agent.WithModel(llm.Model{ID: "m"}))
	app, bus := newChatApp(t, orch)

	subID := "sub-1"
	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{AgentID: subID, AgentName: "developer", Model: llm.Model{ID: "m"}})
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: subID, AgentName: "developer", Content: "working", Reasoning: "thinking hard"})

	waitFor(t, 2*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return app.main != nil && len(app.main.items) == 1
	})

	app.chatMu.Lock()
	subItem := app.main.items[0]
	app.chatMu.Unlock()
	if subItem.Kind != components.ItemSubagent || subItem.AgentID != subID {
		t.Fatalf("main should show subagent item, got %+v", subItem)
	}

	sub := app.subs[subID]
	if sub == nil {
		t.Fatal("subagent transcript missing")
	}
	app.chatMu.Lock()
	defer app.chatMu.Unlock()
	if len(sub.items) != 2 {
		t.Fatalf("subagent items = %d, want 2 (thinking + assistant)", len(sub.items))
	}
	if sub.items[0].Kind != components.ItemThinking || sub.items[0].Reasoning != "thinking hard" {
		t.Errorf("subagent thinking item = %+v", sub.items[0])
	}
	if sub.items[1].Kind != components.ItemAssistant || sub.items[1].Text != "working" {
		t.Errorf("subagent assistant item = %+v", sub.items[1])
	}
}

func TestChatSubagentViewSwitch(t *testing.T) {
	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, bus := newChatApp(t, orch)

	subID := "sub-view-1"
	bus.Publish(agent.TopicAgentStarted, agent.AgentStarted{AgentID: subID, AgentName: "planner"})
	bus.Publish(agent.TopicAgentToken, agent.AgentToken{AgentID: subID, AgentName: "planner", Content: "planning"})

	waitFor(t, 2*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return app.subs[subID] != nil && len(app.subs[subID].items) > 0
	})

	app.chatMu.Lock()
	subItem := app.main.items[0]
	app.chatMu.Unlock()
	app.activateItem(subItem)

	if app.viewID != subID {
		t.Errorf("viewID = %q, want subagent", app.viewID)
	}
	if !app.chat.HasBack() {
		t.Error("chat should show the back header in subagent view")
	}
	if app.chat.Items()[0].Kind != components.ItemAssistant || app.chat.Items()[0].Text != "planning" {
		t.Errorf("subagent view first item = %+v", app.chat.Items()[0])
	}

	app.backFromSubagent()
	if app.viewID != orch.ID {
		t.Errorf("after back viewID = %q, want main (%q)", app.viewID, orch.ID)
	}
	if app.chat.HasBack() {
		t.Error("back header should be cleared")
	}
}

func TestChatFullRunEndToEnd(t *testing.T) {
	orch := agent.New("orch",
		agent.WithProvider(&chatEchoProvider{chunks: []llm.StreamChunk{
			{Reasoning: "thinking...", Content: "Answer"},
			{Content: " is 42"},
		}}),
		agent.WithModel(llm.Model{ID: "echo-model"}),
	)
	app, _ := newChatApp(t, orch)

	app.submitMessage("what is the answer?")

	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		if app.main == nil {
			return false
		}
		return len(app.main.items) >= 3 && !app.running
	})

	app.chatMu.Lock()
	defer app.chatMu.Unlock()
	var userText, asstText string
	var thinking bool
	for _, it := range app.main.items {
		switch it.Kind {
		case components.ItemUser:
			userText = it.Text
		case components.ItemAssistant:
			asstText += it.Text
		case components.ItemThinking:
			thinking = true
		}
	}
	if userText != "what is the answer?" {
		t.Errorf("user item = %q", userText)
	}
	if asstText != "Answer is 42" {
		t.Errorf("assistant text = %q, want 'Answer is 42'", asstText)
	}
	if !thinking {
		t.Error("expected a thinking item for reasoning tokens")
	}
}

func TestChatToolItemAndResult(t *testing.T) {
	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, bus := newChatApp(t, orch)

	bus.Publish(agent.TopicAgentToolCall, agent.AgentToolCall{
		AgentID:   orch.ID,
		AgentName: "orch",
		Call: llm.ToolCall{
			ID:       "call-1",
			Function: llm.Function{Name: "edit", Arguments: `{"filePath":"a.go","oldString":"old","newString":"new"}`},
		},
	})
	bus.Publish(agent.TopicAgentToolResult, agent.AgentToolResult{
		AgentID:  orch.ID,
		CallID:   "call-1",
		ToolName: "edit",
		Output:   "edited",
	})

	app.chatMu.Lock()
	defer app.chatMu.Unlock()
	if app.main == nil || len(app.main.items) != 1 {
		t.Fatalf("expected one tool item, got %d", len(app.main.items))
	}
	it := app.main.items[0]
	if it.Kind != components.ItemTool {
		t.Fatalf("item kind = %v, want tool", it.Kind)
	}
	if it.ToolOutput != "edited" {
		t.Errorf("tool output = %q, want edited", it.ToolOutput)
	}
	if !components.HasDiff(it.Diff) {
		t.Error("edit tool should carry a diff")
	}
}

func TestChatSendsPreviousMessages(t *testing.T) {
	prov := &chatEchoProvider{chunks: []llm.StreamChunk{{Content: "turn one reply"}}}
	orch := agent.New("orch",
		agent.WithProvider(prov),
		agent.WithModel(llm.Model{ID: "m"}),
		agent.WithSystemPrompt("you are a test assistant"),
	)
	app, _ := newChatApp(t, orch)

	app.submitMessage("hello")
	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return !app.running && app.main != nil && len(app.main.items) >= 3
	})

	app.submitMessage("follow up")
	waitFor(t, 5*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return !app.running
	})

	if prov.lastReq == nil {
		t.Fatal("provider never received a request")
	}
	var roles []llm.Role
	var texts []string
	for _, m := range prov.lastReq.Messages {
		roles = append(roles, m.Role)
		if c, ok := m.Content.(string); ok {
			texts = append(texts, c)
		}
	}
	joined := strings.Join(texts, "|")

	if !containsRole(roles, llm.RoleSystem) {
		t.Errorf("second request should include the system prompt, roles=%v", roles)
	}
	if !strings.Contains(joined, "hello") {
		t.Errorf("second request is missing the first user message: %q", joined)
	}
	if !strings.Contains(joined, "turn one reply") {
		t.Errorf("second request is missing the first assistant reply: %q", joined)
	}
	if !strings.Contains(joined, "follow up") {
		t.Errorf("second request is missing the current user message: %q", joined)
	}
}

func containsRole(roles []llm.Role, want llm.Role) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

func TestAppSkillPickerHasBuiltinSkills(t *testing.T) {
	root := t.TempDir()
	if err := skill.MaterializeTo(root); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if err := skill.LoadDirs(root); err != nil {
		t.Fatalf("load skills: %v", err)
	}

	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, _ := newChatApp(t, orch)
	hp := app.home
	hp.HandleKey(tcell.NewEventKey(tcell.KeyRune, '/', 0))
	hp.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', 0))
	if hp.PickCount() == 0 {
		t.Error("builtin skills should appear in the picker (found batch)")
	}
	if it, ok := hp.SkillPicker().Selected(); ok && it.Label != "batch" {
		t.Errorf("selected = %q, want batch", it.Label)
	}
}

func TestAppMentionPickerHasAgents(t *testing.T) {
	for _, name := range []string{"developer", "explorer", "planner"} {
		n := name
		agents.Register(func() (*agent.Agent, error) {
			return agent.New(n), nil
		})
	}

	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, _ := newChatApp(t, orch)
	hp := app.home
	hp.HandleKey(tcell.NewEventKey(tcell.KeyRune, '@', 0))
	hp.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'd', 0))
	if hp.PickCount() == 0 {
		t.Error("mention picker should show registered agents")
	}
	if it, ok := hp.MentionPicker().Selected(); ok && it.Label != "developer" {
		t.Errorf("selected = %q, want developer", it.Label)
	}
}

func TestChatLoadMore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewSQLiteStore()
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sessBus := event.New()
	mgr := session.NewManager(store, sessBus, logger.New(logger.LevelDebug, discardHandler{}))

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := mgr.Create(session.CreateOptions{Title: "load", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i := 1; i <= 130; i++ {
		role := llm.RoleUser
		if i%2 == 0 {
			role = llm.RoleAssistant
		}
		if _, err := mgr.AppendMessage(sess.ID, llm.Message{Role: role, Content: "msg-" + itoa(i)}); err != nil {
			t.Fatalf("append msg %d: %v", i, err)
		}
	}
	allMsgs, err := mgr.Messages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}

	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, _ := newChatApp(t, orch)
	app.deps.Sessions = mgr

	app.chatMu.Lock()
	app.session = &activeSession{info: settings.SessionInfo{ID: sess.ID, Title: "load", Messages: allMsgs}}
	app.loadSessionIntoChatLocked()
	app.chatMu.Unlock()

	if got := len(app.chat.Items()); got != 50 {
		t.Fatalf("initial chat items = %d, want 50", got)
	}
	if !app.chat.HasMore() {
		t.Fatal("hasMore should be true with older messages")
	}
	first := app.chat.Items()[0]
	if first.Kind != components.ItemUser || first.Text != "msg-81" {
		t.Errorf("first loaded item = %+v, want msg-81", first)
	}

	app.loadMore()

	if got := len(app.chat.Items()); got != 100 {
		t.Fatalf("after loadMore chat items = %d, want 100", got)
	}
	if app.chat.Items()[0].Text != "msg-31" {
		t.Errorf("after loadMore first item = %q, want msg-31", app.chat.Items()[0].Text)
	}
	if !app.chat.HasMore() {
		t.Fatal("hasMore should still be true after one batch")
	}

	app.loadMore()
	if got := len(app.chat.Items()); got != 130 {
		t.Fatalf("after second loadMore chat items = %d, want 130", got)
	}
	if app.chat.Items()[0].Text != "msg-1" {
		t.Errorf("first item = %q, want msg-1", app.chat.Items()[0].Text)
	}
	if app.chat.HasMore() {
		t.Error("hasMore should be false after reaching the first message")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestAppErrorMessageShownAndCleared(t *testing.T) {
	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, bus := newChatApp(t, orch)

	bus.Publish(agent.TopicErrorMessage, agent.ErrorMessage{
		AgentID: orch.ID,
		Message: "Request failed (attempt 2): connection reset — retrying in 10s",
	})

	waitFor(t, 2*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return app.errorMsg != ""
	})

	app.chatMu.Lock()
	got := app.errorMsg
	app.chatMu.Unlock()
	if !strings.Contains(got, "connection reset") || !strings.Contains(got, "attempt 2") || !strings.Contains(got, "10s") {
		t.Fatalf("error msg = %q, want failure + attempt + backoff info", got)
	}

	bus.Publish(agent.TopicErrorMessageFinished, agent.ErrorMessageFinished{AgentID: orch.ID})
	waitFor(t, 2*time.Second, func() bool {
		app.chatMu.Lock()
		defer app.chatMu.Unlock()
		return app.errorMsg == ""
	})
}

func TestAppErrorMessageIgnoresOtherAgents(t *testing.T) {
	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "m"}))
	app, bus := newChatApp(t, orch)

	bus.Publish(agent.TopicErrorMessage, agent.ErrorMessage{AgentID: "sub-1", Message: "boom"})
	time.Sleep(50 * time.Millisecond)
	app.chatMu.Lock()
	defer app.chatMu.Unlock()
	if app.errorMsg != "" {
		t.Fatalf("error msg = %q, want empty for subagent events", app.errorMsg)
	}
}

var _ = tcell.NewEventKey
