package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func TestMenuKeyboardNavigation(t *testing.T) {
	var picked string
	m := NewMenu([]MenuItem{
		{Label: "one", OnSelect: func() { picked = "one" }},
		{Label: "two", OnSelect: func() { picked = "two" }},
	}, 0, 0)

	m.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, 0))
	m.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if picked != "two" {
		t.Fatalf("picked = %q, want two", picked)
	}
}

func TestMenuEscCloses(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "one"}}, 0, 0)
	closed := false
	m.SetOnClose(func() { closed = true })
	if !m.HandleKey(tcell.NewEventKey(tcell.KeyEsc, 0, 0)) {
		t.Fatal("Esc should be handled")
	}
	if !closed {
		t.Fatal("Esc should close the menu")
	}
}

func TestMenuMouseSelects(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(40, 20)

	var picked string
	m := NewMenu([]MenuItem{
		{Label: "copy"},
		{Label: "revert", OnSelect: func() { picked = "revert" }},
	}, 5, 5)
	m.Draw(s, layout.Region{Left: 0, Top: 0, Width: 40, Height: 20}, true)

	x := m.region.Left + 2
	y := m.region.Top + 2
	if !m.HandleMouse(x, y, tcell.ButtonPrimary) {
		t.Fatal("mouse should be handled")
	}
	if picked != "revert" {
		t.Fatalf("picked = %q, want revert", picked)
	}
}

func TestMenuHoverHighlights(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(40, 20)

	m := NewMenu([]MenuItem{{Label: "a"}, {Label: "b"}}, 0, 0)
	m.Draw(s, layout.Region{Left: 0, Top: 0, Width: 40, Height: 20}, true)

	if !m.HandleMouse(m.region.Left+2, m.region.Top+2, tcell.ButtonNone) {
		t.Fatal("hover should be handled")
	}
	if m.index != 1 {
		t.Fatalf("index = %d, want 1 after hover", m.index)
	}
}
