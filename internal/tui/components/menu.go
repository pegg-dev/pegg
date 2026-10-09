package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type MenuItem struct {
	Label    string
	OnSelect func()
}

type Menu struct {
	items   []MenuItem
	index   int
	anchorX int
	anchorY int

	region  layout.Region
	onClose func()
}

func NewMenu(items []MenuItem, anchorX, anchorY int) *Menu {
	return &Menu{items: items, anchorX: anchorX, anchorY: anchorY}
}

func (m *Menu) SetOnClose(fn func()) { m.onClose = fn }

func (m *Menu) close() {
	if m.onClose != nil {
		m.onClose()
	}
}

func (m *Menu) width() int {
	w := 0
	for _, it := range m.items {
		if n := DisplayWidth(it.Label); n > w {
			w = n
		}
	}
	return w + 6
}

func (m *Menu) height() int { return len(m.items) + 2 }

func (m *Menu) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEsc:
		m.close()
		return true
	case tcell.KeyUp:
		if m.index > 0 {
			m.index--
		}
		return true
	case tcell.KeyDown:
		if m.index < len(m.items)-1 {
			m.index++
		}
		return true
	case tcell.KeyEnter:
		m.selectIndex(m.index)
		return true
	}
	return false
}

func (m *Menu) selectIndex(i int) {
	if i < 0 || i >= len(m.items) {
		return
	}
	if m.items[i].OnSelect != nil {
		m.items[i].OnSelect()
	}
}

func (m *Menu) Draw(s tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	w := m.width()
	h := m.height()
	if w > bounds.Width {
		w = bounds.Width
	}
	if h > bounds.Height {
		h = bounds.Height
	}
	x := m.anchorX
	y := m.anchorY + 1
	if x+w > bounds.Right() {
		x = bounds.Right() - w
	}
	if y+h > bounds.Bottom() {
		y = bounds.Bottom() - h
	}
	if x < bounds.Left {
		x = bounds.Left
	}
	if y < bounds.Top {
		y = bounds.Top
	}
	m.region = layout.Region{Left: x, Top: y, Width: w, Height: h}

	bg := th.Base().Background(th.InputBg)
	FillRegion(s, m.region, ' ', bg)
	DrawBox(s, m.region, th.Base().Foreground(th.Border))

	for i, it := range m.items {
		rowY := m.region.Top + 1 + i
		if rowY >= m.region.Bottom()-1 {
			break
		}
		rowStyle := bg
		if i == m.index {
			rowStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
		}
		FillRegion(s, layout.Region{Left: m.region.Left + 1, Top: rowY, Width: m.region.Width - 2, Height: 1}, ' ', rowStyle)
		DrawText(s, m.region.Left+2, rowY, TruncateTo(it.Label, m.region.Width-4), rowStyle)
	}
}

func (m *Menu) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	r := m.region
	inside := x >= r.Left && x < r.Right() && y >= r.Top && y < r.Bottom()

	if buttons&tcell.ButtonPrimary != 0 {
		if !inside {
			m.close()
			return true
		}
		if idx := y - (r.Top + 1); idx >= 0 && idx < len(m.items) {
			m.index = idx
			m.selectIndex(idx)
		}
		return true
	}

	if inside {
		if idx := y - (r.Top + 1); idx >= 0 && idx < len(m.items) && idx != m.index {
			m.index = idx
			return true
		}
	}
	return false
}
