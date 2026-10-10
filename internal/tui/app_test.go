package tui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/terminfo"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/page/home"
	"github.com/peggco/pegg/internal/tui/page/settings"
	"github.com/peggco/pegg/internal/tui/styles"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

type mockProvider struct {
	name   string
	models []llm.Model
}

func (p *mockProvider) Name() string { return p.name }
func (p *mockProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, nil
}
func (p *mockProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return nil
}
func (p *mockProvider) ListModels(context.Context) ([]llm.Model, error) { return p.models, nil }

func newTestManager(t *testing.T) *llm.Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	bus := event.New()
	mgr := llm.NewManager(bus, logger.New(logger.LevelDebug, discardHandler{}), store)
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Shutdown)
	return mgr
}

func TestAppSelectPreferredAtStartup(t *testing.T) {
	mgr := newTestManager(t)
	llm.RegisterProvider("tui-mock", func(cfg config.LLMConfig) (llm.Provider, error) {
		return &mockProvider{name: cfg.Provider, models: []llm.Model{{ID: "mock-1", Name: "Mock One"}}}, nil
	})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "tui-mock"}})

	a := &App{deps: settings.Deps{LLM: mgr}}
	a.selectPreferred()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.model.provider != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a.model.provider != "tui-mock" || a.model.model.ID != "mock-1" {
		t.Fatalf("preferred selection = %+v", a.model)
	}
	if got := a.modelDisplay(); got != "tui-mock/Mock One" {
		t.Errorf("modelDisplay = %q, want tui-mock/Mock One", got)
	}
}

func TestAppSelectPreferredNoLLM(t *testing.T) {
	a := &App{}
	a.selectPreferred()
	if a.model.provider != "" {
		t.Errorf("expected no selection without LLM, got %+v", a.model)
	}
}

func TestAppOpenSettingsSeedsModel(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s}
	a.model = selectedModel{provider: "p", model: llm.Model{ID: "m", Name: "M"}}
	a.openSettings()
	ov, ok := a.getOverlay().(*settings.Settings)
	if !ok {
		t.Fatalf("overlay is %T, want *settings.Settings", a.getOverlay())
	}
	if got := ov.ModelDisplay(); got != "p/M" {
		t.Errorf("seeded model display = %q, want p/M", got)
	}
}

func TestAppSettingsModelPickUpdatesApp(t *testing.T) {
	mgr := newTestManager(t)
	llm.RegisterProvider("tui-mock2", func(cfg config.LLMConfig) (llm.Provider, error) {
		return &mockProvider{name: cfg.Provider, models: []llm.Model{{ID: "m-a"}, {ID: "m-b"}}}, nil
	})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "tui-mock2"}})

	bus := event.New()
	a := &App{
		screen: newTestScreen(t),
		bus:    bus,
		deps: settings.Deps{
			LLM:    mgr,
			Bus:    bus,
			Config: &config.Config{Providers: []config.LLMConfig{{Provider: "tui-mock2"}}},
		},
	}
	a.openSettings()
	ov := a.getOverlay().(*settings.Settings)

	ov.HandleKey(newKey(tcell.KeyDown))
	ov.HandleKey(newKey(tcell.KeyDown))
	ov.HandleKey(newKey(tcell.KeyEnter))
	if !ov.HasSub() {
		t.Fatal("model picker should be open")
	}
	ov.HandleKey(newKey(tcell.KeyEnter))
	if ov.HasSub() {
		t.Fatal("picker should close after selection")
	}
	if a.model.provider != "tui-mock2" || a.model.model.ID != "m-a" {
		t.Errorf("app model after pick = %+v, want tui-mock2/m-a", a.model)
	}
}

func TestAppEscWhileRunningArmsInterrupt(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s, chat: components.NewChat(), running: true}
	t.Cleanup(a.clearEscHint)

	a.handleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))

	if !a.escHint || a.lastEsc.IsZero() {
		t.Fatalf("Esc while running should arm the interrupt: hint=%v lastEsc=%v", a.escHint, a.lastEsc)
	}
}

func TestAppEscWithOverlayDoesNotArmInterrupt(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s, chat: components.NewChat(), running: true}
	a.setOverlay(components.NewChat())

	a.handleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))

	if a.escHint || !a.lastEsc.IsZero() {
		t.Fatalf("Esc with an overlay open must not arm the interrupt: hint=%v lastEsc=%v", a.escHint, a.lastEsc)
	}
}

