package settings

import (
	"fmt"
	"math"
	"sort"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/agent/middlewares"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/builtin/middlewares/permission"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
	"github.com/peggco/pegg/internal/tui/styles"
)

type permPreset int

const (
	presetCustom permPreset = iota
	presetAuto
	presetManual
	presetJudge
	presetRecommended
)

var presetNames = []string{"Custom", "Auto", "Manual", "Judge", "Recommended"}

var presetModes = map[permPreset]map[string]string{
	presetAuto: {
		"read": "allow", "write": "allow", "edit": "allow", "delete": "allow",
		"list": "allow", "glob": "allow", "grep": "allow", "bash": "allow",
		"todo": "allow", "todoread": "allow", "todowrite": "allow",
		"webfetch": "allow", "websearch": "allow", "task": "allow", "taskstatus": "allow",
		"askuserquestion": "allow", "enterplanmode": "allow", "exitplanmode": "allow",
	},
	presetManual: {
		"read": "semi-ask", "write": "semi-ask", "edit": "semi-ask", "delete": "semi-ask",
		"list": "semi-ask", "glob": "semi-ask", "grep": "semi-ask", "bash": "semi-ask",
		"todo": "semi-ask", "todoread": "semi-ask", "todowrite": "semi-ask",
		"webfetch": "semi-ask", "websearch": "semi-ask", "task": "semi-ask", "taskstatus": "semi-ask",
		"askuserquestion": "semi-ask", "enterplanmode": "semi-ask", "exitplanmode": "semi-ask",
	},
	presetJudge: {
		"read": "semi-judge", "write": "semi-judge", "edit": "semi-judge", "delete": "semi-judge",
		"list": "semi-judge", "glob": "semi-judge", "grep": "semi-judge", "bash": "semi-judge",
		"todo": "semi-judge", "todoread": "semi-judge", "todowrite": "semi-judge",
		"webfetch": "semi-judge", "websearch": "semi-judge", "task": "semi-judge", "taskstatus": "semi-judge",
		"askuserquestion": "semi-judge", "enterplanmode": "semi-judge", "exitplanmode": "semi-judge",
	},
}

var allModes = []string{"allow", "semi-ask", "ask", "semi-judge", "judge"}

type permFocus int

const (
	permFocusPresets permFocus = iota
	permFocusModel
	permFocusThreshold
	permFocusTools
)

type toolPerm struct {
	name string
	mode string
}

type judgePick struct {
	provider string
	model    string
}

type permissionsTab struct {
	settings  *Settings
	focus     permFocus
	toolIdx   int
	scroll    int
	tools     []toolPerm
	presetIdx int
	loaded    bool

	judgeProvider  string
	judgeModel     string
	judgeThreshold float64
}

func newPermissions(s *Settings) *permissionsTab {
	return &permissionsTab{settings: s, focus: permFocusPresets}
}

func (t *permissionsTab) loadIfNeeded() {
	if t.loaded {
		return
	}
	t.loaded = true
	t.loadTools()
	t.loadJudge()
	t.presetIdx = int(t.detectPreset())
	t.toolIdx = 0
	t.scroll = 0
}

func (t *permissionsTab) loadJudge() {
	t.judgeProvider, t.judgeModel = "", ""
	t.judgeThreshold = config.DefaultJudgeThreshold
	if t.settings.deps.Config != nil && t.settings.deps.Config.Permission != nil {
		t.judgeProvider = t.settings.deps.Config.Permission.JudgeProvider
		t.judgeModel = t.settings.deps.Config.Permission.JudgeModel
		t.judgeThreshold = t.settings.deps.Config.Permission.JudgeThresholdValue()
	}
}

func (t *permissionsTab) decisionEnabled() bool {
	return t.judgeProvider != "" && decision.HasProvider(t.judgeProvider)
}

