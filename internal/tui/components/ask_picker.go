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
	active     bool
	confirming bool
	questions  []AskQuestion
	states     []*askState
	tab        int

	onSend func(answers map[string]string)
	onDone func()
}

type askState struct {
	q      AskQuestion
	input  *Input
	values []string
	cursor int
	scroll int
	answer string
	multi  map[string]bool
	err    string
}

func NewAskPicker() *AskPicker { return &AskPicker{} }

func (a *AskPicker) SetOnSend(fn func(map[string]string)) { a.onSend = fn }
func (a *AskPicker) SetOnDone(fn func())                  { a.onDone = fn }
func (a *AskPicker) Active() bool                         { return a.active }

func (a *AskPicker) Clear() {
	onSend, onDone := a.onSend, a.onDone
	*a = AskPicker{onSend: onSend, onDone: onDone}
}

func (a *AskPicker) SetQuestions(qs []AskQuestion) {
	onSend, onDone := a.onSend, a.onDone
	if len(qs) == 0 {
		a.Clear()
		if onDone != nil {
			onDone()
		}
		return
	}
	*a = AskPicker{active: true, questions: qs, onSend: onSend, onDone: onDone}
	for i := range qs {
		st := &askState{q: qs[i]}
		switch st.q.Type {
		case "select":
			if len(st.q.Options) > 0 {
				st.values = append([]string(nil), st.q.Options...)
			} else {
				a.textState(st)
			}
		case "multiselect":
			if len(st.q.Options) > 0 {
				st.values = append([]string(nil), st.q.Options...)
				st.multi = make(map[string]bool)
			} else {
				a.textState(st)
			}
		case "boolean":
			st.q.Options = []string{"Yes", "No"}
			st.values = []string{"yes", "no"}
		default:
			a.textState(st)
		}
		a.states = append(a.states, st)
	}
	a.focusCurrent()
}

func (a *AskPicker) textState(st *askState) {
	st.q.Type = "text"
	in := NewInput()
	in.SetRows(1, 2)
	in.Placeholder = "Type your answer…"
	st.input = in
}

func (a *AskPicker) current() *askState {
	if a.tab < 0 || a.tab >= len(a.states) {
		return nil
	}
	return a.states[a.tab]
}

func (a *AskPicker) next() {
	if len(a.states) == 0 {
		return
	}
	a.tab = (a.tab + 1) % len(a.states)
	a.focusCurrent()
}

func (a *AskPicker) prev() {
	if len(a.states) == 0 {
		return
	}
	a.tab = (a.tab - 1 + len(a.states)) % len(a.states)
	a.focusCurrent()
}

func (a *AskPicker) focusCurrent() {
	for i, st := range a.states {
		if st.input == nil {
			continue
		}
		if i == a.tab {
			st.input.Focus()
		} else {
			st.input.Blur()
		}
	}
}

func (st *askState) answered() bool {
	switch st.q.Type {
	case "text":
		return st.input != nil && strings.TrimSpace(st.input.Value()) != ""
	case "multiselect":
		for _, v := range st.multi {
			if v {
				return true
			}
		}
		return false
	default:
		return st.answer != ""
	}
}

func (a *AskPicker) answerOf(st *askState) string {
	switch st.q.Type {
	case "text":
		if st.input == nil {
			return ""
		}
		return strings.TrimSpace(st.input.Value())
	case "multiselect":
		sel := make([]string, 0, len(st.q.Options))
		for _, opt := range st.q.Options {
			if st.multi[opt] {
				sel = append(sel, opt)
			}
		}
		return strings.Join(sel, ", ")
	default:
		if st.answer != "" {
			return st.answer
		}
		if len(st.values) > 0 {
			return st.values[st.cursor]
		}
		return ""
	}
}

func (a *AskPicker) moveCursor(st *askState, delta int) {
	n := len(st.q.Options)
	if n == 0 {
		return
	}
	st.cursor = (st.cursor + delta + n) % n
	st.err = ""
	if st.q.Type != "multiselect" {
		st.answer = st.values[st.cursor]
	}
}

func (a *AskPicker) selectCurrent(st *askState) {
	if len(st.values) == 0 {
		return
	}
	st.err = ""
	if st.q.Type == "multiselect" {
		label := st.q.Options[st.cursor]
		st.multi[label] = !st.multi[label]
		return
	}
	st.answer = st.values[st.cursor]
}

func (a *AskPicker) collect() map[string]string {
	answers := make(map[string]string)
	for _, st := range a.states {
		if v := a.answerOf(st); v != "" {
			answers[st.q.ID] = v
		}
	}
	return answers
}

