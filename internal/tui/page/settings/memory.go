package settings

import (
	"fmt"
	"math"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type memoryFocus int

const (
	memoryFocusEnabled memoryFocus = iota
	memoryFocusObserver
	memoryFocusGate
	memoryFocusThreshold
	memoryFocusBudget
	memoryFocusMaxResults
	memoryFocusConsolidate
	memoryFocusEntries
	memoryFocusLastEntry
	memoryFocusClear
)

type memoryPick struct {
	provider string
	model    string
}

type memoryTab struct {
	settings *Settings
	focus    memoryFocus

	enabled     bool
	provider    string
	model       string
	gate        string
	threshold   float64
	budget      int
	maxResults  int
	consolidate bool
	entries     int
	lastEntry   string
	loaded      bool
}

func newMemoryTab(s *Settings) *memoryTab {
	return &memoryTab{settings: s}
}

func (t *memoryTab) loadIfNeeded() {
	if t.loaded {
		return
	}
	t.loaded = true
	t.threshold = config.DefaultMemoryThreshold
	t.budget = config.DefaultMemoryBudget
	t.maxResults = config.DefaultMemoryMaxResults
	t.consolidate = true
	t.enabled = true
	t.gate = config.GateAuto
	if t.settings.deps.Config != nil && t.settings.deps.Config.Memory != nil {
		m := t.settings.deps.Config.Memory
		t.enabled = m.Enabled
		t.provider = m.Provider
		t.model = m.Model
		t.gate = m.GateValue()
		t.threshold = m.ThresholdValue()
		t.budget = m.BudgetValue()
		t.maxResults = m.MaxResultsValue()
		t.consolidate = m.ConsolidateValue()
	}
	t.loadStats()
}

func (t *memoryTab) loadStats() {
	t.entries = 0
	t.lastEntry = ""
	if t.settings.deps.Memory != nil {
		store := t.settings.deps.Memory.Store()
		if store != nil {
			count, lastDate, lastFile := store.Stats()
			t.entries = count
			if lastFile != "" {
				t.lastEntry = lastDate + " · " + lastFile
			}
		}
	}
}

func (t *memoryTab) decisionAvailable() bool {
	if t.settings.deps.Config == nil {
		return false
	}
	if t.provider != "" && decision.HasProvider(t.provider) {
		return true
	}
	for _, p := range t.settings.deps.Config.Providers {
		if decision.HasProvider(p.Provider) {
			return true
		}
	}
	return false
}

func (t *memoryTab) gateUsesThreshold() bool {
	if t.gate == config.GateDecision {
		return true
	}
	return t.gate == config.GateAuto && t.decisionAvailable()
}

func (t *memoryTab) cycleGate(dir int) {
	modes := config.GateModes()
	idx := 0
	for i, m := range modes {
		if m == t.gate {
			idx = i
			break
		}
	}
	t.gate = modes[(idx+dir+len(modes))%len(modes)]
}

func (t *memoryTab) HandleKey(ev *tcell.EventKey) bool {
	t.loadIfNeeded()

	switch ev.Key() {
	case tcell.KeyUp:
		if t.focus > memoryFocusEnabled {
			t.focus--
		} else {
			return false
		}
		return true
	case tcell.KeyDown:
		if t.focus < memoryFocusClear {
			t.focus++
		} else {
			return false
		}
		return true
	case tcell.KeyLeft:
		switch t.focus {
		case memoryFocusEnabled:
			t.enabled = !t.enabled
		case memoryFocusGate:
			t.cycleGate(-1)
		case memoryFocusThreshold:
			if t.gateUsesThreshold() {
				t.adjustThreshold(-0.05)
			}
		case memoryFocusBudget:
			t.adjustBudget(-500)
		case memoryFocusMaxResults:
			t.adjustMaxResults(-1)
		case memoryFocusConsolidate:
			t.consolidate = !t.consolidate
		case memoryFocusEntries, memoryFocusLastEntry:
			return true
		default:
			return false
		}
		t.save()
		return true
	case tcell.KeyRight:
		switch t.focus {
		case memoryFocusEnabled:
			t.enabled = !t.enabled
		case memoryFocusGate:
			t.cycleGate(1)
		case memoryFocusThreshold:
			if t.gateUsesThreshold() {
				t.adjustThreshold(0.05)
			}
		case memoryFocusBudget:
			t.adjustBudget(500)
		case memoryFocusMaxResults:
			t.adjustMaxResults(1)
		case memoryFocusConsolidate:
			t.consolidate = !t.consolidate
		case memoryFocusEntries, memoryFocusLastEntry:
			return true
		default:
			return false
		}
		t.save()
		return true
	case tcell.KeyEnter:
		switch t.focus {
		case memoryFocusEnabled:
			t.enabled = !t.enabled
			t.save()
		case memoryFocusObserver:
			t.openObserverModels()
		case memoryFocusGate:
			t.cycleGate(1)
			t.save()
		case memoryFocusConsolidate:
			t.consolidate = !t.consolidate
			t.save()
		case memoryFocusClear:
			t.openClearConfirm()
		}
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h':
			switch {
			case t.focus == memoryFocusGate:
				t.cycleGate(-1)
				t.save()
			case t.focus == memoryFocusThreshold && t.gateUsesThreshold():
				t.adjustThreshold(-0.05)
				t.save()
			}
			return true
		case 'l':
			switch {
			case t.focus == memoryFocusGate:
				t.cycleGate(1)
				t.save()
			case t.focus == memoryFocusThreshold && t.gateUsesThreshold():
				t.adjustThreshold(0.05)
				t.save()
			}
			return true
		}
	}
	return false
}

