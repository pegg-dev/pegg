package components

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
)

func askKey(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, 0) }

func askRune(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }

func askScreenText(s tcell.Screen, w, h int) string {
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			main, _, _, _ := s.GetContent(x, y)
			b.WriteRune(main)
		}
		b.WriteRune('\n')
	}
	return b.String()
}

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
	if !p.Active() || !p.confirming {
		t.Fatal("Enter should open the confirm page")
	}
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after confirming")
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
	if st.input == nil || !st.input.focused {
		t.Fatal("text input should exist and be focused")
	}

	for _, r := range "not today" {
		if !p.HandleKey(askRune(r)) {
			t.Fatalf("rune %q not handled", r)
		}
	}
	if v := st.input.Value(); v != "not today" {
		t.Fatalf("value = %q, want %q", v, "not today")
	}

	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after confirming")
	}
	if got == nil || got["reason"] != "not today" {
		t.Fatalf("answers = %v, want reason=not today", got)
	}
}

func TestAskPickerTextCaretMoves(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "a", Question: "First?", Type: "text"},
		{ID: "b", Question: "Second?", Type: "text"},
	})
	for _, r := range "abc" {
		p.HandleKey(askRune(r))
	}
	p.HandleKey(askKey(tcell.KeyLeft))
	p.HandleKey(askKey(tcell.KeyLeft))
	p.HandleKey(askRune('X'))

	if got := p.states[0].input.Value(); got != "aXbc" {
		t.Fatalf("value = %q, want %q (← must move the caret, not switch questions)", got, "aXbc")
	}
	if p.tab != 0 {
		t.Fatalf("tab = %d, want 0 (← must not switch questions in a text field)", p.tab)
	}
}

func TestAskPickerTabSwitchesQuestions(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "a", Question: "First?", Type: "text"},
		{ID: "b", Question: "Second?", Type: "text"},
	})
	p.HandleKey(askKey(tcell.KeyTab))
	if p.tab != 1 {
		t.Fatalf("tab = %d, want 1 after Tab", p.tab)
	}
	p.HandleKey(askKey(tcell.KeyBacktab))
	if p.tab != 0 {
		t.Fatalf("tab = %d, want 0 after Shift+Tab", p.tab)
	}
}

func TestAskPickerInsertPaste(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{{ID: "a", Question: "Body?", Type: "text"}})
	p.InsertPaste("hello\nworld")
	if got := p.states[0].input.Value(); got != "hello\nworld" {
		t.Fatalf("value = %q, want %q", got, "hello\nworld")
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
		p.HandleKey(askRune(r))
	}

	bounds := layout.Region{Left: 10, Top: 2, Width: 80, Height: p.Height()}
	p.Draw(s, bounds)

	if all := askScreenText(s, 100, 30); !strings.Contains(all, "not today") {
		t.Fatalf("typed text not visible:\n%s", all)
	}
}

func TestAskPickerLongOptionWraps(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(60, 30)

	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "q", Question: "Pick", Type: "select", Options: []string{"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda"}},
	})
	bounds := layout.Region{Left: 2, Top: 2, Width: 40, Height: p.Height()}
	p.Draw(s, bounds)

	if all := askScreenText(s, 60, 30); !strings.Contains(all, "lambda") {
		t.Fatalf("long option was clipped; tail not rendered:\n%s", all)
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
		{ID: "q1", Question: "Which?", Type: "checkbox"},
	})
	if !p.Active() {
		t.Fatal("picker should activate for an unknown type")
	}
	if p.states[0].input == nil {
		t.Fatal("unknown type should fall back to a text field")
	}

	for _, r := range "yes" {
		p.HandleKey(askRune(r))
	}
	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after confirming")
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
		{ID: "q1", Question: "Which?", Type: "checkbox"},
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

func TestAskPickerMultiSelect(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	sent := false
	p.SetOnSend(func(answers map[string]string) {
		got = answers
		sent = true
	})
	p.SetQuestions([]AskQuestion{
		{ID: "tags", Question: "Tags?", Type: "multiselect", Options: []string{"a", "b", "c"}},
	})

	p.HandleKey(askRune(' '))
	p.HandleKey(askKey(tcell.KeyDown))
	p.HandleKey(askRune(' '))
	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))

	if p.Active() {
		t.Fatal("picker should close after confirming")
	}
	if !sent || got["tags"] != "a, b" {
		t.Fatalf("answers = %v, want tags=\"a, b\"", got)
	}
}

func TestAskPickerMultiSelectRequiredNeedsSelection(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "tags", Question: "Tags?", Type: "multiselect", Options: []string{"a", "b"}, Required: true},
	})
	if !p.HandleKey(askKey(tcell.KeyEnter)) {
		t.Fatal("Enter should be handled")
	}
	if !p.Active() || p.confirming {
		t.Fatal("required multiselect must block entering the confirm page")
	}
}

func TestAskPickerBoolean(t *testing.T) {
	p := NewAskPicker()
	var got map[string]string
	p.SetOnSend(func(answers map[string]string) {
		got = answers
	})
	p.SetQuestions([]AskQuestion{
		{ID: "proceed", Question: "Proceed?", Type: "boolean"},
	})

	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after confirming")
	}
	if got["proceed"] != "yes" {
		t.Fatalf("answers = %v, want proceed=yes", got)
	}
}

func TestAskPickerConfirmEscGoesBack(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{{ID: "a", Question: "First?", Type: "text"}})
	p.HandleKey(askRune('x'))

	p.HandleKey(askKey(tcell.KeyEnter))
	if !p.confirming {
		t.Fatal("Enter should open the confirm page")
	}
	p.HandleKey(askKey(tcell.KeyEsc))
	if !p.Active() || p.confirming {
		t.Fatal("Esc on the confirm page should return to the questions")
	}
	if got := p.states[0].input.Value(); got != "x" {
		t.Fatalf("value = %q, want x (answer must be preserved)", got)
	}

	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.Active() {
		t.Fatal("picker should close after confirming")
	}
}

func TestAskPickerSelectUsesSpace(t *testing.T) {
	p := NewAskPicker()
	p.SetQuestions([]AskQuestion{
		{ID: "c", Question: "Color?", Type: "select", Options: []string{"Red", "Green", "Blue"}},
	})
	p.HandleKey(askKey(tcell.KeyDown))
	p.HandleKey(askRune(' '))
	p.HandleKey(askKey(tcell.KeyEnter))
	p.HandleKey(askKey(tcell.KeyEnter))
	if p.states[0].answer != "Green" {
		t.Fatalf("answer = %q, want Green", p.states[0].answer)
	}
}
