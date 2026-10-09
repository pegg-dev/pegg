package settings

import (
	"context"
	"runtime"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/update"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type systemTab struct {
	settings *Settings
	index    int
	status   string
	checking bool
}

func newSystem(s *Settings) *systemTab { return &systemTab{settings: s} }

const systemRowCount = 5

func (t *systemTab) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		if t.index > 0 {
			t.index--
			return true
		}
		return false
	case tcell.KeyDown:
		if t.index < systemRowCount-1 {
			t.index++
			return true
		}
		return false
	case tcell.KeyEnter:
		if t.index == systemRowCount-1 && !t.checking {
			t.checkUpdate()
			return true
		}
	}
	return false
}

func (t *systemTab) HandleMouse(x, y int, bounds layout.Region, buttons tcell.ButtonMask) bool {
	idx := y - bounds.Top
	if idx < 0 || idx >= systemRowCount {
		return false
	}
	t.index = idx
	if buttons&tcell.ButtonPrimary != 0 && idx == systemRowCount-1 && !t.checking {
		t.checkUpdate()
	}
	return true
}

func (t *systemTab) checkUpdate() {
	t.checking = true
	t.status = "Checking..."
	if t.settings.requestRedraw != nil {
		t.settings.requestRedraw()
	}

	go func() {
		current := config.AppVersion
		rel, found, err := update.DetectLatest(context.Background())
		if err != nil {
			t.status = "Error: " + err.Error()
			t.checking = false
			if t.settings.requestRedraw != nil {
				t.settings.requestRedraw()
			}
			return
		}
		if !found {
			t.status = "No release found"
			t.checking = false
			if t.settings.requestRedraw != nil {
				t.settings.requestRedraw()
			}
			return
		}
		if !update.IsNewerThan(current, rel.Version) {
			t.status = "Already up to date"
			t.checking = false
			if t.settings.requestRedraw != nil {
				t.settings.requestRedraw()
			}
			return
		}
		t.status = ""
		t.checking = false
		if t.settings.onRequestUpdate != nil {
			t.settings.onRequestUpdate()
		}
	}()
}

type sysRow struct {
	label string
	value string
}

func (t *systemTab) rows() []sysRow {
	return []sysRow{
		{label: "App", value: config.AppName},
		{label: "Version", value: config.AppVersion},
		{label: "OS", value: runtime.GOOS},
		{label: "Arch", value: runtime.GOARCH},
		{label: "Update", value: "check for updates"},
	}
}

func (t *systemTab) Draw(screen tcell.Screen, bounds layout.Region, _ bool) {
	th := styles.Current()
	rows := t.rows()
	for i, r := range rows {
		y := bounds.Top + i
		style := th.Base().Background(th.InputBg)
		marker := "  "
		if i == t.index {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		}
		if i == systemRowCount-1 {
			components.DrawText(screen, bounds.Left+1, y, marker+r.label, style)
			btn := "[ Check for updates ]"
			components.DrawText(screen, bounds.Right()-len(btn)-2, y, btn, style)
		} else {
			components.DrawText(screen, bounds.Left+1, y, marker+r.label, style)
			components.DrawText(screen, bounds.Right()-len(r.value)-3, y, r.value, style)
		}
	}
	if t.status != "" {
		y := bounds.Top + systemRowCount
		statusStyle := th.Base().Foreground(th.Placeholder).Background(th.InputBg)
		components.DrawText(screen, bounds.Left+1, y, t.status, statusStyle)
	}
}