func (t *permissionsTab) HandleKey(ev *tcell.EventKey) bool {
	t.loadIfNeeded()

	switch ev.Key() {
	case tcell.KeyUp:
		if t.focus == permFocusTools {
			if t.toolIdx > 0 {
				t.toolIdx--
			} else {
				t.focus = permFocusThreshold
			}
		} else if t.focus == permFocusThreshold {
			t.focus = permFocusModel
		} else if t.focus == permFocusModel {
			t.focus = permFocusPresets
		} else {
			return false
		}
		return true

	case tcell.KeyDown:
		if t.focus == permFocusPresets {
			t.focus = permFocusModel
		} else if t.focus == permFocusModel {
			t.focus = permFocusThreshold
		} else if t.focus == permFocusThreshold {
			t.focus = permFocusTools
		} else {
			if t.toolIdx < len(t.tools)-1 {
				t.toolIdx++
			}
		}
		return true

	case tcell.KeyEnter:
		if t.focus == permFocusModel {
			t.openJudgeModels()
			return true
		}
		return false

	case tcell.KeyLeft:
		if t.focus == permFocusPresets {
			t.presetIdx = (t.presetIdx - 1 + len(presetNames)) % len(presetNames)
			t.applyPreset(permPreset(t.presetIdx))
		} else if t.focus == permFocusThreshold {
			if t.decisionEnabled() {
				t.adjustThreshold(-0.05)
			}
		} else {
			if t.toolIdx >= 0 && t.toolIdx < len(t.tools) {
				t.cycleMode(t.toolIdx)
			}
		}
		return true

	case tcell.KeyRight:
		if t.focus == permFocusPresets {
			t.presetIdx = (t.presetIdx + 1) % len(presetNames)
			t.applyPreset(permPreset(t.presetIdx))
		} else if t.focus == permFocusThreshold {
			if t.decisionEnabled() {
				t.adjustThreshold(0.05)
			}
		} else {
			if t.toolIdx >= 0 && t.toolIdx < len(t.tools) {
				t.cycleModeRev(t.toolIdx)
			}
		}
		return true

	case tcell.KeyTab:
		return false

	case tcell.KeyPgUp:
		t.toolIdx -= 10
		if t.toolIdx < 0 {
			t.toolIdx = 0
		}
		return true
	case tcell.KeyPgDn:
		t.toolIdx += 10
		if t.toolIdx >= len(t.tools) {
			t.toolIdx = len(t.tools) - 1
		}
		return true
	case tcell.KeyHome:
		t.toolIdx = 0
		return true
	case tcell.KeyEnd:
		if len(t.tools) > 0 {
			t.toolIdx = len(t.tools) - 1
		}
		return true

	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h':
			if t.focus == permFocusPresets {
				t.presetIdx = (t.presetIdx - 1 + len(presetNames)) % len(presetNames)
				t.applyPreset(permPreset(t.presetIdx))
			} else if t.focus == permFocusThreshold {
				if t.decisionEnabled() {
					t.adjustThreshold(-0.05)
				}
			} else if t.toolIdx >= 0 && t.toolIdx < len(t.tools) {
				t.cycleMode(t.toolIdx)
			}
			return true
		case 'l':
			if t.focus == permFocusPresets {
				t.presetIdx = (t.presetIdx + 1) % len(presetNames)
				t.applyPreset(permPreset(t.presetIdx))
			} else if t.focus == permFocusThreshold {
				if t.decisionEnabled() {
					t.adjustThreshold(0.05)
				}
			} else if t.toolIdx >= 0 && t.toolIdx < len(t.tools) {
				t.cycleModeRev(t.toolIdx)
			}
			return true
		}
	}

	return false
}

