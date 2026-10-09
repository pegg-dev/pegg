package tui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type updateState int

const (
	updateStateConfirm updateState = iota
	updateStateUpdating
	updateStateDone
	updateStateError
)

type UpdateModal struct {
	version       string
	state         updateState
	errMsg        string
	onUpdate      func() error
	onSkip        func()
	onQuit        func()
	requestRedraw func()
	selected      int
}

func NewUpdateModal(version string, onUpdate func() error, onSkip func()) *UpdateModal {
	return &UpdateModal{
		version:  version,
		state:    updateStateConfirm,
		onUpdate: onUpdate,
		onSkip:   onSkip,
		selected: 0,
	}
}

func (m *UpdateModal) SetRedraw(fn func()) {
	m.requestRedraw = fn
}

func (m *UpdateModal) HandleKey(ev *tcell.EventKey) bool {
	switch m.state {
	case updateStateConfirm:
		switch ev.Key() {
		case tcell.KeyLeft, tcell.KeyRight:
			m.selected = (m.selected + 1) % 2
			return true
		case tcell.KeyEnter:
			if m.selected == 0 && m.onUpdate != nil {
				m.state = updateStateUpdating
				go m.runUpdate()
			} else if m.selected == 1 && m.onSkip != nil {
				m.onSkip()
			}
			return true
		case tcell.KeyEsc:
			if m.onSkip != nil {
				m.onSkip()
			}
			return true
		}
	case updateStateDone, updateStateError:
		switch ev.Key() {
		case tcell.KeyEnter, tcell.KeyEsc:
			if m.onQuit != nil {
				m.onQuit()
			}
			return true
		}
	}
	return false
}

func (m *UpdateModal) runUpdate() {
	err := m.onUpdate()
	if err != nil {
		m.errMsg = err.Error()
		m.state = updateStateError
	} else {
		m.state = updateStateDone
	}
	if m.requestRedraw != nil {
		m.requestRedraw()
	}
}

func (m *UpdateModal) SetQuit(fn func()) {
	m.onQuit = fn
}

func (m *UpdateModal) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	w, h := 50, 10
	if bounds.Width-8 < w {
		w = bounds.Width - 8
	}

	th := styles.Current()
	modalBg := th.Base().Background(th.InputBg)

	switch m.state {
	case updateStateConfirm:
		m.drawConfirm(s, bounds, w, h, th, modalBg)
	case updateStateUpdating:
		m.drawUpdating(s, bounds, w, h, th, modalBg)
	case updateStateDone:
		m.drawDone(s, bounds, w, h, th, modalBg)
	case updateStateError:
		m.drawError(s, bounds, w, h, th, modalBg)
	}
}

func (m *UpdateModal) drawConfirm(s tcell.Screen, bounds layout.Region, w, h int, th styles.Theme, modalBg tcell.Style) {
	inner := components.DrawCenteredBox(s, bounds, w, h, "Update Available")

	y := inner.Top + 1
	msg := "A new version (" + m.version + ") is available."
	components.DrawText(s, inner.Left, y, msg, modalBg.Foreground(th.Foreground))

	y += 2
	components.DrawText(s, inner.Left, y, "Do you want to update?", modalBg.Foreground(th.Foreground))

	y += 2
	updateLabel := "[ Update ]"
	skipLabel := "[ Later ]"

	if m.selected == 0 {
		components.DrawText(s, inner.Left+5, y, updateLabel, modalBg.Foreground(th.Accent))
		components.DrawText(s, inner.Left+25, y, skipLabel, modalBg.Foreground(th.Foreground))
	} else {
		components.DrawText(s, inner.Left+5, y, updateLabel, modalBg.Foreground(th.Foreground))
		components.DrawText(s, inner.Left+25, y, skipLabel, modalBg.Foreground(th.Accent))
	}

	components.DrawFooter(s, inner, "←/→ select  Enter confirm  Esc skip")
}

func (m *UpdateModal) drawUpdating(s tcell.Screen, bounds layout.Region, w, h int, th styles.Theme, modalBg tcell.Style) {
	inner := components.DrawCenteredBox(s, bounds, w, h, "Updating...")

	y := inner.Top + 2
	components.DrawText(s, inner.Left, y, "Downloading and installing...", modalBg.Foreground(th.Foreground))

	components.DrawFooter(s, inner, "Please wait")
}

func (m *UpdateModal) drawDone(s tcell.Screen, bounds layout.Region, w, h int, th styles.Theme, modalBg tcell.Style) {
	inner := components.DrawCenteredBox(s, bounds, w, h, "Update Complete")

	y := inner.Top + 1
	components.DrawText(s, inner.Left, y, "Updated to version "+m.version, modalBg.Foreground(th.Success))

	y += 2
	components.DrawText(s, inner.Left, y, "Please restart to use the new version.", modalBg.Foreground(th.Foreground))

	components.DrawFooter(s, inner, "Enter close")
}

func (m *UpdateModal) drawError(s tcell.Screen, bounds layout.Region, w, h int, th styles.Theme, modalBg tcell.Style) {
	inner := components.DrawCenteredBox(s, bounds, w, h, "Update Failed")

	y := inner.Top + 1
	components.DrawText(s, inner.Left, y, m.errMsg, modalBg.Foreground(th.Error))

	y += 2
	components.DrawText(s, inner.Left, y, "Please try again later.", modalBg.Foreground(th.Foreground))

	components.DrawFooter(s, inner, "Enter close")
}
