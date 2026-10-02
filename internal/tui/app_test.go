package tui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/vesvai/vesvai/internal/core/cache"
	"github.com/vesvai/vesvai/internal/core/config"
	"github.com/vesvai/vesvai/internal/core/event"
	"github.com/vesvai/vesvai/internal/core/logger"
	"github.com/vesvai/vesvai/internal/llm"
	"github.com/vesvai/vesvai/internal/session"
	"github.com/vesvai/vesvai/internal/tui/components"
	"github.com/vesvai/vesvai/internal/tui/page/home"
	"github.com/vesvai/vesvai/internal/tui/page/settings"
	"github.com/vesvai/vesvai/internal/tui/styles"
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