func (t *permissionsTab) HandleMouse(x, y int, bounds layout.Region, buttons tcell.ButtonMask) bool {
	t.loadIfNeeded()

	if buttons&tcell.WheelUp != 0 || buttons&tcell.WheelDown != 0 {
		if len(t.tools) == 0 {
			return true
		}
		t.focus = permFocusTools
		delta := 1
		if buttons&tcell.WheelUp != 0 {
			delta = -1
		}
		t.toolIdx += delta
		if t.toolIdx < 0 {
			t.toolIdx = 0
		}
		if t.toolIdx >= len(t.tools) {
			t.toolIdx = len(t.tools) - 1
		}
		return true
	}

	row := y - bounds.Top
	switch row {
	case 0:
		t.focus = permFocusPresets
		if buttons&tcell.ButtonPrimary != 0 {
			if x < bounds.Left+bounds.Width/3 {
				t.presetIdx = (t.presetIdx - 1 + len(presetNames)) % len(presetNames)
			} else if x > bounds.Right()-bounds.Width/3 {
				t.presetIdx = (t.presetIdx + 1) % len(presetNames)
			} else {
				return true
			}
			t.applyPreset(permPreset(t.presetIdx))
		}
		return true
	case 1:
		t.focus = permFocusModel
		if buttons&tcell.ButtonPrimary != 0 {
			t.openJudgeModels()
		}
		return true
	case 2:
		t.focus = permFocusThreshold
		return true
	}

	listTop := bounds.Top + 6
	if y >= listTop && y < bounds.Bottom()-1 {
		t.focus = permFocusTools
		idx := t.scroll + (y - listTop)
		if idx >= 0 && idx < len(t.tools) {
			t.toolIdx = idx
			if buttons&tcell.ButtonPrimary != 0 && x > bounds.Right()-14 {
				t.cycleMode(idx)
			}
		}
		return true
	}
	return false
}

func (t *permissionsTab) Draw(screen tcell.Screen, bounds layout.Region, focused bool) {
	t.loadIfNeeded()
	th := styles.Current()

	presetY := bounds.Top
	presetLabel := fmt.Sprintf("  %s  ", presetNames[t.presetIdx])

	presetFocused := focused && t.focus == permFocusPresets
	presetStyle := th.Base().Foreground(th.Accent).Background(th.InputBg)
	if presetFocused {
		presetStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
	}

	arrowLeft := "◄ "
	components.DrawText(screen, bounds.Left+2, presetY, arrowLeft, presetStyle)
	presetX := bounds.Left + (bounds.Width-len(presetLabel))/2
	components.DrawText(screen, presetX, presetY, presetLabel, presetStyle)
	arrowRight := " ►"
	components.DrawText(screen, bounds.Right()-4, presetY, arrowRight, presetStyle)

	modelY := presetY + 1
	modelFocused := focused && t.focus == permFocusModel
	modelStyle := th.Base().Foreground(th.Accent).Background(th.InputBg)
	if modelFocused {
		modelStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
	}
	components.DrawText(screen, bounds.Left+2, modelY, "Judge model", modelStyle)
	judgeVal := t.judgeDisplay()
	components.DrawText(screen, bounds.Right()-len(judgeVal)-3, modelY, judgeVal, modelStyle)

	thresholdY := modelY + 1
	decEnabled := t.decisionEnabled()
	thresholdFocused := focused && t.focus == permFocusThreshold
	thresholdStyle := th.Base().Foreground(th.Accent).Background(th.InputBg)
	if thresholdFocused && decEnabled {
		thresholdStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
	} else if !decEnabled {
		thresholdStyle = th.Base().Foreground(th.Placeholder).Background(th.InputBg)
	}
	label := "Threshold"
	if !decEnabled {
		label += " (decision only)"
	}
	components.DrawText(screen, bounds.Left+2, thresholdY, label, thresholdStyle)
	thrVal := t.thresholdDisplay()
	components.DrawText(screen, bounds.Right()-len(thrVal)-3, thresholdY, thrVal, thresholdStyle)

	sepY := thresholdY + 1
	sepStyle := th.Base().Foreground(th.Muted).Background(th.InputBg)
	for x := bounds.Left + 1; x < bounds.Right()-1; x++ {
		screen.SetContent(x, sepY, '─', nil, sepStyle)
	}

	listTop := sepY + 2
	listBottom := bounds.Bottom() - 1
	visible := listBottom - listTop

	if t.toolIdx < t.scroll {
		t.scroll = t.toolIdx
	}
	if t.toolIdx >= t.scroll+visible {
		t.scroll = t.toolIdx - visible + 1
	}
	if t.scroll < 0 {
		t.scroll = 0
	}

	headerStyle := th.Base().Foreground(th.Muted).Background(th.InputBg)
	components.DrawText(screen, bounds.Left+2, listTop, "Tool", headerStyle)
	components.DrawText(screen, bounds.Right()-14, listTop, "Mode", headerStyle)
	listTop++

	for i := 0; i < visible-1; i++ {
		idx := t.scroll + i
		if idx < 0 || idx >= len(t.tools) {
			break
		}
		tp := t.tools[idx]
		y := listTop + i

		toolFocused := focused && t.focus == permFocusTools && idx == t.toolIdx
		rowStyle := th.Base().Background(th.InputBg)
		if toolFocused {
			rowStyle = th.Base().Foreground(th.InputText).Background(th.Selection)
		}

		components.DrawText(screen, bounds.Left+2, y, components.TruncateTo(tp.name, bounds.Width-20), rowStyle)
		modeDisplay := tp.mode
		components.DrawText(screen, bounds.Right()-len(modeDisplay)-3, y, modeDisplay, rowStyle)
	}
}

