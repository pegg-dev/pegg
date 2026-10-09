package session

import (
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func newTestManager(t *testing.T) (*Manager, event.Bus) {
	t.Helper()
	store, err := newJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	bus := event.New()
	mgr := NewManager(store, bus, logger.New(logger.LevelDebug, discardHandler{}))
	return mgr, bus
}

func newSessionWithMessages(t *testing.T, mgr *Manager, title string, n int) (*Session, []string) {
	t.Helper()
	s, err := mgr.Create(CreateOptions{Title: title, Provider: "groq", Model: "m1", ProjectDir: "/proj"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < n; i++ {
		m, err := mgr.AppendMessage(s.ID, llm.UserMessage("msg"))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	return s, ids
}

func TestManagerCreateEvents(t *testing.T) {
	mgr, bus := newTestManager(t)
	var created *SessionCreated
	_ = bus.Subscribe(TopicSessionCreated, func(e SessionCreated) { created = &e })

	s, err := mgr.Create(CreateOptions{Title: "t", Provider: "p", Model: "m", ProjectDir: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" || s.ParentID != "" {
		t.Fatalf("session = %+v", s)
	}
	if created == nil || created.SessionID != s.ID {
		t.Fatalf("created event = %+v", created)
	}
	s2, err := mgr.Create(CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s2.Title == "" {
		t.Fatalf("session with empty title should get default title, got empty")
	}
}

func TestManagerAppendMessageSeqAndUsage(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, _ := newSessionWithMessages(t, mgr, "t", 3)
	var added *SessionMessageAdded
	_ = bus.Subscribe(TopicSessionMessageAdded, func(e SessionMessageAdded) { added = &e })

	m, err := mgr.AppendMessage(s.ID, llm.AssistantMessage("answer"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Seq != 4 || m.Role != llm.RoleAssistant {
		t.Fatalf("message = %+v", m)
	}
	if added == nil || added.Message.ID != m.ID || added.SessionID != s.ID {
		t.Fatalf("added event = %+v", added)
	}

	if err := mgr.AccumulateUsage(s.ID, llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(s.ID)
	if got.Usage.TotalTokens != 15 || got.Usage.Cost != 0 {
		t.Fatalf("usage = %+v", got.Usage)
	}

	if _, err := mgr.AppendMessage("nope", llm.UserMessage("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestManagerFork(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, ids := newSessionWithMessages(t, mgr, "orig", 4)
	var forked *SessionForked
	_ = bus.Subscribe(TopicSessionForked, func(e SessionForked) { forked = &e })

	fork, err := mgr.Fork(s.ID, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if fork.ID == s.ID || fork.ParentID != s.ID {
		t.Fatalf("fork = %+v", fork)
	}
	if fork.Title != s.Title+" - forked" || fork.Provider != s.Provider || fork.Model != s.Model || fork.ProjectDir != s.ProjectDir {
		t.Fatalf("fork meta = %+v", fork)
	}
	msgs, _ := mgr.Messages(fork.ID)
	if len(msgs) != 2 || msgs[0].Seq != 1 || msgs[1].Seq != 2 {
		t.Fatalf("fork messages = %+v", msgs)
	}
	if msgs[0].ID == ids[0] {
		t.Fatal("fork must use fresh message ids")
	}
	if forked == nil || forked.ParentID != s.ID || forked.ParentMessageID != ids[1] {
		t.Fatalf("forked event = %+v", forked)
	}

	if _, err := mgr.Fork(s.ID, "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
}

func TestManagerRevertAndUndo(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, ids := newSessionWithMessages(t, mgr, "orig", 4)
	var reverted *SessionReverted
	_ = bus.Subscribe(TopicSessionReverted, func(e SessionReverted) { reverted = &e })
	var restored *SessionRestored
	_ = bus.Subscribe(TopicSessionRestored, func(e SessionRestored) { restored = &e })

	snapID, err := mgr.Revert(s.ID, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if snapID == "" {
		t.Fatal("want snapshot id")
	}
	msgs, _ := mgr.Messages(s.ID)
	if len(msgs) != 2 {
		t.Fatalf("messages after revert = %+v", msgs)
	}
	if reverted == nil || reverted.ToMessageID != ids[1] || reverted.SnapshotID != snapID {
		t.Fatalf("reverted event = %+v", reverted)
	}

	snaps, err := mgr.Snapshots(s.ID)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots = %+v, err = %v", snaps, err)
	}

	if err := mgr.UndoRevert(s.ID, snapID); err != nil {
		t.Fatal(err)
	}
	msgs, _ = mgr.Messages(s.ID)
	if len(msgs) != 4 || msgs[3].Seq != 4 {
		t.Fatalf("messages after undo = %+v", msgs)
	}
	if restored == nil || restored.SnapshotID != snapID {
		t.Fatalf("restored event = %+v", restored)
	}

	if _, err := mgr.Revert(s.ID, "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
	if err := mgr.UndoRevert(s.ID, "nope"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("want ErrSnapshotNotFound, got %v", err)
	}
	if err := mgr.UndoRevert("other", snapID); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("want ErrSnapshotNotFound for mismatched session, got %v", err)
	}
}

func TestManagerRevertNoopAtTip(t *testing.T) {
	mgr, _ := newTestManager(t)
	s, ids := newSessionWithMessages(t, mgr, "orig", 3)
	snapID, err := mgr.Revert(s.ID, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if snapID != "" {
		t.Fatalf("want empty snapshot id, got %q", snapID)
	}
	snaps, _ := mgr.Snapshots(s.ID)
	if len(snaps) != 0 {
		t.Fatalf("snapshots = %+v", snaps)
	}
}

func TestManagerForkBefore(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, ids := newSessionWithMessages(t, mgr, "orig", 4)
	var forked *SessionForked
	_ = bus.Subscribe(TopicSessionForked, func(e SessionForked) { forked = &e })

	fork, err := mgr.ForkBefore(s.ID, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if fork.ID == s.ID || fork.ParentID != s.ID || fork.Title != s.Title+" - fork" {
		t.Fatalf("fork = %+v", fork)
	}
	msgs, _ := mgr.Messages(fork.ID)
	if len(msgs) != 1 || msgs[0].Seq != 1 {
		t.Fatalf("fork messages = %+v, want only the first message", msgs)
	}
	if msgs[0].ID == ids[0] {
		t.Fatal("fork must use fresh message ids")
	}
	if forked == nil || forked.ParentID != s.ID || forked.ParentMessageID != ids[1] {
		t.Fatalf("forked event = %+v", forked)
	}

	empty, err := mgr.ForkBefore(s.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if msgs, _ := mgr.Messages(empty.ID); len(msgs) != 0 {
		t.Fatalf("fork before first message should be empty, got %+v", msgs)
	}

	if _, err := mgr.ForkBefore(s.ID, "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
}

func TestManagerRevertFrom(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, ids := newSessionWithMessages(t, mgr, "orig", 4)
	var reverted *SessionReverted
	_ = bus.Subscribe(TopicSessionReverted, func(e SessionReverted) { reverted = &e })

	snapID, removed, err := mgr.RevertFrom(s.ID, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if snapID == "" || len(removed) != 3 || removed[0].Seq != 2 {
		t.Fatalf("snapID = %q, removed = %+v", snapID, removed)
	}
	msgs, _ := mgr.Messages(s.ID)
	if len(msgs) != 1 || msgs[0].ID != ids[0] {
		t.Fatalf("messages after revert-from = %+v", msgs)
	}
	if reverted == nil || reverted.ToMessageID != ids[1] || reverted.SnapshotID != snapID {
		t.Fatalf("reverted event = %+v", reverted)
	}

	if err := mgr.UndoRevert(s.ID, snapID); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := mgr.Messages(s.ID); len(msgs) != 4 {
		t.Fatalf("messages after undo = %+v", msgs)
	}

	if _, _, err := mgr.RevertFrom(s.ID, "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
}

func TestManagerCurrentSession(t *testing.T) {
	mgr, bus := newTestManager(t)
	s, _ := newSessionWithMessages(t, mgr, "t", 1)
	var changed *SessionCurrentChanged
	_ = bus.Subscribe(TopicSessionCurrentChanged, func(e SessionCurrentChanged) { changed = &e })

	if _, ok := mgr.Current(); ok {
		t.Fatal("current should be unset initially")
	}
	if err := mgr.SetCurrent(s.ID); err != nil {
		t.Fatal(err)
	}
	id, ok := mgr.Current()
	if !ok || id != s.ID {
		t.Fatalf("current = %q, %v", id, ok)
	}
	if changed == nil || changed.SessionID != s.ID {
		t.Fatalf("changed event = %+v", changed)
	}

	if err := mgr.SetCurrent("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := mgr.SetCurrent(""); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.Current(); ok {
		t.Fatal("current should be cleared")
	}
}

func TestManagerDeleteClearsCurrent(t *testing.T) {
	mgr, _ := newTestManager(t)
	s, _ := newSessionWithMessages(t, mgr, "t", 1)
	if err := mgr.SetCurrent(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.Current(); ok {
		t.Fatal("current should be cleared after delete")
	}
	if err := mgr.Delete(s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestManagerSetTitle(t *testing.T) {
	mgr, _ := newTestManager(t)
	s, _ := newSessionWithMessages(t, mgr, "old", 1)
	if err := mgr.SetTitle(s.ID, "new"); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(s.ID)
	if got.Title != "new" {
		t.Fatalf("title = %q", got.Title)
	}
	if err := mgr.SetTitle(s.ID, ""); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
}