func TestAppEscInSubagentViewDoesNotArmInterrupt(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s, chat: components.NewChat(), running: true}
	a.chat.SetBack(true)

	a.handleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0))

	if a.escHint || !a.lastEsc.IsZero() {
		t.Fatalf("Esc in a subagent transcript must not arm the interrupt: hint=%v lastEsc=%v", a.escHint, a.lastEsc)
	}
}

func newKey(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, 0) }

func TestAppSessionLoadUpdatesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewSQLiteStore()
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	bus := event.New()
	mgr := session.NewManager(store, bus, logger.New(logger.LevelDebug, discardHandler{}))

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := mgr.Create(session.CreateOptions{Title: "My Session", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "hello there"}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	a := &App{screen: newTestScreen(t), deps: settings.Deps{Sessions: mgr, Bus: bus}}
	a.build()
	if a.root == nil {
		t.Fatal("home page not built")
	}
	hp := a.root.(*home.Page)
	if hp.Chat().HasItems() {
		t.Fatal("chat should start empty")
	}

	a.openSettings()
	ov := a.getOverlay().(*settings.Settings)
	ov.HandleKey(newKey(tcell.KeyTab))
	ov.HandleKey(newKey(tcell.KeyEnter))
	ov.HandleKey(newKey(tcell.KeyEnter))

	if a.session == nil {
		t.Fatal("session should be loaded into the app")
	}
	if a.session.info.Title != "My Session" {
		t.Errorf("session title = %q, want My Session", a.session.info.Title)
	}
	if len(a.session.info.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(a.session.info.Messages))
	}
	if !hp.Chat().HasItems() {
		t.Error("home chat should show the loaded session")
	}
	if a.getOverlay() != nil {
		t.Error("settings modal should close automatically after loading a session")
	}
}

func newTestScreen(t *testing.T) tcell.Screen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(s.Fini)
	s.SetSize(100, 30)
	return s
}

func TestAppQuitsOnCtrlC(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s}
	done := make(chan error, 1)
	go func() { done <- a.start() }()

	time.Sleep(100 * time.Millisecond)
	if err := s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)); err != nil {
		t.Fatalf("post event: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not quit on Ctrl+C")
	}
}

func TestAppQuitsOnCtrlQ(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s}
	done := make(chan error, 1)
	go func() { done <- a.start() }()

	time.Sleep(100 * time.Millisecond)
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModCtrl))

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not quit on Ctrl+Q")
	}
}

func TestAppSubmitPublishesEvent(t *testing.T) {
	t.Chdir(t.TempDir())
	s := newTestScreen(t)
	bus := event.New()
	setBus(bus)
	defer setBus(nil)

	got := make(chan SubmitEvent, 1)
	handler := func(e SubmitEvent) { got <- e }
	if err := bus.Subscribe(TopicSubmit, handler); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer bus.Unsubscribe(TopicSubmit, handler)

	a := &App{bus: bus, screen: s}
	done := make(chan error, 1)
	go func() { done <- a.start() }()

	time.Sleep(100 * time.Millisecond)
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone))
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyRune, 'i', tcell.ModNone))
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	select {
	case e := <-got:
		if e.Message != "hi" {
			t.Errorf("submit message = %q, want hi", e.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no submit event received")
	}

	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not quit")
	}
}

func TestAppSettingsModal(t *testing.T) {
	s := newTestScreen(t)
	a := &App{screen: s}
	done := make(chan error, 1)
	go func() { done <- a.start() }()

	time.Sleep(100 * time.Millisecond)
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModCtrl))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.getOverlay() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if a.getOverlay() == nil {
		t.Fatal("Ctrl+P did not open the settings overlay")
	}

	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	closed := false
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.getOverlay() == nil {
			closed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !closed {
		t.Error("Esc did not close the settings overlay")
	}

	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not quit")
	}
}

func TestAppThemeCycle(t *testing.T) {
	s := newTestScreen(t)
	styles.Set("dark")
	a := &App{screen: s}
	done := make(chan error, 1)
	go func() { done <- a.start() }()

	time.Sleep(100 * time.Millisecond)
	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModCtrl))

	changed := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if styles.Name() != "dark" {
			changed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	_ = s.PostEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not quit")
	}
	if !changed {
		t.Error("theme did not change after Ctrl+T")
	}
}

