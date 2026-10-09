package components

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type AskQuestion struct {
	ID       string
	Question string
	Type     string
	Options  []string
	Required bool
}

type AskPicker struct {
	active    bool
	questions []AskQuestion
	answers   map[string]string
	tab       int
	states    []*askPS

	onSend func(answers map[string]string)
	onDone func()
}

type askPS struct {
	q          AskQuestion
	field      *Field
	list       *List
	customMode bool
	multi      bool
}

func NewAskPicker() *AskPicker {
	return &AskPicker{answers: make(map[string]string)}
}

func (a *AskPicker) SetQuestions(qs []AskQuestion) {
	onSend := a.onSend
	onDone := a.onDone
	if len(qs) == 0 {
		a.Clear()
		if onDone != nil {
			onDone()
		}
		return
	}
	*a = AskPicker{
		active:    true,
		questions: qs,
		answers:   make(map[string]string),
		tab:       0,
		onSend:    onSend,
		onDone:    onDone,
	}
	for i := range qs {
		q := qs[i]
		s := &askPS{q: q}
		switch q.Type {
		case "select":
			s.list = NewList("")
			s.list.SetItems(askOptionItems(q.Options, true))
			id := q.ID
			s.list.SetOnSelect(func(_ int, item ListItem) {
				if label, ok := item.Data.(string); ok && label == "__custom__" {
					s.customMode = true
					s.field = NewField("")
					s.field.SetPlaceholder("Type your custom answer...")
					s.field.Focus()
					s.field.SetOnEnter(func(val string) {
						if val != "" {
							a.answers[id] = val
						}
						a.next()
					})
					return
				}
				a.answers[id] = item.Label
				askMarkSelected(s.list, item.Label)
				a.next()
			})
		case "multiselect":
			s.multi = true
			s.list = NewList("")
			s.list.SetItems(askOptionItems(q.Options, false))
			id := q.ID
			s.list.SetOnSelect(func(idx int, item ListItem) {
				s.list.SetMarked(idx, !item.Marked)
				a.answers[id] = askMultiAnswer(s.list)
			})
		case "boolean":
			s.list = NewList("")
			s.list.SetItems([]ListItem{{Label: "Yes", Data: "yes"}, {Label: "No", Data: "no"}})
			id := q.ID
			s.list.SetOnSelect(func(_ int, item ListItem) {
				if v, ok := item.Data.(string); ok {
					a.answers[id] = v
				}
				askMarkSelected(s.list, item.Label)
				a.next()
			})
		default:
			s.q.Type = "text"
			s.field = NewField("")
			s.field.SetPlaceholder("Type your answer...")
			id := q.ID
			s.field.SetOnEnter(func(val string) {
				if val != "" {
					a.answers[id] = val
				}
				a.next()
			})
		}
		if s.field != nil {
			s.field.Focus()
		}
		a.states = append(a.states, s)
	}
}

func askOptionItems(options []string, custom bool) []ListItem {
	items := make([]ListItem, 0, len(options)+1)
	for _, opt := range options {
		items = append(items, ListItem{Label: opt, Data: opt})
	}
	if custom {
		items = append(items, ListItem{Label: "Custom answer...", Data: "__custom__"})
	}
	return items
}

func askMultiAnswer(l *List) string {
	items := l.Items()
	selected := make([]string, 0, len(items))
	for _, item := range items {
		if item.Marked {
			selected = append(selected, item.Label)
		}
	}
	return strings.Join(selected, ", ")
}

func askMarkSelected(l *List, label string) {
	items := l.Items()
	for i := range items {
		items[i].Marked = items[i].Label == label
	}
	l.SetItems(items)
}

func (a *AskPicker) SetOnSend(fn func(map[string]string)) { a.onSend = fn }
func (a *AskPicker) SetOnDone(fn func())                  { a.onDone = fn }
func (a *AskPicker) Active() bool                         { return a.active }
func (a *AskPicker) Clear() {
	*a = AskPicker{onSend: a.onSend, onDone: a.onDone, answers: make(map[string]string)}
}

func (a *AskPicker) Height() int {
	if !a.active {
		return 0
	}
	st := a.current()
	if st == nil {
		return 0
	}
	if st.list != nil {
		h := len(st.q.Options) + 6
		if h > 12 {
			h = 12
		}
		return h
	}
	return 11
}

