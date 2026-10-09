package components

import (
	"fmt"
	"math"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type Cell struct {
	R rune
	S tcell.Style
}

type Line []Cell

type Segment struct {
	Text  string
	Style tcell.Style
}

func (s Segment) Width() int {
	gr := uniseg.NewGraphemes(s.Text)
	w := 0
	for gr.Next() {
		w += gr.Width()
	}
	return w
}

func (l Line) Width() int {
	w := 0
	for _, c := range l {
		w += cellWidth(c.R)
	}
	return w
}

func RuneWidth(r rune) int {
	return cellWidth(r)
}

func DisplayWidth(text string) int {
	gr := uniseg.NewGraphemes(text)
	w := 0
	for gr.Next() {
		w += gr.Width()
	}
	return w
}

func truncateToDisplayWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := cellWidth(r)
		if w+rw > max {
			return s[:i]
		}
		w += rw
	}
	return s
}

func cellWidth(r rune) int {
	switch {
	case r == '\t':
		return 1
	case r >= 0x1F000:
		return 2
	case r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6)):
		return 2
	default:
		return 1
	}
}

type grapheme struct {
	text  string
	width int
}

func graphemes(text string) []grapheme {
	var out []grapheme
	gr := uniseg.NewGraphemes(text)
	for gr.Next() {
		out = append(out, grapheme{text: gr.Str(), width: gr.Width()})
	}
	return out
}

func LineFromSegments(segs []Segment, width int) Line {
	var out Line
	w := 0
	for _, s := range segs {
		for _, g := range graphemes(s.Text) {
			if w+g.width > width {
				return out
			}
			for _, r := range g.text {
				out = append(out, Cell{R: r, S: s.Style})
			}
			w += g.width
		}
	}
	return out
}

func WrapText(text string, style tcell.Style, width int) []Line {
	if width <= 0 {
		return nil
	}
	return WrapSegments([]Segment{{Text: text, Style: style}}, width)
}

func WrapSegments(segs []Segment, width int) []Line {
	if width <= 0 {
		return nil
	}

	var lines []Line
	var cur Line
	curW := 0

	flush := func() {
		if len(cur) > 0 {
			lines = append(lines, cur)
			cur = nil
			curW = 0
		}
	}

	var pendingSpace *Cell
	pendingW := 0

	wordStart := -1
	wordStartW := 0

	for _, s := range segs {
		for _, g := range graphemes(s.Text) {
			if g.text == "\n" {
				flush()
				pendingSpace, pendingW = nil, 0
				wordStart, wordStartW = -1, 0
				continue
			}
			cells := make([]Cell, 0, len(g.text))
			for _, r := range g.text {
				cells = append(cells, Cell{R: r, S: s.Style})
			}

			if g.text == " " {
				c := Cell{R: ' ', S: s.Style}
				pendingSpace = &c
				pendingW = g.width
				wordStart, wordStartW = -1, 0
				continue
			}

			needed := g.width
			if pendingSpace != nil {
				needed += pendingW
			}
			if curW+needed > width {
				if wordStart > 0 {
					partial := append(Line{}, cur[wordStart:]...)
					partialW := curW - wordStartW
					cur = cur[:wordStart]
					flush()
					cur = partial
					curW = partialW
					wordStart, wordStartW = 0, 0
				} else {
					flush()
					wordStart, wordStartW = 0, 0
				}
				pendingSpace, pendingW = nil, 0
			}
			if pendingSpace != nil && len(cur) > 0 {
				cur = append(cur, *pendingSpace)
				curW += pendingW
				wordStart = len(cur)
				wordStartW = curW
				pendingSpace, pendingW = nil, 0
			} else if wordStart < 0 {
				wordStart = len(cur)
				wordStartW = curW
				pendingSpace, pendingW = nil, 0
			}
			cur = append(cur, cells...)
			curW += g.width
		}
	}
	flush()
	return lines
}

func TruncateLine(l Line, width int) Line {
	if width <= 0 {
		return nil
	}
	if l.Width() <= width {
		return l
	}
	if width == 1 {
		return l[:1]
	}
	out := l[:0]
	w := 0
	for _, c := range l {
		cw := cellWidth(c.R)
		if w+cw > width-1 {
			break
		}
		out = append(out, c)
		w += cw
	}
	out = append(out, Cell{R: '…', S: l[len(l)-1].S})
	return out
}

func DrawLine(s tcell.Screen, x, y int, l Line) {
	sW, sH := s.Size()
	if y < 0 || y >= sH || x >= sW {
		return
	}
	px := x
	for _, c := range l {
		if px >= sW {
			return
		}
		s.SetContent(px, y, c.R, nil, c.S)
		px += cellWidth(c.R)
	}
}

func DrawLineBounded(s tcell.Screen, x, y, maxX int, l Line) {
	sW, sH := s.Size()
	if y < 0 || y >= sH || x >= sW {
		return
	}
	if maxX > sW {
		maxX = sW
	}
	px := x
	for _, c := range l {
		if px >= maxX {
			return
		}
		s.SetContent(px, y, c.R, nil, c.S)
		px += cellWidth(c.R)
	}
}

func ClampScroll(offset, max int) int {
	if offset < 0 {
		return 0
	}
	if offset > max {
		return max
	}
	return offset
}

func glowT(elapsed time.Duration) float64 {
	return 0.5 + 0.5*math.Sin(float64(elapsed)/float64(time.Second)*2*math.Pi)
}

func lerpColor(a, b tcell.Color, t float64) tcell.Color {
	if uint64(a)&uint64(tcell.ColorIsRGB) == 0 || uint64(b)&uint64(tcell.ColorIsRGB) == 0 {
		if t < 0.5 {
			return a
		}
		return b
	}
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	return tcell.NewRGBColor(
		ar+int32(float64(br-ar)*t),
		ag+int32(float64(bg-ag)*t),
		ab+int32(float64(bb-ab)*t),
	)
}

var spinnerFrames = []rune{'◐', '◓', '◑', '◒'}

func spinnerAt(elapsed time.Duration) rune {
	return spinnerFrames[int(elapsed/120*time.Millisecond)%len(spinnerFrames)]
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
