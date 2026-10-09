package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/styles"
)

type Tabs struct {
	names   []string
	active  int
	focused bool
	scroll  int

	hits []tabHit

	hasLeft, hasRight bool
	leftX0, leftX1    int
	rightX0, rightX1  int
}

type tabHit struct {
	index  int
	x0, x1 int
}

func NewTabs(names []string) *Tabs {
	return &Tabs{names: names}
}

func (t *Tabs) SetActive(n int) {
	t.active = n
	t.ensureVisible()
}

func (t *Tabs) Active() int { return t.active }

func (t *Tabs) Count() int { return len(t.names) }

func (t *Tabs) SetFocused(f bool) { t.focused = f }

func (t *Tabs) Focused() bool { return t.focused }

func (t *Tabs) HandleKey(ev *tcell.EventKey) bool {
	if !t.focused {
		return false
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		if t.active > 0 {
			t.active--
		} else {
			t.active = len(t.names) - 1
		}
		t.ensureVisible()
		return true
	case tcell.KeyRight:
		if t.active < len(t.names)-1 {
			t.active++
		} else {
			t.active = 0
		}
		t.ensureVisible()
		return true
	}
	return false
}

func tabWidth(name string) int {
	return len([]rune(name)) + 3
}

func (t *Tabs) fitWindow(width int) (int, int) {
	n := len(t.names)
	if n == 0 {
		return 0, 0
	}
	budget := width - 4
	if budget < 0 {
		budget = 0
	}
	total := func(a, b int) int {
		w := 0
		for i := a; i < b; i++ {
			w += tabWidth(t.names[i])
		}
		return w
	}
	if total(0, n) <= budget {
		t.scroll = 0
		return 0, n
	}
	start := t.scroll
	if start >= n {
		start = 0
	}
	if start > t.active {
		start = t.active
	}
	for start < t.active && total(start, t.active+1) > budget {
		start++
	}
	if start == t.active && total(start, t.active+1) > budget {
		t.scroll = start
		return start, t.active + 1
	}
	end := t.active + 1
	for end < n && total(start, end+1) <= budget {
		end++
	}
	for start > 0 && total(start-1, end) <= budget {
		start--
	}
	t.scroll = start
	return start, end
}

func (t *Tabs) ensureVisible() {
	if t.scroll > t.active {
		t.scroll = t.active
	}
}

func (t *Tabs) Draw(s tcell.Screen, x, y, width int, focused bool) {
	th := styles.Current()
	start, end := t.fitWindow(width)
	hasLeft := start > 0
	hasRight := end < len(t.names)

	cx := x
	t.hasLeft, t.hasRight = hasLeft, hasRight
	if hasLeft {
		t.leftX0, t.leftX1 = cx, cx
		DrawText(s, cx, y, "◀", th.Base().Foreground(th.Hint).Background(th.InputBg))
		cx += 2
	}
	t.hits = t.hits[:0]
	for i := start; i < end; i++ {
		name := t.names[i]
		w := tabWidth(name)
		t.hits = append(t.hits, tabHit{index: i, x0: cx, x1: cx + w - 1})
		style := th.Base().Foreground(th.Hint).Background(th.InputBg)
		if i == t.active {
			if focused {
				style = th.Base().Foreground(th.InputBg).Background(th.Accent)
			} else {
				style = th.Base().Foreground(th.InputBg).Background(th.AccentDim)
			}
		}
		DrawText(s, cx, y, " "+name+" ", style)
		cx += w
	}
	if hasRight {
		t.rightX0, t.rightX1 = cx, cx
		DrawText(s, cx, y, "▶", th.Base().Foreground(th.Hint).Background(th.InputBg))
	}
}

func (t *Tabs) HandleMouse(x, y int) (int, bool) {
	if t.hasLeft && x >= t.leftX0 && x <= t.leftX1 {
		if t.active > 0 {
			t.active--
		}
		t.ensureVisible()
		return t.active, true
	}
	if t.hasRight && x >= t.rightX0 && x <= t.rightX1 {
		if t.active < len(t.names)-1 {
			t.active++
		}
		t.ensureVisible()
		return t.active, true
	}
	for _, h := range t.hits {
		if x >= h.x0 && x <= h.x1 {
			return h.index, true
		}
	}
	return 0, false
}