func (t *memoryTab) HandleMouse(x, y int, bounds layout.Region, buttons tcell.ButtonMask) bool {
	t.loadIfNeeded()
	row := y - bounds.Top
	wheel := buttons&(tcell.WheelUp|tcell.WheelDown) != 0
	up := buttons&tcell.WheelUp != 0

	adjust := func(f memoryFocus) bool {
		switch f {
		case memoryFocusEnabled:
			t.enabled = !t.enabled
		case memoryFocusGate:
			dir := 1
			if up {
				dir = -1
			}
			t.cycleGate(dir)
		case memoryFocusThreshold:
			if !t.gateUsesThreshold() {
				return false
			}
			if up {
				t.adjustThreshold(-0.05)
			} else {
				t.adjustThreshold(0.05)
			}
		case memoryFocusBudget:
			if up {
				t.adjustBudget(-500)
			} else {
				t.adjustBudget(500)
			}
		case memoryFocusMaxResults:
			if up {
				t.adjustMaxResults(-1)
			} else {
				t.adjustMaxResults(1)
			}
		case memoryFocusConsolidate:
			t.consolidate = !t.consolidate
		default:
			return false
		}
		t.save()
		return true
	}

	if row >= 0 && row < 7 {
		f := memoryFocus(row)
		t.focus = f
		if wheel {
			adjust(f)
			return true
		}
		if buttons&tcell.ButtonPrimary != 0 {
			if f == memoryFocusObserver {
				t.openObserverModels()
				return true
			}
			adjust(f)
		}
		return true
	}
	switch row {
	case 9:
		t.focus = memoryFocusEntries
		return true
	case 10:
		t.focus = memoryFocusLastEntry
		return true
	case 11:
		t.focus = memoryFocusClear
		if buttons&tcell.ButtonPrimary != 0 {
			t.openClearConfirm()
		}
		return true
	}
	return false
}