func (a *AskPicker) enterConfirm() {
	answers := a.collect()
	for i, st := range a.states {
		if st.q.Required && answers[st.q.ID] == "" {
			st.err = "This answer is required"
			a.tab = i
			a.confirming = false
			a.focusCurrent()
			return
		}
	}
	a.confirming = true
}

func (a *AskPicker) finalSubmit() {
	answers := a.collect()
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

func (a *AskPicker) InsertPaste(text string) {
	st := a.current()
	if st == nil || st.q.Type != "text" || st.input == nil {
		return
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			st.input.Newline()
		}
		for _, r := range line {
			st.input.InsertRune(r)
		}
	}
}

func (a *AskPicker) HandleKey(ev *tcell.EventKey) bool {
	if !a.active {
		return false
	}

	if a.confirming {
		switch ev.Key() {
		case tcell.KeyEsc:
			a.confirming = false
			a.focusCurrent()
		case tcell.KeyEnter:
			a.finalSubmit()
		}
		return true
	}

	st := a.current()
	if st == nil {
		return false
	}

	switch ev.Key() {
	case tcell.KeyEsc:
		a.cancel()
		return true
	case tcell.KeyBacktab:
		a.prev()
		return true
	case tcell.KeyTab:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.prev()
		} else {
			a.next()
		}
		return true
	}

	if st.q.Type == "text" {
		if st.input == nil {
			return false
		}
		if ev.Key() == tcell.KeyEnter && ev.Modifiers()&tcell.ModShift == 0 {
			a.enterConfirm()
			return true
		}
		return st.input.HandleKey(ev)
	}

	switch ev.Key() {
	case tcell.KeyUp:
		a.moveCursor(st, -1)
		return true
	case tcell.KeyDown:
		a.moveCursor(st, 1)
		return true
	case tcell.KeyLeft:
		a.prev()
		return true
	case tcell.KeyRight:
		a.next()
		return true
	case tcell.KeyEnter:
		a.enterConfirm()
		return true
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			a.selectCurrent(st)
			return true
		}
	}
	return false
}

func (a *AskPicker) Height() int {
	if !a.active {
		return 0
	}
	if a.confirming {
		n := len(a.states)
		if n < 1 {
			n = 1
		}
		return n + 4
	}
	st := a.current()
	if st == nil {
		return 0
	}
	body := 4
	if st.q.Type != "text" {
		body = len(st.q.Options)
		if body < 3 {
			body = 3
		}
		if body > 8 {
			body = 8
		}
	}
	return 8 + body
}

func askTabLabel(q AskQuestion, idx int) string {
	words := strings.Fields(q.Question)
	if len(words) == 0 {
		return fmt.Sprintf("Q%d", idx+1)
	}
	label := strings.Join(words, " ")
	if len(words) > 3 {
		label = strings.Join(words[:3], " ") + "…"
	}
	label = TruncateTo(label, 20)
	return label
}

func askRestyle(l Line, st tcell.Style) Line {
	out := make(Line, len(l))
	for i, c := range l {
		out[i] = Cell{R: c.R, S: st}
	}
	return out
}

func (a *AskPicker) Draw(s tcell.Screen, bounds layout.Region) {
	if !a.active || bounds.Width < 8 || bounds.Height < 4 {
		return
	}
	th := styles.Current()
	FillRegion(s, bounds, ' ', th.Base().Background(th.InputBg))
	DrawBox(s, bounds, th.Base().Foreground(th.Border))

	title := fmt.Sprintf(" Ask · Q%d/%d ", a.tab+1, len(a.states))
	if a.confirming {
		title = " Ask · Confirm "
	}
	DrawText(s, bounds.Left+2, bounds.Top, title, th.Base().Foreground(th.Accent).Background(th.InputBg))

	inner := layout.Region{
		Left:   bounds.Left + 2,
		Top:    bounds.Top + 1,
		Width:  bounds.Width - 4,
		Height: bounds.Height - 2,
	}
	if inner.Width < 4 || inner.Height < 4 {
		return
	}

	if a.confirming {
		a.drawConfirm(s, inner, th)
		return
	}

	st := a.current()
	if st == nil {
		return
	}

	footerY := inner.Bottom() - 1
	y := inner.Top

	cx := inner.Left
	for i, qs := range a.states {
		label := askTabLabel(qs.q, i)
		if qs.answered() {
			label = "✓ " + label
		}
		chip := " " + label + " "
		w := DisplayWidth(chip)
		if cx+w > inner.Right() {
			break
		}
		style := th.Base().Foreground(th.Muted).Background(th.InputBg)
		if i == a.tab {
			style = th.Base().Foreground(th.InputBg).Background(th.Accent)
		}
		DrawText(s, cx, y, chip, style)
		cx += w + 1
	}
	y += 2

	qLines := WrapText(st.q.Question, th.Base().Foreground(th.Foreground).Background(th.InputBg), inner.Width)
	if len(qLines) > 2 {
		qLines = qLines[:2]
	}
	for i, ln := range qLines {
		if y+i >= footerY {
			break
		}
		DrawLine(s, inner.Left, y+i, ln)
	}
	y += len(qLines) + 1

	bodyTop := y
	bodyH := footerY - bodyTop
	if bodyH < 1 {
		bodyTop = footerY - 1
		bodyH = 1
	}
	body := layout.Region{Left: inner.Left, Top: bodyTop, Width: inner.Width, Height: bodyH}

	if st.q.Type == "text" {
		st.input.SetInnerWidth(body.Width - 2)
		st.input.Draw(s, body, true)
	} else {
		a.drawOptions(s, st, body)
	}

	hint := "Tab next · ↑/↓ choose · Space select · ⏎ confirm · Esc cancel"
	switch st.q.Type {
	case "text":
		hint = "Tab next · ⇧⏎ newline · ⏎ confirm · Esc cancel"
	case "multiselect":
		hint = "Tab next · ↑/↓ move · Space toggle · ⏎ confirm · Esc cancel"
	}
	hintStyle := th.Base().Foreground(th.Hint).Background(th.InputBg)
	if st.err != "" {
		hint = st.err
		hintStyle = th.Base().Foreground(th.Error).Background(th.InputBg)
	}
	DrawText(s, inner.Left, footerY, TruncateTo(hint, inner.Width), hintStyle)
}

