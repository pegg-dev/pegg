package home

import (
	"math/rand"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type focusTarget int

const (
	focusInput focusTarget = iota
	focusAttachments
)

type pickKind int

const (
	pickNone pickKind = iota
	pickSkills
	pickMention
)

type Page struct {
	logo          *components.Logo
	input         *components.Input
	status        *components.StatusBar
	chat          *components.Chat
	attachmentBar *components.AttachmentBar
	hint          string

	skillPicker   *components.Picker
	mentionPicker *components.Picker
	askPicker     *components.AskPicker
	focus         focusTarget
	pickKind      pickKind

	pickDismissed bool
	pickTokenAt   int
}

func New() *Page {
	p := &Page{
		logo:          components.DefaultLogo(),
		input:         components.NewInput(),
		status:        components.NewStatusBar(),
		chat:          components.NewChat(),
		attachmentBar: components.NewAttachmentBar(),
		hint:          randomHint(),
		skillPicker:   components.NewPicker("Skills — type to filter"),
		mentionPicker: components.NewPicker("Mentions — type to filter"),
		askPicker:     components.NewAskPicker(),
		focus:         focusInput,
		pickTokenAt:   -1,
	}
	p.input.Focus()
	return p
}

func (p *Page) SetSkills(items []components.ListItem) {
	p.skillPicker.SetAll(items)
}

func (p *Page) SetMentionItems(items []components.ListItem) {
	p.mentionPicker.SetAll(items)
}

var defaultHints = []string{
	"use '@' to mention files, folders, or agents",
	"use '/' to run a skill",
	"use '/init' to create an AGENTS.md for this project",
	"use '/rule' to save a rule Pegg always follows",
	"use '/skillify' to turn this session into a reusable skill",
	"use '/review' to review a pull request",
	"use '/batch' to change many files at once",
	"press Ctrl+P for settings",
	"press Ctrl+T to cycle the theme",
	"press Esc twice to stop a running agent",
	"paste or drop a file to attach it",
}

func randomHint() string {
	if len(defaultHints) == 0 {
		return ""
	}
	return defaultHints[rand.Intn(len(defaultHints))]
}

func (p *Page) PickOpen() bool { return p.pickKind != pickNone }

func (p *Page) PickCount() int {
	switch p.pickKind {
	case pickSkills:
		return p.skillPicker.Count()
	case pickMention:
		return p.mentionPicker.Count()
	}
	return 0
}

func (p *Page) SkillPicker() *components.Picker { return p.skillPicker }

func (p *Page) MentionPicker() *components.Picker { return p.mentionPicker }

func (p *Page) AskPicker() *components.AskPicker { return p.askPicker }

func (p *Page) AskOpen() bool { return p.askPicker.Active() }

func (p *Page) SetAsk(items []components.AskQuestion) {
	p.askPicker.SetQuestions(items)
}

func (p *Page) ClearAsk() {
	p.askPicker.Clear()
}

func (p *Page) Input() *components.Input { return p.input }

func (p *Page) Chat() *components.Chat { return p.chat }

func (p *Page) AttachmentBar() *components.AttachmentBar { return p.attachmentBar }

func (p *Page) Logo() *components.Logo { return p.logo }

func (p *Page) SetModel(m string) {
	if m == "" {
		p.status.SetModelName("")
		p.status.SetProvider("")
		return
	}
	if i := strings.Index(m, "/"); i > 0 {
		p.status.SetProvider(m[:i])
		p.status.SetModelName(m[i+1:])
	} else {
		p.status.SetModelName(m)
		p.status.SetProvider("")
	}
}

func (p *Page) SetRunning(r bool)           { p.status.SetRunning(r) }
func (p *Page) SetReasoningEffort(e string) { p.status.SetReasoningEffort(e) }
func (p *Page) SetUsage(u llm.Usage)        { p.status.SetUsage(u) }
func (p *Page) SetMaxInputTokens(n int)     { p.status.SetMaxInputTokens(n) }

func (p *Page) HandleScroll(delta int) bool {
	if !p.chat.HasItems() {
		return false
	}
	p.chat.ScrollBy(delta)
	return true
}

func (p *Page) HandleClick(x, y, w, h int) bool {
	if !p.chat.HasItems() {
		return false
	}
	chatRegion := p.chatBounds(layout.Region{Left: 0, Top: 0, Width: w, Height: h})
	if x < chatRegion.Left || x >= chatRegion.Right() || y < chatRegion.Top || y >= chatRegion.Bottom() {
		return false
	}
	return p.chat.HandleClick(x, y, chatRegion.Top)
}

func (p *Page) HandleMouse(x, y, w, h int, buttons tcell.ButtonMask) bool {
	if !p.chat.HasItems() {
		return false
	}
	chatRegion := p.chatBounds(layout.Region{Left: 0, Top: 0, Width: w, Height: h})
	inside := x >= chatRegion.Left && x < chatRegion.Right() && y >= chatRegion.Top && y < chatRegion.Bottom()
	if !inside && buttons&tcell.ButtonPrimary != 0 {
		return false
	}
	return p.chat.HandleMouse(x, y, chatRegion.Top, chatRegion.Left, buttons)
}

func (p *Page) chatBounds(bounds layout.Region) layout.Region {
	statusRegion := layout.BottomAligned(bounds, 1)
	innerW := bounds.Width - 4
	if innerW < 1 {
		innerW = 1
	}
	p.input.SetInnerWidth(innerW)
	inputH := p.input.VisibleRows() + 2
	inputRegion := layout.Region{
		Left:   bounds.Left + 2,
		Top:    statusRegion.Top - inputH - 1,
		Width:  bounds.Width - 4,
		Height: inputH,
	}.Clamp(bounds)

	attachH := p.attachmentBar.RequiredHeight()
	chatBottom := inputRegion.Top - 1
	if attachH > 0 {
		chatBottom = inputRegion.Top - attachH - 2
	}

	return layout.Region{
		Left:   bounds.Left + 1,
		Top:    bounds.Top + 1,
		Width:  bounds.Width - 2,
		Height: chatBottom - bounds.Top - 1,
	}
}

func (p *Page) DrawErrorBanner(s tcell.Screen, bounds layout.Region, msg string, style tcell.Style) {
	if msg == "" {
		return
	}
	region := p.chatBounds(bounds)
	if region.Width < 1 || region.Height < 1 {
		return
	}
	lines := components.WrapText(msg, style, region.Width)
	if len(lines) == 0 {
		return
	}
	if len(lines) > region.Height {
		lines = lines[:region.Height]
	}
	y := region.Bottom() - len(lines)
	for i, ln := range lines {
		components.DrawLine(s, region.Left, y+i, ln)
	}
}

func (p *Page) HandleKey(ev *tcell.EventKey) bool {
	if p.askPicker.Active() {
		return p.askPicker.HandleKey(ev)
	}
	if ev.Key() == tcell.KeyTab {
		if p.attachmentBar.Count() > 0 {
			switch p.focus {
			case focusInput:
				p.focus = focusAttachments
				p.attachmentBar.Focus()
				p.input.Blur()
			case focusAttachments:
				p.focus = focusInput
				p.attachmentBar.Blur()
				p.input.Focus()
			}
			return true
		}
		if p.focus == focusAttachments {
			p.attachmentBar.Blur()
			p.focus = focusInput
			p.input.Focus()
		}
		return p.input.HandleKey(ev)
	}
	if ev.Key() == tcell.KeyEsc {
		if p.chat.HasBack() && p.chat.HandleKey(ev) {
			return true
		}
		if p.focus == focusAttachments {
			p.attachmentBar.Blur()
			p.focus = focusInput
			p.input.Focus()
			return true
		}
	}
	if p.focus == focusAttachments {
		handled := p.attachmentBar.HandleKey(ev)
		if p.attachmentBar.Count() == 0 {
			p.attachmentBar.Blur()
			p.focus = focusInput
			p.input.Focus()
		}
		return handled
	}

	atTokAt, _, atActive := p.input.AtToken()

	if atActive && len(p.mentionPicker.All()) > 0 && !p.pickDismissed && p.pickKind != pickSkills {
		p.pickKind = pickMention
		switch ev.Key() {
		case tcell.KeyUp, tcell.KeyDown:
			return p.mentionPicker.HandleKey(ev)
		case tcell.KeyEnter, tcell.KeyTab:
			if it, ok := p.mentionPicker.Selected(); ok {
				p.input.ReplaceMention(it.Label)
				p.pickKind = pickNone
				p.pickDismissed = true
				p.pickTokenAt = atTokAt
				return true
			}
			p.pickKind = pickNone
			p.pickDismissed = true
			p.pickTokenAt = atTokAt
			return true
		case tcell.KeyEsc:
			p.pickKind = pickNone
			p.pickDismissed = true
			p.pickTokenAt = atTokAt
			return true
		case tcell.KeyRune:
			if ev.Rune() == ' ' {
				p.pickKind = pickNone
				p.pickDismissed = true
				p.pickTokenAt = atTokAt
			}
		}
		handled := p.input.HandleKey(ev)
		_, q, _ := p.input.AtToken()
		p.mentionPicker.Update(q)
		return handled
	}

	slashTokAt, _, slashActive := p.input.SlashToken()
	if slashActive && len(p.skillPicker.All()) > 0 && !p.pickDismissed && p.pickKind != pickMention {
		p.pickKind = pickSkills
		switch ev.Key() {
		case tcell.KeyUp, tcell.KeyDown:
			return p.skillPicker.HandleKey(ev)
		case tcell.KeyEnter, tcell.KeyTab:
			if it, ok := p.skillPicker.Selected(); ok {
				p.input.ReplaceSkill(it.Label)
				p.pickKind = pickNone
				p.pickDismissed = true
				p.pickTokenAt = slashTokAt
				return true
			}
			p.pickKind = pickNone
			p.pickDismissed = true
			p.pickTokenAt = slashTokAt
			return true
		case tcell.KeyEsc:
			p.pickKind = pickNone
			p.pickDismissed = true
			p.pickTokenAt = slashTokAt
			return true
		case tcell.KeyRune:
			if ev.Rune() == ' ' {
				p.pickKind = pickNone
				p.pickDismissed = true
				p.pickTokenAt = slashTokAt
			}
		}
		handled := p.input.HandleKey(ev)
		_, q, _ := p.input.SlashToken()
		p.skillPicker.Update(q)
		return handled
	}
	p.syncPickers()

	switch ev.Key() {
	case tcell.KeyPgUp:
		if p.chat.HasItems() {
			p.chat.ScrollUp(15)
			return true
		}
	case tcell.KeyPgDn:
		if p.chat.HasItems() {
			p.chat.ScrollDown(15)
			return true
		}
	}
	return p.input.HandleKey(ev)
}

func (p *Page) syncPickers() {
	atAt, _, atActive := p.input.AtToken()
	slashAt, _, slashActive := p.input.SlashToken()

	if !atActive && !slashActive {
		p.pickKind = pickNone
		p.pickDismissed = false
		p.pickTokenAt = -1
		return
	}

	var newKind pickKind
	var tokAt int
	if atActive {
		newKind = pickMention
		tokAt = atAt
	} else {
		newKind = pickSkills
		tokAt = slashAt
	}

	if tokAt != p.pickTokenAt {
		p.pickDismissed = false
		p.pickTokenAt = tokAt
	}

	if p.pickDismissed || p.pickKind != pickNone {
		return
	}

	switch newKind {
	case pickMention:
		if len(p.mentionPicker.All()) > 0 {
			p.pickKind = pickMention
			_, q, _ := p.input.AtToken()
			p.mentionPicker.Update(q)
		}
	case pickSkills:
		if len(p.skillPicker.All()) > 0 {
			p.pickKind = pickSkills
			_, q, _ := p.input.SlashToken()
			p.skillPicker.Update(q)
		}
	}
}

func (p *Page) OnTick(blinkOn bool) {
	p.input.SetBlink(blinkOn)
	p.logo.SetBlink(p.input.CursorVisible(time.Now()))
	p.chat.OnTick(blinkOn)
}

func (p *Page) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	th := styles.Current()
	Fill(s, bounds, th)

	statusRegion := layout.BottomAligned(bounds, 1)

	innerW := bounds.Width - 4
	if innerW < 1 {
		innerW = 1
	}
	p.input.SetInnerWidth(innerW)
	inputH := p.input.VisibleRows() + 2
	if p.askPicker.Active() {
		inputH = p.askPicker.Height()
	}
	inputRegion := layout.Region{
		Left:   bounds.Left + 2,
		Top:    statusRegion.Top - inputH - 1,
		Width:  bounds.Width - 4,
		Height: inputH,
	}.Clamp(bounds)

	attachH := p.attachmentBar.RequiredHeight()
	if p.askPicker.Active() {
		attachH = 0
	}
	attachRegion := layout.Region{}
	if attachH > 0 {
		attachRegion = layout.Region{
			Left:   bounds.Left + 2,
			Top:    inputRegion.Top - attachH - 1,
			Width:  bounds.Width - 4,
			Height: attachH,
		}.Clamp(bounds)
	}

	chatBottom := inputRegion.Top - 1
	if attachH > 0 {
		chatBottom = attachRegion.Top - 1
	}

	topArea := layout.Region{
		Left:   bounds.Left,
		Top:    bounds.Top,
		Width:  bounds.Width,
		Height: chatBottom - bounds.Top,
	}

	if p.chat.HasItems() {
		chatRegion := layout.Region{
			Left:   topArea.Left + 1,
			Top:    topArea.Top + 1,
			Width:  topArea.Width - 2,
			Height: topArea.Height - 2,
		}
		p.chat.Draw(s, chatRegion, false)
	} else {
		logoRegion := layout.CenterIn(topArea, p.logo.Width, p.logo.Height)
		p.logo.Draw(s, logoRegion, false)
	}

	if attachH > 0 {
		p.attachmentBar.Draw(s, attachRegion, focused && p.focus == focusAttachments)
	}

	if p.askPicker.Active() {
		p.askPicker.Draw(s, inputRegion)
	} else {
		p.input.Draw(s, inputRegion, focused && p.focus == focusInput)
	}

	p.drawHint(s, bounds, inputRegion)

	p.syncPickers()
	if p.pickKind != pickNone && !p.askPicker.Active() {
		var count int
		switch p.pickKind {
		case pickSkills:
			count = p.skillPicker.Count()
		case pickMention:
			count = p.mentionPicker.Count()
		}
		pickH := count + 4
		if pickH > 13 {
			pickH = 13
		}
		if count == 0 {
			pickH = 4
		}
		pickW := inputRegion.Width
		if pickW < 24 {
			pickW = 24
		}
		pickRegion := layout.Region{
			Left:   bounds.Left + 2,
			Top:    inputRegion.Top - pickH - 1,
			Width:  pickW,
			Height: pickH,
		}.Clamp(bounds)
		if pickRegion.Top >= bounds.Top {
			switch p.pickKind {
			case pickSkills:
				p.skillPicker.Draw(s, pickRegion)
			case pickMention:
				p.mentionPicker.Draw(s, pickRegion)
			}
		}
	}
	p.status.Draw(s, statusRegion, true)
}

func (p *Page) drawHint(s tcell.Screen, bounds, inputRegion layout.Region) {
	if p.hint == "" || !p.input.Empty() || p.chat.HasItems() || p.attachmentBar.Count() > 0 || p.askPicker.Active() {
		return
	}
	y := inputRegion.Top - 1
	if y < bounds.Top || y >= bounds.Bottom() {
		return
	}
	const bullet = "• "
	textW := components.DisplayWidth(bullet) + components.DisplayWidth(p.hint)
	if textW > bounds.Width {
		return
	}
	x := bounds.Left + (bounds.Width-textW)/2
	th := styles.Current()
	components.DrawText(s, x, y, bullet, th.Base().Foreground(th.AccentDim).Background(th.Background))
	components.DrawText(s, x+components.DisplayWidth(bullet), y, p.hint, th.Base().Foreground(th.Muted).Background(th.Background))
}

func Fill(s tcell.Screen, bounds layout.Region, th styles.Theme) {
	components.FillRegion(s, bounds, ' ', th.Base().Background(th.Background))
}