func TestPrepareTerminfo(t *testing.T) {
	ti := &terminfo.Terminfo{Name: "xterm", EnterKeypad: "\x1b[?1h\x1b=", ExitKeypad: "\x1b[?1l\x1b>"}
	got := prepareTerminfo(ti)
	if got == nil {
		t.Fatal("expected a clone")
	}
	if got.EnterKeypad != "" || got.ExitKeypad != "" {
		t.Fatalf("keypad caps not cleared: enter=%q exit=%q", got.EnterKeypad, got.ExitKeypad)
	}
	if ti.EnterKeypad == "" || ti.ExitKeypad == "" {
		t.Fatal("original terminfo must not be mutated")
	}
}

func TestPrepareTerminfoNil(t *testing.T) {
	if got := prepareTerminfo(nil); got != nil {
		t.Fatalf("nil terminfo should yield nil, got %+v", got)
	}
}

func TestRewriteKeypad(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"numlock on kp2", "\x1b[57420;129u", "2"},
		{"numlock on kp4", "\x1b[57417;129u", "4"},
		{"numlock on kp5", "\x1b[57427;129u", "5"},
		{"numlock off kp2 stays nav", "\x1b[57420;1u", "\x1b[57420;1u"},
		{"real arrow untouched", "\x1b[A", "\x1b[A"},
		{"plain digit untouched", "7", "7"},
		{"digit keypad code untouched", "\x1b[57401;129u", "\x1b[57401;129u"},
		{"win32 numpad 2", "\x1b[40;0;50;1;0;1_", "2"},
		{"win32 numpad 4", "\x1b[37;0;52;1;0;1_", "4"},
		{"win32 numpad 5 already works", "\x1b[101;0;53;1;0;1_", "\x1b[101;0;53;1;0;1_"},
		{"win32 numpad 6", "\x1b[39;0;54;1;0;1_", "6"},
		{"win32 numpad 8", "\x1b[38;0;56;1;0;1_", "8"},
		{"win32 release untouched", "\x1b[40;0;50;0;0;1_", "\x1b[40;0;50;0;0;1_"},
		{"win32 real arrow untouched", "\x1b[40;0;0;1;0;1_", "\x1b[40;0;0;1;0;1_"},
		{"win32 ctrl-c untouched", "\x1b[67;0;3;1;8;1_", "\x1b[67;0;3;1;8;1_"},
	}
	for _, c := range cases {
		if got := string(rewriteKeypad([]byte(c.in))); got != c.want {
			t.Errorf("%s: rewriteKeypad(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRewriteKeypadSequence(t *testing.T) {
	in := "\x1b[40;0;50;1;0;1_" + // numpad 2 down
		"\x1b[40;0;50;0;0;1_" + // numpad 2 up (ignored by tcell)
		"\x1b[37;0;52;1;0;1_" + // numpad 4 down
		"\x1b[37;0;52;0;0;1_" + // numpad 4 up
		"\x1b[101;0;53;1;0;1_" // numpad 5 down (already works)
	want := "2" +
		"\x1b[40;0;50;0;0;1_" +
		"4" +
		"\x1b[37;0;52;0;0;1_" +
		"\x1b[101;0;53;1;0;1_"
	if got := string(rewriteKeypad([]byte(in))); got != want {
		t.Fatalf("rewriteKeypad = %q, want %q", got, want)
	}
}

func newSessionApp(t *testing.T) (*App, *session.Manager) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewSQLiteStore()
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	bus := event.New()
	mgr := session.NewManager(store, bus, logger.New(logger.LevelDebug, discardHandler{}))
	a := &App{screen: newTestScreen(t), deps: settings.Deps{Sessions: mgr, Bus: bus}}
	a.build()
	return a, mgr
}

func chatItemByText(items []*components.ChatItem, text string) *components.ChatItem {
	for _, it := range items {
		if it.Kind == components.ItemUser && it.Text == text {
			return it
		}
	}
	return nil
}

func TestAppRevertMessage(t *testing.T) {
	a, mgr := newSessionApp(t)
	dir, _ := os.Getwd()
	sess, err := mgr.Create(session.CreateOptions{Title: "S", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "first"})
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	second, _ := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "second"})
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleAssistant, Content: "reply"})

	info, ok := a.sessionInfoFor(sess.ID)
	if !ok {
		t.Fatal("session info not loaded")
	}
	a.activateSession(info)

	target := chatItemByText(a.chat.Items(), "second")
	if target == nil {
		t.Fatal("second user message not found in chat")
	}

	a.doRevert(sess.ID, second.ID, target)

	msgs, _ := mgr.Messages(sess.ID)
	if len(msgs) != 2 {
		t.Fatalf("messages after revert = %d, want 2", len(msgs))
	}
	if got := a.home.Input().Value(); got != "second" {
		t.Fatalf("input = %q, want second", got)
	}
}

