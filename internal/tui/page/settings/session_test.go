package settings

import (
	"os"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type sessDiscardHandler struct{}

func (sessDiscardHandler) Write(logger.Record) error { return nil }
func (sessDiscardHandler) Close() error              { return nil }

func newSessionManager(t *testing.T) (*session.Manager, *session.SQLiteStore) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewSQLiteStore()
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	bus := event.New()
	mgr := session.NewManager(store, bus, logger.New(logger.LevelDebug, sessDiscardHandler{}))
	return mgr, store
}

func TestSessionTabDisabledWithoutActive(t *testing.T) {
	s := New(Deps{})
	st := s.session
	if !st.rowEnabled(0) || !st.rowEnabled(1) {
		t.Error("load/new should always be enabled")
	}
	if st.rowEnabled(2) || st.rowEnabled(3) {
		t.Error("delete/title should be disabled without an active session")
	}
	s.tab = tabSession
	s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub != nil {
		t.Error("Enter on disabled delete row should not open a modal")
	}
}

func TestSessionTabNewClears(t *testing.T) {
	s := New(Deps{})
	s.active = &SessionInfo{ID: "x", Title: "old"}
	cleared := false
	s.SetOnSessionClear(func() { cleared = true })

	s.tab = tabSession
	s.session.index = 1
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))

	if !cleared {
		t.Error("new session should fire onSessionClear")
	}
	if s.active != nil {
		t.Error("new session should clear the active session")
	}
}

