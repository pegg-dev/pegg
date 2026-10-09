package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/event"
)

func TestParseBlockHeader(t *testing.T) {
	id, typ, title, date := parseBlockHeader("## obs-9f3a1b [bugfix] Fixed auth token refresh — 2026-10-02T10:00")
	if id != "obs-9f3a1b" || typ != "bugfix" || title != "Fixed auth token refresh" || date != "2026-10-02T10:00" {
		t.Fatalf("parsed = %q %q %q %q", id, typ, title, date)
	}
	id, typ, title, date = parseBlockHeader("## sum-2c4d5e [summary] Session summary — 2026-10-02T11:00")
	if id != "sum-2c4d5e" || typ != "summary" || title != "Session summary" {
		t.Fatalf("parsed = %q %q %q", id, typ, title)
	}
}

func TestSearchXMLOutput(t *testing.T) {
	store := newTestStore(t)
	blocks := []string{
		"## obs-aaaaaa [bugfix] Fixed auth token refresh — 2026-10-02T10:00\n- files: internal/auth/token.go\n- keywords: auth, token\n- hash: aaaa\nthe refresh endpoint returned 401 on expired tokens; retry once.",
	}
	entries := []IndexEntry{
		{ID: "obs-aaaaaa", Date: "2026-10-02T10:00", Type: "bugfix", Title: "Fixed auth token refresh", Keywords: "auth, token", File: todayFile()},
	}
	if _, err := store.AppendEntries(blocks, entries); err != nil {
		t.Fatal(err)
	}

	m := newManager(Deps{Config: defaultCfg(), Bus: event.New()}, t.TempDir())
	m.store = store

	out, err := m.search(context.Background(), `{"query":"auth token refresh"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<memory_results query="auth token refresh" count="1">`,
		`<result>`,
		`<id>obs-aaaaaa</id>`,
		`<type>bugfix</type>`,
		`<title>Fixed auth token refresh</title>`,
		`<date>2026-10-02T10:00</date>`,
		`<file>` + todayFile() + `</file>`,
		`<snippet>`,
		`</memory_results>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	if out2, _ := m.search(context.Background(), `{"query":"unrelated xyz"}`); !strings.Contains(out2, "No results found") {
		t.Fatalf("empty output = %q", out2)
	}
}

func TestSearchXMLEscapes(t *testing.T) {
	out := xmlEscape(`a < b & "c" 'd' > e`)
	want := "a &lt; b &amp; &quot;c&quot; &apos;d&apos; &gt; e"
	if out != want {
		t.Fatalf("escaped = %q, want %q", out, want)
	}
}