func (t *memoryTab) Draw(screen tcell.Screen, bounds layout.Region, focused bool) {
	t.loadIfNeeded()
	th := styles.Current()

	rows := []struct {
		label string
		value string
	}{
		{"Enabled", t.boolLabel(t.enabled)},
		{"Observer", t.observerDisplay()},
		{"Relevance gating", t.gateDisplay()},
		{"Relevance threshold", t.thresholdDisplay()},
		{"Context budget", fmt.Sprintf("%d chars", t.budget)},
		{"Max results", fmt.Sprintf("%d", t.maxResults)},
		{"Consolidate", t.boolLabel(t.consolidate)},
	}
	for i, r := range rows {
		y := bounds.Top + i
		f := memoryFocus(i)
		style := th.Base().Background(th.InputBg)
		marker := "  "
		if f == t.focus {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		} else if f == memoryFocusThreshold && !t.gateUsesThreshold() {
			style = th.Base().Foreground(th.Placeholder).Background(th.InputBg)
		}
		label := r.label
		if f == memoryFocusThreshold && !t.gateUsesThreshold() {
			label += " (decision only)"
		}
		components.DrawText(screen, bounds.Left+1, y, marker+label, style)
		if r.value != "" {
			components.DrawText(screen, bounds.Right()-len(r.value)-3, y, r.value, style)
		}
	}

	sepY := bounds.Top + len(rows) + 1
	sepStyle := th.Base().Foreground(th.Muted).Background(th.InputBg)
	components.DrawText(screen, bounds.Left+1, sepY, "── Memory ─────────────────────────────", sepStyle)

	stats := []struct {
		label string
		value string
		focus memoryFocus
	}{
		{"Entries", fmt.Sprintf("%d", t.entries), memoryFocusEntries},
		{"Last entry", t.lastEntry, memoryFocusLastEntry},
	}
	for i, s := range stats {
		y := sepY + 1 + i
		style := th.Base().Background(th.InputBg)
		marker := "  "
		if s.focus == t.focus {
			style = th.Base().Foreground(th.InputText).Background(th.Selection)
			marker = "> "
		}
		components.DrawText(screen, bounds.Left+1, y, marker+s.label, style)
		if s.value != "" {
			components.DrawText(screen, bounds.Right()-len(s.value)-3, y, s.value, style)
		}
	}

	clearY := sepY + 3
	clearStyle := th.Base().Background(th.InputBg)
	marker := "  "
	if t.focus == memoryFocusClear {
		clearStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
		marker = "> "
	}
	components.DrawText(screen, bounds.Left+1, clearY, marker+"Clear memory", clearStyle)
}

