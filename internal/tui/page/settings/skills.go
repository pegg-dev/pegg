package settings

import (
	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
)

type skillsTab struct {
	settings *Settings
	list     *components.List
}

func newSkills(s *Settings) *skillsTab {
	t := &skillsTab{settings: s, list: components.NewList("Skills")}
	t.rebuild()
	return t
}

func (t *skillsTab) rebuild() {
	var items []components.ListItem
	for _, sk := range skill.List() {
		detail := sk.Description
		if detail == "" {
			detail = sk.Source
		}
		items = append(items, components.ListItem{Label: sk.Name, Detail: detail, Data: sk.Path})
	}
	if len(items) == 0 {
		items = append(items, components.ListItem{Label: "(no skills loaded)"})
	}
	t.list.SetItems(items)
}

func (t *skillsTab) HandleKey(ev *tcell.EventKey) bool {
	return t.list.HandleKey(ev)
}

func (t *skillsTab) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	return t.list.HandleMouse(x, y, buttons)
}

func (t *skillsTab) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	t.list.Draw(s, bounds, focused)
}