func TestAppResolveUserMessage(t *testing.T) {
	a, mgr := newSessionApp(t)
	dir, _ := os.Getwd()
	sess, err := mgr.Create(session.CreateOptions{Title: "S", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "first"})
	second, _ := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "second"})

	info, _ := a.sessionInfoFor(sess.ID)
	a.activateSession(info)

	target := chatItemByText(a.chat.Items(), "second")
	sid, mid, ok := a.resolveUserMessage(target)
	if !ok || sid != sess.ID || mid != second.ID {
		t.Fatalf("resolveUserMessage = (%q,%q,%v), want (%q,%q,true)", sid, mid, ok, sess.ID, second.ID)
	}

	live := &components.ChatItem{Kind: components.ItemUser, Text: "third"}
	a.chat.AppendItem(live)
	third, _ := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "third"})
	_, mid, ok = a.resolveUserMessage(live)
	if !ok || mid != third.ID {
		t.Fatalf("live resolve = (%q,%v), want %q", mid, ok, third.ID)
	}
}

func TestPrepareAgentRunAppliesModelChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bus := event.New()
	log := logger.New(logger.LevelDebug, discardHandler{})
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

	llm.RegisterProvider("tui-model-prov", func(config.LLMConfig) (llm.Provider, error) {
		return &chatEchoProvider{}, nil
	})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "tui-model-prov"}})

	orch := agent.New("orch", agent.WithProvider(&chatEchoProvider{}), agent.WithModel(llm.Model{ID: "old"}))
	app := &App{
		bus:         bus,
		deps:        settings.Deps{Agent: orch, Bus: bus, LLM: mgr},
		agent:       orch,
		screen:      newTestScreen(t),
		ctx:         context.Background(),
		subs:        map[string]*agentTranscript{},
		subItemByID: map[string]*components.ChatItem{},
	}
	app.model = selectedModel{provider: "tui-model-prov", model: llm.Model{ID: "new"}}

	_, cancel, _ := app.prepareAgentRun("hi", nil)
	cancel()

	if app.agent.Model.ID != "new" {
		t.Fatalf("agent model = %q, want new", app.agent.Model.ID)
	}
	if app.agent.Provider == nil {
		t.Fatal("agent provider should be set")
	}
}

func TestAppForkMessage(t *testing.T) {
	a, mgr := newSessionApp(t)
	dir, _ := os.Getwd()
	sess, err := mgr.Create(session.CreateOptions{Title: "S", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "first"})
	_, _ = mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleAssistant, Content: "hi"})
	second, _ := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "second"})

	info, ok := a.sessionInfoFor(sess.ID)
	if !ok {
		t.Fatal("session info not loaded")
	}
	a.activateSession(info)

	target := chatItemByText(a.chat.Items(), "second")
	if target == nil {
		t.Fatal("second user message not found in chat")
	}

	a.forkMessage(sess.ID, second.ID, target)

	if a.session == nil || a.session.info.ID == sess.ID {
		t.Fatal("active session should switch to the fork")
	}
	if a.session.info.Title != "S - fork" {
		t.Fatalf("fork title = %q, want 'S - fork'", a.session.info.Title)
	}
	forkMsgs, _ := mgr.Messages(a.session.info.ID)
	if len(forkMsgs) != 2 {
		t.Fatalf("fork messages = %d, want 2", len(forkMsgs))
	}
	if got := a.home.Input().Value(); got != "second" {
		t.Fatalf("input = %q, want second", got)
	}
	origMsgs, _ := mgr.Messages(sess.ID)
	if len(origMsgs) != 3 {
		t.Fatalf("original messages = %d, want 3", len(origMsgs))
	}
}
