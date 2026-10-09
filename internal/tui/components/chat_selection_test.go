package components

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/tui/styles"
)

func TestChatSelectionCopyStripsFrame(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "hello world"})
	c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "hi there"})
	c.rebuildFlat(80)
	if len(c.flat) == 0 {
		t.Fatal("no flat lines")
	}

	last := len(c.flat) - 1
	c.setSelection(selPos{line: 0, cell: 0}, selPos{line: last, cell: len(c.flat[last])})
	if !c.HasSelection() {
		t.Fatal("expected selection")
	}
	got := c.SelectedText()
	if !strings.Contains(got, "hello world") {
		t.Fatalf("selection missing user text: %q", got)
	}
	if !strings.Contains(got, "hi there") {
		t.Fatalf("selection missing assistant text: %q", got)
	}
	if strings.ContainsAny(got, "┌┐└┘─│▍") {
		t.Fatalf("selection should not contain frame characters: %q", got)
	}
}

func TestChatSelectionPartial(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemAssistant, Text: "hello world"})
	c.rebuildFlat(80)

	line, from, to := findText(c.flat, "world")
	if line < 0 {
		t.Fatalf("could not locate text in %v", c.flat)
	}
	c.setSelection(selPos{line: line, cell: from}, selPos{line: line, cell: to})
	if got := c.SelectedText(); got != "world" {
		t.Fatalf("SelectedText = %q, want %q", got, "world")
	}
}

func TestChatClearSelectionOnSetItems(t *testing.T) {
	styles.RegisterDefaults()
	styles.Set("dark")
	c := NewChat()
	c.AppendItem(&ChatItem{Kind: ItemUser, Text: "x"})
	c.rebuildFlat(80)
	c.setSelection(selPos{0, 0}, selPos{0, 1})
	c.SetItems(nil)
	if c.HasSelection() {
		t.Fatal("SetItems should clear the selection")
	}
}

func findText(lines []Line, needle string) (int, int, int) {
	target := []rune(needle)
	for li, line := range lines {
		for start := 0; start+len(target) <= len(line); start++ {
			match := true
			for k, r := range target {
				if line[start+k].R != r {
					match = false
					break
				}
			}
			if match {
				return li, start, start + len(target)
			}
		}
	}
	return -1, 0, 0
}
