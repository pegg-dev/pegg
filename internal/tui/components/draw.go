package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
)

func FillRegion(s tcell.Screen, r layout.Region, ch rune, style tcell.Style) {
	for y := r.Top; y < r.Bottom(); y++ {
		for x := r.Left; x < r.Right(); x++ {
			s.SetContent(x, y, ch, nil, style)
		}
	}
}

func DrawText(s tcell.Screen, x, y int, text string, style tcell.Style) {
	cx := x
	for _, r := range text {
		s.SetContent(cx, y, r, nil, style)
		cx++
	}
}

func DrawBox(s tcell.Screen, r layout.Region, style tcell.Style) {
	if r.Width < 2 || r.Height < 2 {
		return
	}
	right, bottom := r.Right()-1, r.Bottom()-1
	for x := r.Left + 1; x < right; x++ {
		s.SetContent(x, r.Top, '─', nil, style)
		s.SetContent(x, bottom, '─', nil, style)
	}
	for y := r.Top + 1; y < bottom; y++ {
		s.SetContent(r.Left, y, '│', nil, style)
		s.SetContent(right, y, '│', nil, style)
	}
	s.SetContent(r.Left, r.Top, '┌', nil, style)
	s.SetContent(right, r.Top, '┐', nil, style)
	s.SetContent(r.Left, bottom, '└', nil, style)
	s.SetContent(right, bottom, '┘', nil, style)
}

func TruncateTo(s string, w int) string {
	rs := []rune(s)
	if len(rs) <= w || w < 0 {
		return s
	}
	return string(rs[:w])
}

func TruncateLeftTo(s string, w int) string {
	rs := []rune(s)
	if len(rs) <= w || w < 0 {
		return s
	}
	return string(rs[len(rs)-w:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
