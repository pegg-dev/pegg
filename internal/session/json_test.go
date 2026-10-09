package session

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
)

func newTestJSONStore(t *testing.T) *JSONStore {
	t.Helper()
	s, err := newJSONStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testSession(id string) Session {
	return Session{
		ID:         id,
		Title:      "test-" + id,
		Provider:   "groq",
		Model:      "llama-3.3",
		ProjectDir: "/proj/" + id,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

func testMessage(sessionID string, seq int, role llm.Role, content string) Message {
	return Message{
		ID:        fmt.Sprintf("%s-m%d", sessionID, seq),
		SessionID: sessionID,
		Seq:       seq,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
}

func TestJSONStoreCreateGetUpdateDelete(t *testing.T) {
	s := newTestJSONStore(t)

	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(testSession("s1")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}

	got, err := s.Get("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "s1" || got.Title != "test-s1" || got.Provider != "groq" {
		t.Fatalf("got %+v", got)
	}

	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	got.Title = "renamed"
	if err := s.Update(*got); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Get("s1")
	if again.Title != "renamed" {
		t.Fatalf("title = %q", again.Title)
	}

	if err := s.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestJSONStoreMessagesTruncate(t *testing.T) {
	s := newTestJSONStore(t)
	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := s.InsertMessage(testMessage("s1", i, llm.RoleUser, "hi")); err != nil {
			t.Fatal(err)
		}
	}

	msgs, err := s.Messages("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 || msgs[3].Seq != 4 {
		t.Fatalf("messages = %+v", msgs)
	}

	removed, err := s.TruncateAfter("s1", "s1-m2")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || removed[0].Seq != 3 {
		t.Fatalf("removed = %+v", removed)
	}
	msgs, _ = s.Messages("s1")
	if len(msgs) != 2 {
		t.Fatalf("messages after truncate = %+v", msgs)
	}

	if _, err := s.TruncateAfter("s1", "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
}

func TestJSONStoreMessagesTruncateFrom(t *testing.T) {
	s := newTestJSONStore(t)
	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := s.InsertMessage(testMessage("s1", i, llm.RoleUser, "hi")); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := s.TruncateFrom("s1", "s1-m2")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 3 || removed[0].Seq != 2 {
		t.Fatalf("removed = %+v", removed)
	}
	msgs, _ := s.Messages("s1")
	if len(msgs) != 1 || msgs[0].Seq != 1 {
		t.Fatalf("messages after truncate-from = %+v", msgs)
	}

	if _, err := s.TruncateFrom("s1", "nope"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("want ErrMessageNotFound, got %v", err)
	}
}

func TestJSONStoreSnapshotRestore(t *testing.T) {
	s := newTestJSONStore(t)
	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		_ = s.InsertMessage(testMessage("s1", i, llm.RoleUser, "hi"))
	}

	if _, err := s.TruncateAfter("s1", "s1-m1"); err != nil {
		t.Fatal(err)
	}

	snap := Snapshot{
		ID:            "snap1",
		SessionID:     "s1",
		HeadMessageID: "s1-m1",
		CreatedAt:     time.Now(),
		Messages:      []Message{testMessage("s1", 2, llm.RoleAssistant, "a"), testMessage("s1", 3, llm.RoleUser, "b")},
	}
	if err := s.SaveSnapshot(snap); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSnapshot("snap1")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "s1" || len(got.Messages) != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
	if _, err := s.GetSnapshot("nope"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("want ErrSnapshotNotFound, got %v", err)
	}

	all, err := s.Snapshots("s1")
	if err != nil || len(all) != 1 {
		t.Fatalf("snapshots = %+v, err = %v", all, err)
	}

	if err := s.RestoreSnapshot("s1", snap.Messages); err != nil {
		t.Fatal(err)
	}
	msgs, _ := s.Messages("s1")
	if len(msgs) != 3 || msgs[1].Seq != 2 || msgs[2].Seq != 3 {
		t.Fatalf("restored messages = %+v", msgs)
	}
}

func TestJSONStoreList(t *testing.T) {
	s := newTestJSONStore(t)
	created := time.Now().Add(-time.Hour)
	for i, id := range []string{"a", "b", "c", "d"} {
		sess := testSession(id)
		sess.CreatedAt = created.Add(time.Duration(i+1) * time.Minute)
		if err := s.Create(sess); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("default sort desc", func(t *testing.T) {
		all, total, err := s.List(query.Query{})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 || len(all) != 4 || all[0].ID != "d" {
			t.Fatalf("all = %+v, total = %d", all, total)
		}
	})

	t.Run("search", func(t *testing.T) {
		got, total, err := s.List(query.Query{
			Search:        "test-b",
			SearchColumns: []string{"title"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || got[0].ID != "b" {
			t.Fatalf("got = %+v, total = %d", got, total)
		}
	})

	t.Run("filter", func(t *testing.T) {
		_, total, err := s.List(query.Query{
			Filters: []query.Filter{{Column: "model", Operator: query.OpEqual, Value: "llama-3.3"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 {
			t.Fatalf("total = %d", total)
		}
	})

	t.Run("page", func(t *testing.T) {
		got, total, err := s.List(query.Query{Page: query.Page{Number: 2, Size: 2}})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 || len(got) != 2 || got[0].ID != "b" {
			t.Fatalf("got = %+v, total = %d", got, total)
		}
	})

	t.Run("bad column", func(t *testing.T) {
		if _, _, err := s.List(query.Query{
			Filters: []query.Filter{{Column: "nope", Operator: query.OpEqual, Value: "x"}},
		}); err == nil {
			t.Fatal("want error for unknown column")
		}
	})
}
