package session

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/query"
	_ "modernc.org/sqlite"
)

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := newSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteStoreCreateGetUpdateDelete(t *testing.T) {
	s := newTestSQLiteStore(t)

	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(testSession("s1")); err == nil {
		t.Fatal("want duplicate error")
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
	got.Usage = llm.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}
	if err := s.Update(*got); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Get("s1")
	if again.Title != "renamed" || again.Usage.TotalTokens != 8 {
		t.Fatalf("got %+v", again)
	}

	if err := s.Delete("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestSQLiteStoreMessagesTruncate(t *testing.T) {
	s := newTestSQLiteStore(t)
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

func TestSQLiteStoreMessagesTruncateFrom(t *testing.T) {
	s := newTestSQLiteStore(t)
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

func TestSQLiteStoreToolCallsRoundTrip(t *testing.T) {
	s := newTestSQLiteStore(t)
	if err := s.Create(testSession("s1")); err != nil {
		t.Fatal(err)
	}
	m := testMessage("s1", 1, llm.RoleAssistant, "")
	m.ToolCalls = []llm.ToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: llm.Function{
			Name:      "echo",
			Arguments: `{"msg":"x"}`,
		},
	}}
	if err := s.InsertMessage(m); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.Messages("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || len(msgs[0].ToolCalls) != 1 || msgs[0].ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestSQLiteStoreSnapshotRestore(t *testing.T) {
	s := newTestSQLiteStore(t)
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

func TestSQLiteStoreList(t *testing.T) {
	s := newTestSQLiteStore(t)
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
		got, total, err := s.List(query.Query{
			Filters: []query.Filter{{Column: "model", Operator: query.OpEqual, Value: "llama-3.3"}},
			Sort:    []query.Sort{{Column: "id", Dir: query.Asc}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 || got[0].ID != "a" {
			t.Fatalf("got = %+v, total = %d", got, total)
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
}
