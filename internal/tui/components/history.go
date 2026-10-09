package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type HistoryKind int

const (
	HistoryUser HistoryKind = iota
	HistoryAssistant
	HistoryTool
)

type HistoryLine struct {
	Prefix string
	Text   string
	Kind   HistoryKind
}

type History struct {
	lines      []HistoryLine
	scroll     int
	autoScroll bool
}

func NewHistory() *History { return &History{autoScroll: true} }

func (h *History) SetLines(lines []HistoryLine) {
	h.lines = lines
	h.scroll = len(lines)
	h.autoScroll = true
}

func (h *History) Clear() {
	h.lines = nil
	h.scroll = 0
	h.autoScroll = true
}

func (h *History) HasContent() bool { return len(h.lines) > 0 }

func (h *History) scrollBy(n int) {
	h.autoScroll = false
	h.scroll += n
}

func (h *History) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyPgUp:
		h.scrollBy(-10)
		return true
	case tcell.KeyPgDn:
		h.scrollBy(10)
		return true
	case tcell.KeyHome:
		h.autoScroll = false
		h.scroll = 0
		return true
	case tcell.KeyEnd:
		h.autoScroll = true
		h.scroll = len(h.lines)
		return true
	case tcell.KeyUp:
		h.scrollBy(-1)
		return true
	case tcell.KeyDown:
		h.scrollBy(1)
		return true
	}
	return false
}

func (h *History) Draw(s tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	bg := th.Base().Background(th.Background)
	FillRegion(s, bounds, ' ', bg)

	if len(h.lines) == 0 {
		DrawText(s, bounds.Left+1, bounds.Top+1, "(no history)", th.Base().Foreground(th.Placeholder).Background(th.Background))
		return
	}

	visible := bounds.Height
	if h.autoScroll || h.scroll > len(h.lines) {
		h.scroll = len(h.lines)
	}
	if h.scroll < 0 {
		h.scroll = 0
	}
	top := h.scroll - visible
	if top < 0 {
		top = 0
	}

	userStyle := th.Base().Foreground(th.Accent).Bold(true).Background(th.Background)
	assistantStyle := th.Base().Foreground(th.Foreground).Background(th.Background)
	toolStyle := th.Base().Foreground(th.Hint).Background(th.Background)

	for i := top; i < len(h.lines) && i < top+visible; i++ {
		line := h.lines[i]
		var style tcell.Style
		switch line.Kind {
		case HistoryUser:
			style = userStyle
		case HistoryTool:
			style = toolStyle
		default:
			style = assistantStyle
		}
		text := TruncateTo(line.Prefix+line.Text, bounds.Width-2)
		DrawText(s, bounds.Left+1, bounds.Top+i-top, text, style)
	}
}
