package settings

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
)

type rulesTab struct {
	settings *Settings
	list     *components.List
}

func newRules(s *Settings) *rulesTab {
	t := &rulesTab{settings: s, list: components.NewList("Rules")}
	t.rebuild()
	return t
}

func (t *rulesTab) rebuild() {
	var items []components.ListItem

	globalDir, _ := config.GetConfigPath("rules")
	projectDir, _ := config.GetProjectConfigPath("rules")

	globalRules := listRuleFiles(globalDir)
	for _, name := range globalRules {
		items = append(items, components.ListItem{
			Label:  name,
			Detail: "global",
			Data:   filepath.Join(globalDir, name),
		})
	}

	projectRules := listRuleFiles(projectDir)
	for _, name := range projectRules {
		items = append(items, components.ListItem{
			Label:  name,
			Detail: "project",
			Data:   filepath.Join(projectDir, name),
		})
	}

	if len(items) == 0 {
		items = append(items, components.ListItem{Label: "(no rules found)"})
	}
	t.list.SetItems(items)
}

func listRuleFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func (t *rulesTab) HandleKey(ev *tcell.EventKey) bool {
	return t.list.HandleKey(ev)
}

func (t *rulesTab) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	return t.list.HandleMouse(x, y, buttons)
}

func (t *rulesTab) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	t.list.Draw(s, bounds, focused)
}