func TestSessionTabLoadAndDelete(t *testing.T) {
	mgr, _ := newSessionManager(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := mgr.Create(session.CreateOptions{Title: "Test Sess", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := mgr.AppendMessage(sess.ID, llm.Message{Role: llm.RoleUser, Content: "hello"}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	s := New(Deps{Sessions: mgr})
	var loaded SessionInfo
	var cleared int
	s.SetOnSessionChange(func(info SessionInfo) { loaded = info })
	s.SetOnSessionClear(func() { cleared++ })

	s.tab = tabSession
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub == nil {
		t.Fatal("session list should open")
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub != nil {
		t.Fatal("list should close after selection")
	}
	if loaded.ID != sess.ID || loaded.Title != "Test Sess" {
		t.Errorf("loaded = %+v", loaded)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "hello" {
		t.Errorf("loaded messages = %+v", loaded.Messages)
	}
	if s.active == nil {
		t.Fatal("active session should be set")
	}

	s.session.index = 2
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub == nil {
		t.Fatal("confirm modal should open")
	}
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.active != nil {
		t.Error("active session should clear after delete")
	}
	if cleared != 1 {
		t.Errorf("onSessionClear calls = %d, want 1", cleared)
	}
	if _, err := mgr.Get(sess.ID); err == nil {
		t.Error("session should be gone from the store")
	}
}

func TestSessionTabSaveTitle(t *testing.T) {
	mgr, _ := newSessionManager(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := mgr.Create(session.CreateOptions{Title: "Old Title", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	s := New(Deps{Sessions: mgr})
	s.SetActiveSession(&SessionInfo{ID: sess.ID, Title: "Old Title"})
	var updated SessionInfo
	s.SetOnSessionChange(func(info SessionInfo) { updated = info })

	s.tab = tabSession
	s.session.index = 3
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if s.sub == nil {
		t.Fatal("title modal should open")
	}
	if fm, ok := s.sub.(*fieldModal); ok {
		fm.field.Focus()
		fm.field.SetText("")
		for _, r := range []rune("New Title") {
			fm.field.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, 0))
		}
		fm.field.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	} else {
		t.Fatalf("sub is %T, want fieldModal", s.sub)
	}

	if s.sub != nil {
		t.Fatal("title modal should close after save")
	}
	if updated.Title != "New Title" {
		t.Errorf("updated title = %q, want New Title", updated.Title)
	}
	got, err := mgr.Get(sess.ID)
	if err != nil || got.Title != "New Title" {
		t.Errorf("persisted title = %+v, err = %v", got, err)
	}
}

func TestSessionTabNavigation(t *testing.T) {
	s := New(Deps{})
	st := s.session
	st.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	if st.index != 1 {
		t.Errorf("after Down index = %d, want 1", st.index)
	}
	for i := 0; i < 20; i++ {
		st.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	}
	if st.index != st.totalRows()-1 {
		t.Errorf("index should clamp at %d, got %d", st.totalRows()-1, st.index)
	}
	st.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, 0))
	if st.index != st.totalRows()-2 {
		t.Errorf("after Up index = %d, want %d", st.totalRows()-2, st.index)
	}
}

func TestSessionTabLoadClosesModal(t *testing.T) {
	mgr, _ := newSessionManager(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := mgr.Create(session.CreateOptions{Title: "T", Provider: "p", Model: "m", ProjectDir: dir})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	s := New(Deps{Sessions: mgr})
	closed := false
	s.SetOnClose(func() { closed = true })

	s.tab = tabSession
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if !closed {
		t.Error("loading a session should close the settings modal")
	}
	if s.active == nil || s.active.ID != sess.ID {
		t.Errorf("session should still be active: %+v", s.active)
	}
}

func TestSessionTabNewClosesModal(t *testing.T) {
	s := New(Deps{})
	s.active = &SessionInfo{ID: "x", Title: "old"}
	closed := false
	s.SetOnClose(func() { closed = true })

	s.tab = tabSession
	s.session.index = 1
	s.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if !closed {
		t.Error("new session should close the settings modal")
	}
	if s.active != nil {
		t.Error("active session should be cleared")
	}
}

func TestSessionTabCompactionCheckboxes(t *testing.T) {
	s := New(Deps{})
	st := s.session
	st.loadCompaction()

	if !st.compEnabled {
		t.Error("compaction should default to enabled")
	}
	if !st.compStrategyOn[0] || !st.compStrategyOn[1] || st.compStrategyOn[2] {
		t.Errorf("default strategies = %+v, want [true true false]", st.compStrategyOn)
	}

	st.index = compSectionStart + 1
	st.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if st.compStrategyOn[0] {
		t.Error("Right on tool-clearing should toggle it off")
	}
	st.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if !st.compStrategyOn[0] {
		t.Error("Enter on tool-clearing should toggle it back on")
	}

	for i := 0; i < compStrategyCount; i++ {
		if st.compStrategyOn[i] {
			st.index = compSectionStart + 1 + i
			st.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
		}
	}
	for i, on := range st.compStrategyOn {
		if on {
			t.Errorf("strategy %d should be off after clearing all", i)
		}
	}
}

func TestSessionTabCompactionEnabledArrows(t *testing.T) {
	s := New(Deps{})
	st := s.session
	st.loadCompaction()

	st.index = compSectionStart
	start := st.compEnabled

	st.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	if st.compEnabled == start {
		t.Error("Left on Enabled should toggle compaction")
	}
	st.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	if st.compEnabled != start {
		t.Error("Right on Enabled should toggle back")
	}
}

func TestSessionTabCompactionSaveMultiStrategy(t *testing.T) {
	s := New(Deps{Config: config.DefaultConfig()})
	st := s.session
	st.loadCompaction()

	st.index = compSectionStart + 2
	st.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))
	st.index = compSectionStart + 3
	st.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, 0))

	cfg := s.deps.Config
	if cfg == nil || cfg.Compaction == nil {
		t.Fatal("compaction config not persisted")
	}
	got := cfg.Compaction
	if got.Enabled != true {
		t.Errorf("enabled = %v, want true", got.Enabled)
	}
	want := []string{"tool-clearing", "summarization"}
	if len(got.Strategy) != len(want) {
		t.Fatalf("strategy = %v, want %v", got.Strategy, want)
	}
	for i, w := range want {
		if got.Strategy[i] != w {
			t.Errorf("strategy[%d] = %q, want %q", i, got.Strategy[i], w)
		}
	}
	if got.Threshold != float64(st.compThresh) || got.MaxMessages != st.compMaxMsg || got.MaxToolOutputChars != st.compMaxTool {
		t.Errorf("numeric settings not saved: %+v", got)
	}
}
