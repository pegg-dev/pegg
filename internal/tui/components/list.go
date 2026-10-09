package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
	"github.com/peggco/pegg/internal/utils/search"
)

type ListItem struct {
	Label    string
	Detail   string
	Data     any
	Marked   bool
	Disabled bool
}

type List struct {
	title    string
	all      []ListItem
	filtered []int
	index    int
	scroll   int
	filter   []rune
	search   bool
	onSelect func(index int, item ListItem)

	drawRegion  layout.Region
	drawTop     int
	drawVisible int
}

func NewList(title string) *List {
	return &List{title: title}
}

func (l *List) SetTitle(title string) { l.title = title }

func (l *List) Title() string { return l.title }

func (l *List) SetSearchable(b bool) { l.search = b }

func (l *List) SetOnSelect(fn func(index int, item ListItem)) { l.onSelect = fn }

func (l *List) SetItems(items []ListItem) {
	l.all = items
	l.index = 0
	l.scroll = 0
	l.filter = nil
	l.rebuild()
}

func (l *List) Items() []ListItem { return l.all }

func (l *List) SetMarked(index int, marked bool) {
	if index < 0 || index >= len(l.all) {
		return
	}
	l.all[index].Marked = marked
}

func (l *List) Count() int { return len(l.filtered) }

func (l *List) FilterText() string { return string(l.filter) }

func (l *List) Selected() (ListItem, bool) {
	if len(l.filtered) == 0 {
		return ListItem{}, false
	}
	return l.all[l.filtered[l.index]], true
}

func (l *List) SelectedIndex() int {
	if len(l.filtered) == 0 {
		return -1
	}
	return l.filtered[l.index]
}

func (l *List) rebuild() {
	idxs := make([]int, len(l.all))
	for i := range idxs {
		idxs[i] = i
	}
	filtered := search.Filter(string(l.filter), idxs, func(i int) []string {
		return []string{l.all[i].Label, l.all[i].Detail}
	})
	l.filtered = append(l.filtered[:0], filtered...)
	if l.index >= len(l.filtered) {
		l.index = 0
	}
}

func (l *List) MoveUp() {
	for i := l.index - 1; i >= 0; i-- {
		if !l.all[l.filtered[i]].Disabled {
			l.index = i
			return
		}
	}
}

func (l *List) MoveDown() {
	for i := l.index + 1; i < len(l.filtered); i++ {
		if !l.all[l.filtered[i]].Disabled {
			l.index = i
			return
		}
	}
}

func (l *List) SelectFirstEnabled() {
	for i := range l.filtered {
		if !l.all[l.filtered[i]].Disabled {
			l.index = i
			return
		}
	}
}

func (l *List) MovePageUp(page int) {
	l.index -= page
	if l.index < 0 {
		l.index = 0
	}
}

func (l *List) MovePageDown(page int) {
	l.index += page
	if l.index >= len(l.filtered) {
		l.index = len(l.filtered) - 1
	}
}

func (l *List) Home() {
	for i := range l.filtered {
		if !l.all[l.filtered[i]].Disabled {
			l.index = i
			return
		}
	}
	l.index = 0
}

func (l *List) End() {
	for i := len(l.filtered) - 1; i >= 0; i-- {
		if !l.all[l.filtered[i]].Disabled {
			l.index = i
			return
		}
	}
}

func (l *List) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		if l.index == 0 {
			return false
		}
		l.MoveUp()
		return true
	case tcell.KeyDown:
		if len(l.filtered) == 0 || l.index >= len(l.filtered)-1 {
			return false
		}
		l.MoveDown()
		return true
	case tcell.KeyPgUp:
		l.MovePageUp(10)
		return true
	case tcell.KeyPgDn:
		l.MovePageDown(10)
		return true
	case tcell.KeyHome:
		l.Home()
		return true
	case tcell.KeyEnd:
		l.End()
		return true
	case tcell.KeyEnter:
		if l.onSelect != nil {
			if item, ok := l.Selected(); ok && !item.Disabled {
				l.onSelect(l.SelectedIndex(), item)
			}
		}
		return true
	}
	if l.search {
		switch ev.Key() {
		case tcell.KeyRune:
			if r := ev.Rune(); r != 0 && r >= ' ' {
				l.filter = append(l.filter, r)
				l.rebuild()
				return true
			}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(l.filter) > 0 {
				l.filter = l.filter[:len(l.filter)-1]
				l.rebuild()
				return true
			}
		case tcell.KeyEscape:
			if len(l.filter) > 0 {
				l.filter = nil
				l.rebuild()
				return true
			}
		}
	}
	return false
}

func (l *List) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	if l.drawRegion.Width == 0 || x < l.drawRegion.Left || x >= l.drawRegion.Right() {
		return false
	}
	switch {
	case buttons&tcell.WheelUp != 0:
		l.MoveUp()
		return true
	case buttons&tcell.WheelDown != 0:
		l.MoveDown()
		return true
	}

	idx := y - l.drawTop
	inRow := idx >= 0 && idx < l.drawVisible
	abs := l.scroll + idx

	if buttons&tcell.ButtonPrimary != 0 {
		if !inRow || abs < 0 || abs >= len(l.filtered) {
			return false
		}
		if l.all[l.filtered[abs]].Disabled {
			return true
		}
		l.index = abs
		if l.onSelect != nil {
			if item, ok := l.Selected(); ok {
				l.onSelect(l.SelectedIndex(), item)
			}
		}
		return true
	}

	if inRow && abs >= 0 && abs < len(l.filtered) && abs != l.index && !l.all[l.filtered[abs]].Disabled {
		l.index = abs
		return true
	}
	return false
}

func (l *List) Draw(s tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	style := th.Base().Background(th.InputBg)
	FillRegion(s, bounds, ' ', style)

	l.drawRegion = bounds
	top := bounds.Top
	if l.search && len(l.filter) > 0 {
		DrawText(s, bounds.Left+1, top, "search: "+string(l.filter), th.Base().Foreground(th.Accent).Background(th.InputBg))
		top++
	}
	l.drawTop = top

	if len(l.filtered) == 0 {
		DrawText(s, bounds.Left+1, top, "(no items)", th.Base().Foreground(th.Placeholder).Background(th.InputBg))
		l.drawVisible = 0
		return
	}

	visible := bounds.Bottom() - top
	l.drawVisible = visible
	if l.index < l.scroll {
		l.scroll = l.index
	}
	if l.index >= l.scroll+visible {
		l.scroll = l.index - visible + 1
	}

	for i := 0; i < visible; i++ {
		idx := l.scroll + i
		if idx >= len(l.filtered) {
			break
		}
		item := l.all[l.filtered[idx]]
		y := top + i
		rowStyle := style
		switch {
		case item.Disabled:
			rowStyle = th.Base().Foreground(th.Muted).Background(th.InputBg)
		case idx == l.index:
			rowStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
		}
		label := item.Label
		if item.Marked {
			label = "● " + label
		}
		labelW := bounds.Width - 2
		detail := ""
		if item.Detail != "" {
			detail = TruncateTo(item.Detail, bounds.Width/2)
			if dw := DisplayWidth(detail); dw+2 < labelW {
				labelW = bounds.Width - 2 - dw - 2
			} else {
				detail = ""
			}
		}
		DrawText(s, bounds.Left+1, y, TruncateTo(label, labelW), rowStyle)
		if detail != "" {
			DrawText(s, bounds.Right()-DisplayWidth(detail)-1, y, detail, rowStyle)
		}
	}
}