func (t *permissionsTab) loadTools() {
	cfg := t.settings.deps.Config
	names := tools.Names()
	sort.Strings(names)

	t.tools = make([]toolPerm, 0, len(names))
	for _, name := range names {
		mode := t.resolveMode(cfg, name)
		t.tools = append(t.tools, toolPerm{name: name, mode: mode})
	}
}

func (t *permissionsTab) resolveMode(cfg *config.Config, name string) string {
	if cfg != nil && cfg.Permission != nil {
		if r, ok := cfg.Permission.Rules[name]; ok {
			return r
		}
		return cfg.Permission.Default
	}
	if mode, ok := config.DefaultConfig().Permission.Rules[name]; ok {
		return mode
	}
	return "semi-ask"
}

func (t *permissionsTab) detectPreset() permPreset {
	defaults := config.DefaultConfig().Permission.Rules
	if len(t.tools) == 0 {
		return presetCustom
	}
	for _, tp := range t.tools {
		defMode := "semi-ask"
		if mode, ok := defaults[tp.name]; ok {
			defMode = mode
		}
		if tp.mode != defMode {
			goto checkUniform
		}
	}
	return presetRecommended

checkUniform:
	for _, p := range []permPreset{presetAuto, presetManual, presetJudge} {
		expected := presetModes[p]
		matches := true
		for _, tp := range t.tools {
			exp, inPreset := expected[tp.name]
			if !inPreset {
				exp = "semi-ask"
				if v, ok := expected["read"]; ok {
					exp = v
				}
			}
			if tp.mode != exp {
				matches = false
				break
			}
		}
		if matches {
			return p
		}
	}
	return presetCustom
}

func (t *permissionsTab) applyPreset(p permPreset) {
	if p == presetRecommended {
		defaults := config.DefaultConfig().Permission.Rules
		for i := range t.tools {
			defMode := "semi-ask"
			if mode, ok := defaults[t.tools[i].name]; ok {
				defMode = mode
			}
			t.tools[i].mode = defMode
		}
		t.applyLive()
		return
	}
	expected, ok := presetModes[p]
	if !ok {
		return
	}
	for i := range t.tools {
		if exp, inPreset := expected[t.tools[i].name]; inPreset {
			t.tools[i].mode = exp
		} else {
			defVal := "semi-ask"
			if v, ok := expected["read"]; ok {
				defVal = v
			}
			t.tools[i].mode = defVal
		}
	}
	t.applyLive()
}

func (t *permissionsTab) cycleMode(idx int) {
	current := t.tools[idx].mode
	next := ""
	found := false
	for _, m := range allModes {
		if found {
			next = m
			break
		}
		if m == current {
			found = true
		}
	}
	if next == "" {
		next = allModes[0]
	}
	t.tools[idx].mode = next
	t.presetIdx = int(t.detectPreset())
	t.applyLive()
}

func (t *permissionsTab) cycleModeRev(idx int) {
	current := t.tools[idx].mode
	prev := allModes[len(allModes)-1]
	for i := len(allModes) - 2; i >= 0; i-- {
		if allModes[i] == current {
			break
		}
		prev = allModes[i]
	}
	t.tools[idx].mode = prev
	t.presetIdx = int(t.detectPreset())
	t.applyLive()
}

func (t *permissionsTab) judgeDisplay() string {
	if t.judgeProvider == "" {
		return "—"
	}
	if t.judgeModel != "" {
		return t.judgeProvider + "/" + t.judgeModel
	}
	return t.judgeProvider + " (default)"
}

func (t *permissionsTab) thresholdDisplay() string {
	return fmt.Sprintf("%.2f", t.judgeThreshold)
}

