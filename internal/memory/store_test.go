package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func todayFile() string { return timeNow().Format("2006-01-02") + ".md" }

func TestAppendEntriesAndDedup(t *testing.T) {
	s := newTestStore(t)

	blocks := []string{
		"## obs-aaaaaa [bugfix] Fixed auth — 2026-10-02T10:00:00Z\n- hash: aaaa\nnarrative one",
		"## obs-bbbbbb [discovery] Learned API — 2026-10-02T10:01:00Z\n- hash: bbbb\nnarrative two",
	}
	entries := []IndexEntry{
		{ID: "obs-aaaaaa", Date: "2026-10-02T10:00", Type: "bugfix", Title: "Fixed auth", Keywords: "auth", File: todayFile()},
		{ID: "obs-bbbbbb", Date: "2026-10-02T10:01", Type: "discovery", Title: "Learned API", Keywords: "api", File: todayFile()},
	}

	n, err := s.AppendEntries(blocks, entries)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("appended %d, want 2", n)
	}

	n, err = s.AppendEntries(blocks, entries)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("dedup appended %d, want 0", n)
	}

	idx, err := s.ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 2 {
		t.Fatalf("index has %d entries, want 2", len(idx))
	}
	if idx[1].ID != "obs-bbbbbb" || idx[1].Title != "Learned API" {
		t.Fatalf("unexpected index entry: %+v", idx[1])
	}
}

func TestReadBlock(t *testing.T) {
	s := newTestStore(t)
	blocks := []string{
		"## obs-aaaaaa [bugfix] Fixed auth — 2026-10-02T10:00:00Z\n- hash: aaaa\nnarrative one",
		"## obs-bbbbbb [discovery] Learned API — 2026-10-02T10:01:00Z\n- hash: bbbb\nnarrative two",
	}
	entries := []IndexEntry{
		{ID: "obs-aaaaaa", Date: "d", Type: "bugfix", Title: "A", File: todayFile()},
		{ID: "obs-bbbbbb", Date: "d", Type: "discovery", Title: "B", File: todayFile()},
	}
	if _, err := s.AppendEntries(blocks, entries); err != nil {
		t.Fatal(err)
	}

	block, ok := s.ReadBlock(todayFile(), "obs-bbbbbb")
	if !ok || !strings.Contains(block, "narrative two") {
		t.Fatalf("block = %q, ok = %v", block, ok)
	}
	if _, ok := s.ReadBlock(todayFile(), "obs-zzzz"); ok {
		t.Fatal("expected missing block")
	}
}

func TestSearchFilesScoring(t *testing.T) {
	s := newTestStore(t)
	blocks := []string{
		"## obs-aaaaaa [bugfix] Fixed auth token refresh — d\n- keywords: auth, token\n- hash: aaaa\nthe refresh endpoint returned 401",
		"## obs-bbbbbb [feature] Added cache eviction — d\n- keywords: cache\n- hash: bbbb\neviction policy for the response cache",
	}
	entries := []IndexEntry{
		{ID: "obs-aaaaaa", Date: "d", Type: "bugfix", Title: "Fixed auth token refresh", Keywords: "auth, token", File: todayFile()},
		{ID: "obs-bbbbbb", Date: "d", Type: "feature", Title: "Added cache eviction", Keywords: "cache", File: todayFile()},
	}
	if _, err := s.AppendEntries(blocks, entries); err != nil {
		t.Fatal(err)
	}

	hits := s.SearchFiles("auth token refresh", 5)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].ID != "obs-aaaaaa" {
		t.Fatalf("top hit = %s, want obs-aaaaaa", hits[0].ID)
	}

	hits = s.SearchFiles("unrelated query", 5)
	if len(hits) != 0 {
		t.Fatalf("hits = %d, want 0", len(hits))
	}
}

func TestCuratedAndGlobalLessons(t *testing.T) {
	s := newTestStore(t)
	if err := s.WriteCurated(decisionsFile, "## Decisions\n- use WAL"); err != nil {
		t.Fatal(err)
	}
	if got := s.ReadCurated(decisionsFile); !strings.Contains(got, "use WAL") {
		t.Fatalf("curated = %q", got)
	}
	if err := s.AppendGlobalLessons("- [global] lesson one"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendGlobalLessons("- [global] lesson one"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.globalLessonsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "lesson one") != 1 {
		t.Fatalf("global lessons dedup failed: %q", string(data))
	}
}

func TestStatsAndClear(t *testing.T) {
	s := newTestStore(t)
	blocks := []string{"## obs-aaaaaa [bugfix] A — d\n- hash: aaaa\nx"}
	entries := []IndexEntry{{ID: "obs-aaaaaa", Date: "2026-10-02T10:00", Type: "bugfix", Title: "A", File: todayFile()}}
	if _, err := s.AppendEntries(blocks, entries); err != nil {
		t.Fatal(err)
	}
	count, lastDate, _ := s.Stats()
	if count != 1 || lastDate != "2026-10-02T10:00" {
		t.Fatalf("stats = %d/%s", count, lastDate)
	}
	if !s.HasMemory() {
		t.Fatal("expected memory present")
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.root(), indexFile)); !os.IsNotExist(err) {
		t.Fatalf("index file should be gone, err = %v", err)
	}
	count, _, _ = s.Stats()
	if count != 0 {
		t.Fatalf("stats after clear = %d, want 0", count)
	}
}

func TestIndexRowRoundtrip(t *testing.T) {
	e := IndexEntry{
		ID: "obs-9f3a1b", Date: "2026-10-02T10:30", Type: "bugfix",
		Title: "Fixed auth | token", Keywords: "auth, token", File: todayFile(),
	}
	row := e.row()
	parsed, ok := parseIndexRow(row)
	if !ok {
		t.Fatalf("parse failed for %q", row)
	}
	if parsed.Title != "Fixed auth - token" {
		t.Fatalf("title = %q (pipe should be sanitized)", parsed.Title)
	}
	if parsed.ID != e.ID || parsed.File != e.File {
		t.Fatalf("parsed = %+v", parsed)
	}
}