func (a *AskPicker) drawOptions(s tcell.Screen, st *askState, region layout.Region) {
	th := styles.Current()
	base := th.Base().Foreground(th.InputText).Background(th.InputBg)
	dim := th.Base().Foreground(th.Placeholder).Background(th.InputBg)

	rows := make([]Line, 0, len(st.q.Options))
	rowOf := make([]int, 0, len(st.q.Options))
	cursorStart, cursorEnd := 0, 0
	for i, opt := range st.q.Options {
		var marker string
		switch {
		case st.q.Type == "multiselect":
			if st.multi[opt] {
				marker = "[x] "
			} else {
				marker = "[ ] "
			}
		case st.answer != "" && st.values[i] == st.answer:
			marker = "(•) "
		default:
			marker = "( ) "
		}
		style := base
		if st.q.Type == "multiselect" && !st.multi[opt] {
			style = dim
		}
		lines := WrapText(marker+opt, style, region.Width)
		if len(lines) == 0 {
			lines = []Line{nil}
		}
		if i == st.cursor {
			cursorStart = len(rows)
		}
		for _, ln := range lines {
			rows = append(rows, ln)
			rowOf = append(rowOf, i)
		}
		if i == st.cursor {
			cursorEnd = len(rows) - 1
		}
	}

	visible := region.Height
	if st.scroll > cursorStart {
		st.scroll = cursorStart
	}
	if cursorEnd >= st.scroll+visible {
		st.scroll = cursorEnd - visible + 1
	}
	if st.scroll < 0 {
		st.scroll = 0
	}
	if maxScroll := len(rows) - visible; st.scroll > maxScroll {
		st.scroll = maxScroll
		if st.scroll < 0 {
			st.scroll = 0
		}
	}

	cursorStyle := th.Base().Foreground(th.InputText).Background(th.Selection)
	for i := 0; i < visible; i++ {
		ri := st.scroll + i
		if ri >= len(rows) {
			break
		}
		ln := rows[ri]
		if rowOf[ri] == st.cursor {
			ln = askRestyle(ln, cursorStyle)
		}
		DrawLine(s, region.Left, region.Top+i, ln)
	}
}

func (a *AskPicker) drawConfirm(s tcell.Screen, inner layout.Region, th styles.Theme) {
	footerY := inner.Bottom() - 1
	qStyle := th.Base().Foreground(th.Muted).Background(th.InputBg)
	aStyle := th.Base().Foreground(th.InputText).Background(th.InputBg)
	half := inner.Width / 2
	if half < 8 {
		half = 8
	}
	y := inner.Top
	for _, st := range a.states {
		if y >= footerY {
			break
		}
		DrawText(s, inner.Left, y, TruncateTo(st.q.Question, half), qStyle)
		ans := a.answerOf(st)
		if ans == "" {
			ans = "—"
		}
		DrawText(s, inner.Left+half+1, y, TruncateTo(ans, inner.Width-half-1), aStyle)
		y++
	}
	DrawText(s, inner.Left, footerY, TruncateTo("Esc back · ⏎ submit", inner.Width),
		th.Base().Foreground(th.Hint).Background(th.InputBg))
}
