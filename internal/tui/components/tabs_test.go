package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/styles"
)

func TestTabsScrollKeepsActiveVisible(t *testing.T) {
	names := []string{"General", "Session", "MCP", "Skills", "Rules", "Plugins", "Permissions", "Memory", "System"}
	tabs := NewTabs(names)

	width := 40
	tabs.SetActive(8)
	start, end := tabs.fitWindow(width)
	if start >= 8 || end < 9 {
		t.Fatalf("window [%d,%d) does not contain active tab 8", start, end)
	}
	if tabs.scroll != start {
		t.Fatalf("scroll = %d, want %d", tabs.scroll, start)
	}

	tabs.SetActive(0)
	start, end = tabs.fitWindow(width)
	if start != 0 || end <= 0 {
		t.Fatalf("window [%d,%d) does not contain active tab 0", start, end)
	}

	tabs.SetActive(8)
	start, end = tabs.fitWindow(200)
	if start != 0 || end != len(names) {
		t.Fatalf("window [%d,%d), want [0,%d)", start, end, len(names))
	}
	if tabs.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", tabs.scroll)
	}
}

func TestTabsMouseHit(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(80, 10)

	tabs := NewTabs([]string{"General", "Session", "MCP"})
	tabs.Draw(s, 0, 0, 40, true)
	idx, ok := tabs.HandleMouse(1, 0)
	if !ok || idx != 0 {
		t.Fatalf("hit = (%d,%v), want (0,true)", idx, ok)
	}

	narrow := NewTabs([]string{"General", "Session", "MCP", "Skills", "Rules", "Plugins", "Permissions", "Memory", "System"})
	narrow.Draw(s, 0, 0, 20, true)
	if !narrow.hasRight {
		t.Skip("tabs did not overflow at this width")
	}
	before := narrow.Active()
	idx, ok = narrow.HandleMouse(narrow.rightX0, 0)
	if !ok || idx != before+1 {
		t.Fatalf("right arrow hit = (%d,%v), want (%d,true)", idx, ok, before+1)
	}
}
