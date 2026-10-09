package settings

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
	"github.com/peggco/pegg/internal/utils/query"
)

func newSessionTab(s *Settings) *sessionTab { return &sessionTab{settings: s} }

type sessionTab struct {
	settings *Settings
	index    int

	compEnabled    bool
	compStrategyOn []bool
	compThresh     int
	compMaxMsg     int
	compMaxTool    int
	compLoaded     bool
}

var compStrategies = []string{"tool-clearing", "sliding-window", "summarization"}

const sessionRowCount = 4
const compSectionStart = sessionRowCount + 2
const compStrategyCount = 3
const compRowCount = 1 + compStrategyCount + 3

func (t *sessionTab) loadCompaction() {
	if t.compLoaded {
		return
	}
	t.compLoaded = true
	t.compStrategyOn = make([]bool, compStrategyCount)
	cfg := t.settings.deps.Config
	if cfg != nil && cfg.Compaction != nil {
		c := cfg.Compaction
		t.compEnabled = c.Enabled
		t.compThresh = int(c.Threshold)
		t.compMaxMsg = c.MaxMessages
		t.compMaxTool = c.MaxToolOutputChars
		for i, s := range compStrategies {
			for _, cs := range c.Strategy {
				if s == cs {
					t.compStrategyOn[i] = true
					break
				}
			}
		}
	} else {
		t.compEnabled = true
		t.compStrategyOn[0] = true
		t.compStrategyOn[1] = true
		t.compThresh = 80
		t.compMaxMsg = 50
		t.compMaxTool = 4000
	}
}

func (t *sessionTab) totalRows() int {
	return compSectionStart + compRowCount
}

func (t *sessionTab) rowEnabled(i int) bool {
	if i == 2 || i == 3 {
		return t.settings.active != nil
	}
	return true
}

func (t *sessionTab) HandleKey(ev *tcell.EventKey) bool {
	t.loadCompaction()
	switch ev.Key() {
	case tcell.KeyUp:
		if t.index > 0 {
			t.index--
			return true
		}
		return false
	case tcell.KeyDown:
		if t.index < t.totalRows()-1 {
			t.index++
			return true
		}
		return false
	case tcell.KeyLeft:
		if t.index >= compSectionStart {
			t.adjustComp(false)
			return true
		}
	case tcell.KeyRight:
		if t.index >= compSectionStart {
			t.adjustComp(true)
			return true
		}
	case tcell.KeyEnter:
		if t.index < sessionRowCount {
			if !t.rowEnabled(t.index) {
				return true
			}
			switch t.index {
			case 0:
				t.settings.openSessionList()
			case 1:
				t.settings.newSession()
			case 2:
				t.settings.openDeleteConfirm()
			case 3:
				t.settings.openTitle()
			}
			return true
		}
		if t.index >= compSectionStart {
			t.toggleCompRow()
			return true
		}
	}
	return false
}

func (t *sessionTab) HandleMouse(x, y int, bounds layout.Region, buttons tcell.ButtonMask) bool {
	t.loadCompaction()
	row := y - bounds.Top
	if row < 0 || row >= t.totalRows() {
		return false
	}
	if row < sessionRowCount || row >= compSectionStart {
		t.index = row
	}
	if buttons&tcell.ButtonPrimary == 0 {
		return true
	}
	if row < sessionRowCount {
		if !t.rowEnabled(row) {
			return true
		}
		switch row {
		case 0:
			t.settings.openSessionList()
		case 1:
			t.settings.newSession()
		case 2:
			t.settings.openDeleteConfirm()
		case 3:
			t.settings.openTitle()
		}
		return true
	}
	if row >= compSectionStart {
		t.toggleCompRow()
	}
	return true
}

func (t *sessionTab) adjustComp(right bool) {
	switch t.index {
	case compSectionStart:
		t.compEnabled = !t.compEnabled
	case compSectionStart + 1, compSectionStart + 2, compSectionStart + 3:
		i := t.index - compSectionStart - 1
		t.compStrategyOn[i] = !t.compStrategyOn[i]
	case compSectionStart + 4:
		dir := 1
		if !right {
			dir = -1
		}
		t.compThresh += dir * 5
		if t.compThresh < 10 {
			t.compThresh = 10
		}
		if t.compThresh > 100 {
			t.compThresh = 100
		}
	case compSectionStart + 5:
		dir := 1
		if !right {
			dir = -1
		}
		t.compMaxMsg += dir * 5
		if t.compMaxMsg < 5 {
			t.compMaxMsg = 5
		}
		if t.compMaxMsg > 200 {
			t.compMaxMsg = 200
		}
	case compSectionStart + 6:
		dir := 1
		if !right {
			dir = -1
		}
		t.compMaxTool += dir * 500
		if t.compMaxTool < 500 {
			t.compMaxTool = 500
		}
		if t.compMaxTool > 20000 {
			t.compMaxTool = 20000
		}
	}
	t.saveCompaction()
}

