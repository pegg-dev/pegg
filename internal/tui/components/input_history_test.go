package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func typeInto(in *Input, text string) {
	for _, r := range text {
		in.InsertRune(r)
	}
}

func submit(in *Input, text string) {
	typeInto(in, text)
	in.Submit()
}

func pressUp(in *Input) bool {
	return in.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, 0))
}

func pressDown(in *Input) bool {
	return in.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
}

func TestUpRecallsPreviousPrompts(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "first")
	submit(in, "second")

	pressUp(in)
	if in.Value() != "second" {
		t.Fatalf("first Up = %q, want second", in.Value())
	}
	pressUp(in)
	if in.Value() != "first" {
		t.Fatalf("second Up = %q, want first", in.Value())
	}
	pressUp(in)
	if in.Value() != "first" {
		t.Fatalf("Up past oldest = %q, want first", in.Value())
	}
}

func TestDownWalksBackToDraft(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "first")
	submit(in, "second")
	typeInto(in, "draft")

	pressUp(in)
	pressUp(in)
	if in.Value() != "first" {
		t.Fatalf("after two Up = %q, want first", in.Value())
	}
	pressDown(in)
	if in.Value() != "second" {
		t.Fatalf("after Down = %q, want second", in.Value())
	}
	pressDown(in)
	if in.Value() != "draft" {
		t.Fatalf("Down should restore draft, got %q", in.Value())
	}
	if in.BrowsingHistory() {
		t.Error("draft restored, should no longer browse history")
	}
}

func TestRecalledPromptCursorAtEnd(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "hello")
	pressUp(in)
	if in.Row() != 0 || in.Col() != 5 {
		t.Errorf("cursor = (%d,%d), want (0,5)", in.Row(), in.Col())
	}
}

func TestUpMovesWithinMultilineBeforeHistory(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "earlier")
	typeInto(in, "line one")
	in.Newline()
	typeInto(in, "line two")

	pressUp(in)
	if in.Value() != "line one\nline two" {
		t.Fatalf("Up on last line should move cursor, value = %q", in.Value())
	}
	if in.Row() != 0 {
		t.Fatalf("cursor row = %d, want 0", in.Row())
	}
	pressUp(in)
	if in.Value() != "earlier" {
		t.Fatalf("Up on first line should recall history, value = %q", in.Value())
	}
}

func TestHistorySkipsBlanksAndDuplicates(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "same")
	submit(in, "same")
	in.Submit()
	if got := in.History(); len(got) != 1 || got[0] != "same" {
		t.Errorf("history = %v, want [same]", got)
	}
}

func TestSetHistorySeedsRecall(t *testing.T) {
	in := NewInput()
	in.Focus()
	in.SetHistory([]string{"old", "", "new"})
	pressUp(in)
	if in.Value() != "new" {
		t.Fatalf("Up = %q, want new", in.Value())
	}
	pressUp(in)
	if in.Value() != "old" {
		t.Fatalf("Up = %q, want old", in.Value())
	}
}

func TestSubmitResetsHistoryCursor(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "first")
	submit(in, "second")
	pressUp(in)
	in.Submit()
	if in.BrowsingHistory() {
		t.Error("submit should reset the history cursor")
	}
	pressUp(in)
	if in.Value() != "second" {
		t.Errorf("Up after resubmit = %q, want second", in.Value())
	}
}

func TestShiftUpSelectsInsteadOfRecalling(t *testing.T) {
	in := NewInput()
	in.Focus()
	submit(in, "earlier")
	typeInto(in, "keep me")
	in.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModShift))
	if in.Value() != "keep me" {
		t.Errorf("Shift+Up should not recall, value = %q", in.Value())
	}
}