func (a *AskPicker) current() *askPS {
	if a.tab < 0 || a.tab >= len(a.states) {
		return nil
	}
	return a.states[a.tab]
}

func (a *AskPicker) next() {
	if a.tab >= len(a.states)-1 {
		a.send()
		return
	}
	a.tab++
	a.focusCurrent()
}

func (a *AskPicker) prev() {
	if a.tab > 0 {
		a.tab--
	}
	a.focusCurrent()
}

func (a *AskPicker) focusCurrent() {
	st := a.current()
	if st == nil {
		return
	}
	if st.field != nil {
		st.field.Focus()
	}
}

func (a *AskPicker) send() {
	answers := make(map[string]string)
	for _, st := range a.states {
		if v, ok := a.answers[st.q.ID]; ok && v != "" {
			answers[st.q.ID] = v
		}
	}
	if a.onSend != nil {
		a.onSend(answers)
	}
	a.active = false
	if a.onDone != nil {
		a.onDone()
	}
}

func (a *AskPicker) cancel() {
	if a.onSend != nil {
		a.onSend(nil)
	}
	a.active = false
	if a.onDone != nil {
		a.onDone()
	}
}

func (a *AskPicker) HandleKey(ev *tcell.EventKey) bool {
	if !a.active {
		return false
	}

	st := a.current()
	if st == nil {
		if ev.Key() == tcell.KeyEsc {
			a.cancel()
			return true
		}
		return false
	}

	if st.customMode {
		if ev.Key() == tcell.KeyEsc {
			st.customMode = false
			st.field = nil
			return true
		}
		if st.field != nil && st.field.HandleKey(ev) {
			return true
		}
		return false
	}

	if ev.Key() == tcell.KeyEsc {
		a.cancel()
		return true
	}

	if st.field != nil {
		switch ev.Key() {
		case tcell.KeyLeft:
			a.prev()
			return true
		case tcell.KeyRight:
			a.next()
			return true
		}
		if st.field.HandleKey(ev) {
			return true
		}
		return false
	}

	if st.list != nil {
		switch ev.Key() {
		case tcell.KeyLeft:
			a.prev()
			return true
		case tcell.KeyRight:
			a.next()
			return true
		}
		if st.multi {
			switch ev.Key() {
			case tcell.KeyRune:
				if ev.Rune() == ' ' {
					if idx := st.list.SelectedIndex(); idx >= 0 {
						if item, ok := st.list.Selected(); ok {
							st.list.SetMarked(idx, !item.Marked)
							a.answers[st.q.ID] = askMultiAnswer(st.list)
						}
					}
					return true
				}
			case tcell.KeyEnter:
				if st.q.Required && a.answers[st.q.ID] == "" {
					return true
				}
				a.next()
				return true
			}
		}
		return st.list.HandleKey(ev)
	}

	return false
}

func (a *AskPicker) Draw(s tcell.Screen, bounds layout.Region) {
	if !a.active {
		return
	}
	th := styles.Current()

	n := len(a.states)
	title := fmt.Sprintf("Ask — Q%d/%d", a.tab+1, n)
	inner := DrawCenteredBox(s, bounds, bounds.Width, bounds.Height, title)

	st := a.current()
	if st == nil {
		return
	}
	q := st.q

	qy := inner.Top
	qtext := TruncateTo(q.Question, inner.Width)
	DrawText(s, inner.Left, qy, qtext, th.Base().Foreground(th.Foreground).Background(th.InputBg))

	iy := qy + 2
	inputRegion := layout.Region{
		Left:   inner.Left,
		Top:    iy,
		Width:  inner.Width,
		Height: inner.Bottom() - iy - 2,
	}

	if st.field != nil {
		st.field.Draw(s, inputRegion, true)
	} else if st.list != nil {
		st.list.Draw(s, inputRegion, true)
	}

	fy := inner.Bottom() - 1
	nav := fmt.Sprintf("  ◀  ▶  Q%d/%d  ", a.tab+1, n)
	if st.multi {
		nav = "  [Space] toggle  ◀  ▶  "
	}
	if a.tab == n-1 {
		if st.multi {
			nav = "  [Space] toggle  [Enter] Confirm"
		} else {
			nav = "  [Enter] Confirm"
		}
	}
	DrawText(s, inner.Left, fy, nav, th.Base().Foreground(th.Hint).Background(th.InputBg))
}