func (t *sessionTab) toggleCompRow() {
	switch t.index {
	case compSectionStart:
		t.compEnabled = !t.compEnabled
	case compSectionStart + 1, compSectionStart + 2, compSectionStart + 3:
		i := t.index - compSectionStart - 1
		t.compStrategyOn[i] = !t.compStrategyOn[i]
	default:
		return
	}
	t.saveCompaction()
}

func (t *sessionTab) saveCompaction() {
	strategy := []string{}
	for i, on := range t.compStrategyOn {
		if on {
			strategy = append(strategy, compStrategies[i])
		}
	}
	cfg := &config.CompactionConfig{
		Enabled:            t.compEnabled,
		Strategy:           strategy,
		Threshold:          float64(t.compThresh),
		MaxMessages:        t.compMaxMsg,
		MaxToolOutputChars: t.compMaxTool,
	}
	_ = config.UpsertCompaction(cfg)
	if t.settings.deps.Config != nil {
		t.settings.deps.Config.Compaction = cfg
	}
}

func (t *sessionTab) Draw(screen tcell.Screen, bounds layout.Region, _ bool) {
	t.loadCompaction()
	th := styles.Current()

	rows := []struct {
		label, value string
	}{
		{"Load session", t.activeValue()},
		{"New session", ""},
		{"Delete session", ""},
		{"Change title", ""},
	}
	for i, r := range rows {
		y := bounds.Top + i
		style := th.Base().Background(th.InputBg)
		marker := "  "
		enabled := t.rowEnabled(i)
		if i == t.index {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		} else if !enabled {
			style = th.Base().Foreground(th.Placeholder).Background(th.InputBg)
		}
		label := r.label
		if !enabled {
			label += " (no active session)"
		}
		components.DrawText(screen, bounds.Left+1, y, marker+label, style)
		if r.value != "" {
			components.DrawText(screen, bounds.Right()-len(r.value)-3, y, r.value, style)
		}
	}

	sepY := bounds.Top + sessionRowCount + 1
	sepStyle := th.Base().Foreground(th.Muted).Background(th.InputBg)
	components.DrawText(screen, bounds.Left+1, sepY, "── Compaction ──────────────────────────", sepStyle)

	compRows := []struct {
		label string
		value string
	}{
		{"Enabled", t.enabledLabel()},
	}
	for i, name := range compStrategies {
		cb := "[ ]"
		if t.compStrategyOn[i] {
			cb = "[x]"
		}
		compRows = append(compRows, struct {
			label string
			value string
		}{label: cb + " " + name})
	}
	compRows = append(compRows,
		[]struct {
			label string
			value string
		}{
			{"Threshold", fmt.Sprintf("%d%%", t.compThresh)},
			{"Max messages", fmt.Sprintf("%d", t.compMaxMsg)},
			{"Max tool output", fmt.Sprintf("%d chars", t.compMaxTool)},
		}...,
	)
	for i, r := range compRows {
		y := bounds.Top + compSectionStart + i
		rowIdx := compSectionStart + i
		style := th.Base().Background(th.InputBg)
		marker := "  "
		if rowIdx == t.index {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		}
		components.DrawText(screen, bounds.Left+1, y, marker+r.label, style)
		if r.value != "" {
			components.DrawText(screen, bounds.Right()-len(r.value)-3, y, r.value, style)
		}
	}
}

func (t *sessionTab) enabledLabel() string {
	if t.compEnabled {
		return "on"
	}
	return "off"
}

func (t *sessionTab) activeValue() string {
	if t.settings.active == nil {
		return "—"
	}
	return t.settings.active.Title
}

func (s *Settings) hasSessions() bool { return s.deps.Sessions != nil }