func (t *permissionsTab) adjustThreshold(delta float64) {
	next := math.Round((t.judgeThreshold+delta)*100) / 100
	if next < 0.05 {
		next = 0.05
	}
	if next > 1 {
		next = 1
	}
	t.judgeThreshold = next
	t.saveThreshold()
}

func (t *permissionsTab) saveThreshold() {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	perm := cfg.Permission
	if perm == nil {
		perm = &config.PermissionConfig{}
	}
	thr := t.judgeThreshold
	perm.JudgeThreshold = &thr
	cfg.Permission = perm
	_ = config.Save(cfg)
	if m, ok := middlewares.Get("permission"); ok {
		if p, ok := m.(*permission.Middleware); ok {
			p.UpdateConfig(perm)
		}
	}
	t.settings.deps.Config = cfg
}

func (t *permissionsTab) openJudgeModels() {
	l := components.NewList("Select judge model")
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
			marked := t.judgeProvider == p.Provider &&
				(t.judgeModel == model || (t.judgeModel == "" && model == decision.DefaultModel(p.Provider)))
			decisionItems = append(decisionItems, components.ListItem{
				Label:  model,
				Detail: p.Provider + " · decision",
				Marked: marked,
				Data:   judgePick{provider: p.Provider, model: model},
			})
			if t.judgeProvider == p.Provider && t.judgeModel != "" && t.judgeModel != model {
				decisionItems = append(decisionItems, components.ListItem{
					Label:  t.judgeModel,
					Detail: p.Provider + " · decision",
					Marked: true,
					Data:   judgePick{provider: p.Provider, model: t.judgeModel},
				})
			}
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
					Marked: t.judgeProvider == p.Provider && t.judgeModel == m.ID,
					Data:   judgePick{provider: p.Provider, model: m.ID},
				})
			}
		}
	}

	if len(decisionItems) > 0 {
		items = append(items, components.ListItem{Label: "Decisions", Disabled: true})
		items = append(items, decisionItems...)
	}
	if len(llmItems) > 0 {
		items = append(items, components.ListItem{Label: "LLM models", Disabled: true})
		items = append(items, llmItems...)
	}
	if len(items) == 0 {
		items = append(items, components.ListItem{Label: "(no models available)"})
	}

	l.SetItems(items)
	l.SetOnSelect(func(_ int, item components.ListItem) {
		if item.Disabled {
			return
		}
		if pick, ok := item.Data.(judgePick); ok {
			t.setJudgeModel(pick.provider, pick.model)
			return
		}
		t.settings.back()
	})
	t.settings.openSub(&listModal{title: "Judge Model", list: l, onBack: t.settings.back})
}

func (t *permissionsTab) setJudgeModel(provider, model string) {
	cfg, err := config.Load()
	if err != nil {
		t.settings.errMsg = "failed to load config: " + err.Error()
		t.settings.back()
		return
	}
	perm := cfg.Permission
	if perm == nil {
		perm = config.DefaultConfig().Permission
	}
	perm.JudgeProvider = provider
	perm.JudgeModel = model
	cfg.Permission = perm
	if err := config.Save(cfg); err != nil {
		t.settings.errMsg = "failed to save config: " + err.Error()
		t.settings.back()
		return
	}
	t.settings.errMsg = ""
	t.settings.deps.Config = cfg
	t.judgeProvider = provider
	t.judgeModel = model
	if m, ok := middlewares.Get("permission"); ok {
		if p, ok := m.(*permission.Middleware); ok {
			p.UpdateConfig(perm)
		}
	}
	t.settings.back()
}

func (t *permissionsTab) applyLive() {
	rules := make(map[string]string, len(t.tools))
	for _, tp := range t.tools {
		rules[tp.name] = tp.mode
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	perm := cfg.Permission
	if perm == nil {
		perm = &config.PermissionConfig{}
	}
	perm.Default = "semi-ask"
	perm.Rules = rules
	cfg.Permission = perm
	_ = config.Save(cfg)
	if m, ok := middlewares.Get("permission"); ok {
		if p, ok := m.(*permission.Middleware); ok {
			p.UpdateConfig(perm)
		}
	}
	t.settings.deps.Config = cfg
}
