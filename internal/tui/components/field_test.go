package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

func TestFieldMouseClick(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(40, 10)

	f := NewField("title")
	f.SetText("hello")
	f.Focus()
	bounds := layout.Region{Left: 0, Top: 0, Width: 40, Height: 6}
	f.Draw(s, bounds, true)

	if !f.HandleMouse(bounds.Left+3, bounds.Top+2, tcell.ButtonPrimary) {
		t.Fatal("click inside the field should be handled")
	}
	if f.pos != 2 {
		t.Fatalf("pos = %d, want 2", f.pos)
	}

	if f.HandleMouse(bounds.Left+3, bounds.Bottom()+5, tcell.ButtonPrimary) {
		t.Fatal("click outside the field should not be handled")
	}
}