func (t *memoryTab) boolLabel(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (t *memoryTab) observerDisplay() string {
	if t.provider == "" {
		return "default (agent model)"
	}
	if t.model != "" {
		return t.provider + "/" + t.model
	}
	return t.provider + " (default)"
}

func (t *memoryTab) gateDisplay() string {
	switch t.gate {
	case config.GateDecision:
		return "decision (JEV)"
	case config.GateLLM:
		return "llm"
	case config.GateScore:
		return "score (free)"
	case config.GateOff:
		return "off"
	default:
		return "auto"
	}
}

func (t *memoryTab) thresholdDisplay() string {
	return fmt.Sprintf("%.2f", t.threshold)
}

func (t *memoryTab) adjustThreshold(delta float64) {
	next := math.Round((t.threshold+delta)*100) / 100
	if next < 0.05 {
		next = 0.05
	}
	if next > 1 {
		next = 1
	}
	t.threshold = next
}

func (t *memoryTab) adjustBudget(delta int) {
	next := t.budget + delta
	if next < 500 {
		next = 500
	}
	if next > 100000 {
		next = 100000
	}
	t.budget = next
}

func (t *memoryTab) adjustMaxResults(delta int) {
	next := t.maxResults + delta
	if next < 1 {
		next = 1
	}
	if next > 10 {
		next = 10
	}
	t.maxResults = next
}

func (t *memoryTab) save() {
	cfg := &config.MemoryConfig{
		Enabled:       t.enabled,
		Provider:      t.provider,
		Model:         t.model,
		Gate:          t.gate,
		ContextBudget: t.budget,
		MaxResults:    t.maxResults,
	}
	thr := t.threshold
	cfg.Threshold = &thr
	cons := t.consolidate
	cfg.Consolidate = &cons
	if err := config.UpsertMemory(cfg); err != nil {
		t.settings.errMsg = "failed to save memory config: " + err.Error()
		return
	}
	t.settings.errMsg = ""
	if t.settings.deps.Config != nil {
		t.settings.deps.Config.Memory = cfg
	}
	if t.settings.deps.Memory != nil {
		t.settings.deps.Memory.UpdateConfig(cfg)
	}
}

func (t *memoryTab) openObserverModels() {
	l := components.NewList("Select memory observer")
	l.SetSearchable(true)

	var items []components.ListItem
	var decisionItems []components.ListItem
	if t.settings.deps.Config != nil {
		for _, p := range t.settings.deps.Config.Providers {
			if p.APIKey == "" || !decision.HasProvider(p.Provider) {
				continue
			}
			model := decision.DefaultModel(p.Provider)
			if model == "" {
				model = "default"
			}
			decisionItems = append(decisionItems, components.ListItem{
				Label:  model,
				Detail: p.Provider + " · decision",
				Marked: t.provider == p.Provider && t.model == model,
				Data:   memoryPick{provider: p.Provider, model: model},
			})
		}
	}

	var llmItems []components.ListItem
	if t.settings.deps.LLM != nil && t.settings.deps.Config != nil {
		for _, p := range t.settings.deps.Config.Providers {
			models, err := t.settings.deps.LLM.Models(p.Provider)
			if err != nil {
				continue
			}
			for _, m := range models {
				label := m.ID
				if m.Name != "" {
					label = m.Name
				}
				llmItems = append(llmItems, components.ListItem{
					Label:  label,
					Detail: p.Provider,
					Marked: t.provider == p.Provider && t.model == m.ID,
					Data:   memoryPick{provider: p.Provider, model: m.ID},
				})
			}
		}
	}

	items = append(items, components.ListItem{
		Label:  "Default (agent model)",
		Detail: "use the running agent's model",
		Marked: t.provider == "",
		Data:   memoryPick{provider: "", model: ""},
	})
	if len(decisionItems) > 0 {
		items = append(items, components.ListItem{Label: "Decisions", Disabled: true})
		items = append(items, decisionItems...)
	}
	if len(llmItems) > 0 {
		items = append(items, components.ListItem{Label: "LLM models", Disabled: true})
		items = append(items, llmItems...)
	}
	if len(items) == 1 {
		items = append(items, components.ListItem{Label: "(no models available)"})
	}

	l.SetItems(items)
	l.SetOnSelect(func(_ int, item components.ListItem) {
		if item.Disabled {
			return
		}
		if pick, ok := item.Data.(memoryPick); ok {
			t.provider = pick.provider
			t.model = pick.model
			t.save()
		}
		t.settings.back()
	})
	t.settings.openSub(&listModal{title: "Memory Observer", list: l, onBack: t.settings.back})
}

func (t *memoryTab) openClearConfirm() {
	l := components.NewList("Clear memory")
	l.SetItems([]components.ListItem{
		{Label: "Clear", Detail: "delete all memory files for this project"},
		{Label: "Cancel", Detail: "keep the memory"},
	})
	l.SetOnSelect(func(i int, _ components.ListItem) {
		if i == 0 {
			t.clearMemory()
		} else {
			t.settings.back()
		}
	})
	t.settings.openSub(&listModal{title: "Confirm clear", list: l, onBack: t.settings.back})
}

func (t *memoryTab) clearMemory() {
	if t.settings.deps.Memory == nil {
		t.settings.errMsg = "memory store unavailable"
		t.settings.back()
		return
	}
	store := t.settings.deps.Memory.Store()
	if store == nil {
		t.settings.errMsg = "memory store unavailable"
		t.settings.back()
		return
	}
	if err := store.Clear(); err != nil {
		t.settings.errMsg = "failed to clear memory: " + err.Error()
	} else {
		t.settings.errMsg = ""
		t.loadStats()
	}
	t.settings.back()
}
