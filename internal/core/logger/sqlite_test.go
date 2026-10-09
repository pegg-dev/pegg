package logger

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/utils/query"
	_ "modernc.org/sqlite"
)

func newTestSQLiteHandler(t *testing.T) *SQLiteHandler {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		level TEXT,
		message TEXT
	)`); err != nil {
		t.Fatal(err)
	}

	h, err := newSQLiteHandler(db, 100)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSQLiteHandlerQuery(t *testing.T) {
	h := newTestSQLiteHandler(t)

	for i, m := range []struct {
		level Level
		msg   string
	}{
		{LevelInfo, "server started"},
		{LevelError, "connection timeout"},
		{LevelWarn, "slow query detected"},
		{LevelError, "timeout on retry"},
	} {
		h.Write(Record{Timestamp: time.Now().Add(time.Duration(i) * time.Second), Level: m.level, Message: m.msg})
	}

	t.Run("filter by level", func(t *testing.T) {
		recs, total, err := h.Query(query.Query{
			Filters: []query.Filter{{Column: "level", Operator: query.OpEqual, Value: "ERROR"}},
			Sort:    []query.Sort{{Column: "id", Dir: query.Asc}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 {
			t.Errorf("total: got %d, want 2", total)
		}
		if len(recs) != 2 {
			t.Fatalf("records: got %d, want 2", len(recs))
		}
		for _, r := range recs {
			if r.Level != LevelError {
				t.Errorf("level: got %v, want ERROR", r.Level)
			}
		}
	})

	t.Run("defaults to newest timestamp first", func(t *testing.T) {
		recs, total, err := h.Query(query.Query{})
		if err != nil {
			t.Fatal(err)
		}
		if total != 4 {
			t.Errorf("total: got %d, want 4", total)
		}
		if len(recs) != 4 {
			t.Fatalf("records: got %d, want 4", len(recs))
		}
		if recs[0].Message != "timeout on retry" {
			t.Errorf("first record message: got %q, want %q", recs[0].Message, "timeout on retry")
		}
	})

	t.Run("search and paginate", func(t *testing.T) {
		recs, total, err := h.Query(query.Query{
			Search:        "timeout",
			SearchColumns: []string{"message"},
			Sort:          []query.Sort{{Column: "id", Dir: query.Desc}},
			Page:          query.Page{Number: 1, Size: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 {
			t.Errorf("total: got %d, want 2", total)
		}
		if len(recs) != 1 {
			t.Fatalf("records: got %d, want 1", len(recs))
		}
		if recs[0].Message != "timeout on retry" {
			t.Errorf("first page message: got %q, want %q", recs[0].Message, "timeout on retry")
		}
	})
}

func TestSQLiteHandlerClose(t *testing.T) {
	h := newTestSQLiteHandler(t)

	if err := h.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := h.Write(Record{Timestamp: time.Now(), Level: LevelInfo, Message: "after close"}); err == nil {
		t.Fatal("expected error writing after close")
	}
}

func TestSQLiteHandlerQueryRejectsUnknownColumn(t *testing.T) {
	h := newTestSQLiteHandler(t)

	_, _, err := h.Query(query.Query{
		Filters: []query.Filter{{Column: "payload", Operator: query.OpEqual, Value: "x"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown column")
	}
}

func TestSQLiteHandlerConcurrentReadWrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lcfg := config.LoggerConfig{Driver: "sqlite", MaxLogCount: 100}

	writer, err := NewSQLiteHandler(lcfg)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = writer.Write(Record{Timestamp: time.Now(), Level: LevelInfo, Message: fmt.Sprintf("msg %d", i)})
		}
	}()

	reader, err := NewSQLiteHandler(lcfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	for i := 0; i < 100; i++ {
		if _, _, err := reader.Query(query.Query{}); err != nil {
			t.Fatalf("concurrent query failed (iteration %d): %v", i, err)
		}
	}

	<-done
}
