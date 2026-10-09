package components

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
	"github.com/peggco/pegg/internal/utils/search"
)

type Picker struct {
	title    string
	all      []ListItem
	filtered []ListItem
	index    int
	scroll   int
}

func NewPicker(title string) *Picker {
	return &Picker{title: title}
}

func (p *Picker) SetAll(items []ListItem) {
	p.all = items
	p.Update("")
}

func (p *Picker) All() []ListItem { return p.all }

func (p *Picker) Count() int { return len(p.filtered) }

func (p *Picker) Update(query string) {
	p.filtered = search.Filter(query, p.all, func(it ListItem) []string {
		return []string{it.Label, it.Detail}
	})
	p.index = 0
	p.scroll = 0
}

func (p *Picker) Selected() (ListItem, bool) {
	if len(p.filtered) == 0 {
		return ListItem{}, false
	}
	return p.filtered[p.index], true
}

func (p *Picker) MoveUp() {
	if p.index > 0 {
		p.index--
	}
}

func (p *Picker) MoveDown() {
	if p.index < len(p.filtered)-1 {
		p.index++
	}
}

func (p *Picker) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		p.MoveUp()
		return true
	case tcell.KeyDown:
		p.MoveDown()
		return true
	}
	return false
}

func (p *Picker) Draw(s tcell.Screen, bounds layout.Region) {
	th := styles.Current()
	bg := th.Base().Background(th.InputBg)

	inner := DrawCenteredBox(s, bounds, bounds.Width, bounds.Height, p.title)
	if len(p.filtered) == 0 {
		DrawText(s, inner.Left+1, inner.Top+1, "(no matching items)", th.Base().Foreground(th.Placeholder).Background(th.InputBg))
		return
	}

	visible := inner.Height - 2
	if p.index < p.scroll {
		p.scroll = p.index
	}
	if p.index >= p.scroll+visible {
		p.scroll = p.index - visible + 1
	}

	for i := 0; i < visible; i++ {
		idx := p.scroll + i
		if idx >= len(p.filtered) {
			break
		}
		it := p.filtered[idx]
		y := inner.Top + 1 + i
		style := bg
		marker := "  "
		if idx == p.index {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		}
		DrawText(s, inner.Left+1, y, marker+TruncateTo(it.Label, inner.Width-2), style)
		if it.Detail != "" {
			d := TruncateTo(it.Detail, inner.Width/3)
			DrawText(s, inner.Right()-len(d)-1, y, d, style)
		}
	}
}
