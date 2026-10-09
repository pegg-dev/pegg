package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
)

func askKey(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, 0) }

func TestAskPickerReasonFlow(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	p.SetOnSend(func(answers map[string]string) {
		got = answers
	})
	p.SetQuestions([]AskQuestion{
		{ID: "decision", Question: "Allow?", Type: "select", Options: []string{"Allow", "Allow All", "Reject"}, Required: true},
	})

	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after the only question")
	}
	if got == nil || got["decision"] != "Allow" {
		t.Fatalf("answers = %v, want decision=Allow", got)
	}
}

func TestAskPickerSingleTextReason(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	p.SetOnSend(func(answers map[string]string) {
		got = answers
	})
	p.SetQuestions([]AskQuestion{
		{ID: "reason", Question: "Reason?", Type: "text"},
	})

	st := p.states[p.tab]
	if st.field == nil || !st.field.focused {
		t.Fatal("text field should exist and be focused")
	}

	for _, r := range "not today" {
		if !p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) {
			t.Fatalf("rune %q not handled", r)
		}
	}
	if v := st.field.Value(); v != "not today" {
		t.Fatalf("field value = %q, want %q", v, "not today")
	}

	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after Enter")
	}
	if got == nil || got["reason"] != "not today" {
		t.Fatalf("answers = %v, want reason=not today", got)
	}
}

func TestAskPickerTypedTextVisible(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 30)

	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "reason", Question: "Reason?", Type: "text"},
	})
	for _, r := range "not today" {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}

	h := p.Height()
	bounds := layout.Region{Left: 10, Top: 2, Width: 80, Height: h}
	p.Draw(s, bounds)

	var lines []string
	for y := 0; y < 30; y++ {
		var b strings.Builder
		for x := 0; x < 100; x++ {
			main, _, _, _ := s.GetContent(x, y)
			b.WriteRune(main)
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	found := false
	for _, line := range lines {
		if strings.Contains(line, "not today") {
			found = true
		}
	}
	if !found {
		t.Fatalf("typed text not visible at height %d:\n%s", h, strings.Join(lines, "\n"))
	}
}

func TestAskPickerUnknownTypeFallsBackToText(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	sent := false
	p.SetOnSend(func(answers map[string]string) {
		got = answers
		sent = true
	})
	p.SetQuestions([]AskQuestion{
		{ID: "q1", Question: "Which?", Type: "boolean"},
	})
	if !p.Active() {
		t.Fatal("picker should activate for an unknown type")
	}
	if p.states[0].field == nil {
		t.Fatal("unknown type should fall back to a text field")
	}

	for _, r := range "yes" {
		p.HandleKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after Enter")
	}
	if !sent || got["q1"] != "yes" {
		t.Fatalf("answers = %v, want q1=yes", got)
	}
}

func TestAskPickerEmptyQuestionsDeactivates(t *testing.T) {
	p := NewAskPicker()
	done := false
	p.SetOnDone(func() { done = true })
	p.SetQuestions(nil)
	if p.Active() {
		t.Fatal("picker must not activate without questions")
	}
	if !done {
		t.Fatal("onDone should be called for empty questions")
	}
}

func TestAskPickerEscCancels(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	sent := false
	p.SetOnSend(func(answers map[string]string) {
		got = answers
		sent = true
	})
	p.SetQuestions([]AskQuestion{
		{ID: "q1", Question: "Which?", Type: "boolean"},
	})
	if !p.HandleKey(askKey(tcell.KeyEsc)) {
		t.Fatal("Esc should be handled")
	}
	if p.Active() {
		t.Fatal("Esc should cancel the picker")
	}
	if !sent || got != nil {
		t.Fatalf("cancel should send nil answers, got sent=%v answers=%v", sent, got)
	}
}
