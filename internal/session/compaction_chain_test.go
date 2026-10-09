package session

import (
	"testing"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
)

func TestManagerCompactSessionCreatesLinkedSession(t *testing.T) {
	mgr, bus := newTestManager(t)
	src, _ := newSessionWithMessages(t, mgr, "orig", 3)

	var compacted *SessionCompacted
	_ = bus.Subscribe(TopicSessionCompacted, func(e SessionCompacted) { compacted = &e })

	msgs := []llm.Message{
		llm.SystemMessage("sys"),
		llm.SystemMessage("[Context compacted: older messages removed]"),
		llm.UserMessage("keep this"),
	}
	child, err := mgr.CompactSession(src.ID, msgs, "sliding-window")
	if err != nil {
		t.Fatal(err)
	}
	if child.ID == src.ID {
		t.Fatal("compacted session must have a new ID")
	}
	if child.CompactionParentID != src.ID {
		t.Fatalf("compaction parent = %q, want %q", child.CompactionParentID, src.ID)
	}
	if child.Title != src.Title+" (compacted)" {
		t.Fatalf("title = %q", child.Title)
	}
	if compacted == nil || compacted.SessionID != child.ID || compacted.ParentSessionID != src.ID {
		t.Fatalf("event = %+v", compacted)
	}

	got, err := mgr.Messages(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("messages = %d, want 3", len(got))
	}
	for i, m := range got {
		if m.Seq != i+1 || m.SessionID != child.ID {
			t.Fatalf("message %d = %+v", i, m)
		}
	}

	orig, err := mgr.Messages(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orig) != 3 {
		t.Fatalf("original messages = %d, want 3", len(orig))
	}
}

func TestManagerCompactionChain(t *testing.T) {
	mgr, _ := newTestManager(t)
	src, _ := newSessionWithMessages(t, mgr, "orig", 2)

	child1, err := mgr.CompactSession(src.ID, []llm.Message{llm.SystemMessage("sum1")}, "summarization")
	if err != nil {
		t.Fatal(err)
	}
	child2, err := mgr.CompactSession(child1.ID, []llm.Message{llm.SystemMessage("sum2")}, "summarization")
	if err != nil {
		t.Fatal(err)
	}

	children, err := mgr.CompactionChildren(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != child1.ID {
		t.Fatalf("children = %+v", children)
	}

	latest, err := mgr.LatestInChain(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != child2.ID {
		t.Fatalf("latest = %q, want %q", latest.ID, child2.ID)
	}

	latest, err = mgr.LatestInChain(child2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != child2.ID {
		t.Fatalf("latest of head = %q, want %q", latest.ID, child2.ID)
	}

	alone, _ := newSessionWithMessages(t, mgr, "alone", 1)
	latest, err = mgr.LatestInChain(alone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != alone.ID {
		t.Fatalf("latest = %q, want %q", latest.ID, alone.ID)
	}
}

func TestSQLiteStoreCompactionChain(t *testing.T) {
	s := newTestSQLiteStore(t)
	src := testSession("s1")
	if err := s.Create(src); err != nil {
		t.Fatal(err)
	}
	c1 := src
	c1.ID = "c1"
	c1.CompactionParentID = "s1"
	c1.CreatedAt = src.CreatedAt.Add(1)
	if err := s.Create(c1); err != nil {
		t.Fatal(err)
	}
	c2 := c1
	c2.ID = "c2"
	c2.CompactionParentID = "c1"
	c2.CreatedAt = src.CreatedAt.Add(2)
	if err := s.Create(c2); err != nil {
		t.Fatal(err)
	}

	children, err := s.CompactionChildren("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != "c1" {
		t.Fatalf("children = %+v", children)
	}

	latest, err := s.LatestInChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "c2" {
		t.Fatalf("latest = %q, want c2", latest.ID)
	}

	if _, err := s.LatestInChain("nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestJSONStoreCompactionChain(t *testing.T) {
	s, err := newJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	src := testSession("s1")
	if err := s.Create(src); err != nil {
		t.Fatal(err)
	}
	c1 := src
	c1.ID = "c1"
	c1.CompactionParentID = "s1"
	c1.CreatedAt = src.CreatedAt.Add(1)
	if err := s.Create(c1); err != nil {
		t.Fatal(err)
	}
	c2 := c1
	c2.ID = "c2"
	c2.CompactionParentID = "c1"
	c2.CreatedAt = src.CreatedAt.Add(2)
	if err := s.Create(c2); err != nil {
		t.Fatal(err)
	}

	children, err := s.CompactionChildren("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != "c1" {
		t.Fatalf("children = %+v", children)
	}

	latest, err := s.LatestInChain("s1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "c2" {
		t.Fatalf("latest = %q, want c2", latest.ID)
	}
}

func TestManagerDeleteCascadesCompactionChain(t *testing.T) {
	mgr, _ := newTestManager(t)
	src, _ := newSessionWithMessages(t, mgr, "orig", 2)
	child1, err := mgr.CompactSession(src.ID, []llm.Message{llm.SystemMessage("sum1")}, "summarization")
	if err != nil {
		t.Fatal(err)
	}
	child2, err := mgr.CompactSession(child1.ID, []llm.Message{llm.SystemMessage("sum2")}, "summarization")
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Delete(src.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{src.ID, child1.ID, child2.ID} {
		if _, err := mgr.Get(id); err != ErrNotFound {
			t.Fatalf("session %s still exists after cascade delete (err=%v)", id, err)
		}
	}

	child1s, _ := newSessionWithMessages(t, mgr, "fork-src", 2)
	fork, err := mgr.Fork(child1s.ID, mustFirstMessageID(t, mgr, child1s.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Delete(child1s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Get(fork.ID); err != nil {
		t.Fatalf("fork should survive original delete: %v", err)
	}
}

func TestListFiltersOutCompactionChildren(t *testing.T) {
	s, err := newJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	src := testSession("s1")
	if err := s.Create(src); err != nil {
		t.Fatal(err)
	}
	child := src
	child.ID = "c1"
	child.CompactionParentID = "s1"
	if err := s.Create(child); err != nil {
		t.Fatal(err)
	}

	sessions, total, err := s.List(query.Query{
		Page: query.Page{Number: 1, Size: 50},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("list = %+v (total %d), want only s1", sessions, total)
	}
}

func mustFirstMessageID(t *testing.T, mgr *Manager, sessionID string) string {
	t.Helper()
	msgs, err := mgr.Messages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("no messages")
	}
	return msgs[0].ID
}