func (s *Settings) openSessionList() {
	if !s.hasSessions() {
		s.errMsg = "session store unavailable"
		return
	}
	l := components.NewList("Load session")

	var items []components.ListItem
	sessions, _, err := s.deps.Sessions.List(query.Query{
		Page: query.Page{Number: 1, Size: 200},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	})
	if err == nil {
		if dir, werr := os.Getwd(); werr == nil {
			var filtered []session.Session
			for _, sess := range sessions {
				if sess.ProjectDir == dir {
					filtered = append(filtered, sess)
				}
			}
			sessions = filtered
		}
		sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt) })
		for _, sess := range sessions {
			items = append(items, components.ListItem{
				Label: sess.Title,
				Data:  sess.ID,
			})
		}
	} else {
		s.errMsg = "failed to list sessions: " + err.Error()
	}
	if len(items) == 0 {
		items = append(items, components.ListItem{Label: "(no sessions in this project)"})
	}
	l.SetItems(items)
	l.SetOnSelect(func(_ int, item components.ListItem) {
		if id, ok := item.Data.(string); ok {
			s.loadSession(id)
		} else {
			s.back()
		}
	})
	s.openSub(&listModal{title: "Sessions", list: l, onBack: s.back})
}

func (s *Settings) loadSession(id string) {
	if !s.hasSessions() {
		s.errMsg = "session store unavailable"
		s.back()
		return
	}
	latest, err := s.deps.Sessions.LatestInChain(id)
	if err != nil {
		latest, err = s.deps.Sessions.Get(id)
		if err != nil {
			s.errMsg = "failed to load session: " + err.Error()
			s.back()
			return
		}
	}
	sess := latest
	msgs, err := s.deps.Sessions.Messages(sess.ID)
	if err != nil {
		s.errMsg = "failed to load messages: " + err.Error()
		s.back()
		return
	}
	info := SessionInfo{
		ID:                 sess.ID,
		Title:              sess.Title,
		Provider:           sess.Provider,
		Model:              sess.Model,
		ReasoningEffort:    sess.ReasoningEffort,
		CompactionParentID: sess.CompactionParentID,
		Messages:           msgs,
		Usage:              sess.Usage,
	}
	s.active = &info
	s.errMsg = ""
	if s.onSessionChange != nil {
		s.onSessionChange(info)
	}
	s.back()
	s.closeModal()
}

func (s *Settings) newSession() {
	s.active = nil
	s.errMsg = ""
	if s.onSessionClear != nil {
		s.onSessionClear()
	}
	s.back()
	s.closeModal()
}

func (s *Settings) closeModal() {
	if s.onClose != nil {
		s.onClose()
	}
}

func (s *Settings) openDeleteConfirm() {
	if s.active == nil {
		return
	}
	l := components.NewList("Delete session")
	title := s.active.Title
	if title == "" {
		title = "(untitled)"
	}
	l.SetItems([]components.ListItem{
		{Label: "Delete", Detail: "session \"" + title + "\" will be permanently removed"},
		{Label: "Cancel", Detail: "keep the session"},
	})
	l.SetOnSelect(func(i int, _ components.ListItem) {
		if i == 0 {
			s.deleteSession()
		} else {
			s.back()
		}
	})
	s.openSub(&listModal{title: "Confirm deletion", list: l, onBack: s.back})
}

func (s *Settings) deleteSession() {
	if s.active == nil || !s.hasSessions() {
		s.back()
		return
	}
	id := s.active.ID
	s.active = nil
	if err := s.deps.Sessions.Delete(id); err != nil {
		s.errMsg = "failed to delete session: " + err.Error()
	} else {
		s.errMsg = ""
	}
	if s.onSessionClear != nil {
		s.onSessionClear()
	}
	s.back()
}

func (s *Settings) openTitle() {
	if s.active == nil {
		return
	}
	f := components.NewField("Session title")
	f.SetPlaceholder("enter a title")
	f.SetText(s.active.Title)
	f.Focus()
	f.SetOnEnter(func(title string) { s.saveTitle(strings.TrimSpace(title)) })
	f.SetOnCancel(s.back)
	s.openSub(&fieldModal{title: "Change title", field: f})
}

func (s *Settings) saveTitle(title string) {
	if title == "" || s.active == nil || !s.hasSessions() {
		s.back()
		return
	}
	if err := s.deps.Sessions.SetTitle(s.active.ID, title); err != nil {
		s.errMsg = "failed to save title: " + err.Error()
		s.back()
		return
	}
	s.active.Title = title
	s.errMsg = ""
	if s.onSessionChange != nil {
		s.onSessionChange(*s.active)
	}
	s.back()
}
