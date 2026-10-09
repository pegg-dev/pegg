package settings

import (
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/plugin"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
)

type pluginsTab struct {
	settings *Settings
	list     *components.List
}

type pluginData struct {
	name string
}

func newPlugins(s *Settings) *pluginsTab {
	t := &pluginsTab{settings: s, list: components.NewList("Plugins")}
	t.rebuild()
	t.list.SetOnSelect(func(_ int, item components.ListItem) {
		if pd, ok := item.Data.(pluginData); ok {
			t.toggle(pd.name)
		}
	})
	return t
}

func (t *pluginsTab) rebuild() {
	var items []components.ListItem

	pluginDir, err := plugin.GetPluginDir()
	if err != nil {
		items = append(items, components.ListItem{
			Label:    "failed to get plugin directory",
			Detail:   err.Error(),
			Disabled: true,
		})
		t.list.SetItems(items)
		return
	}

	cfg := t.settings.deps.Config
	excludeSet := make(map[string]bool)
	globalEnabled := true
	if cfg != nil {
		globalEnabled = cfg.Plugins.Enabled
		for _, name := range cfg.Plugins.Exclude {
			excludeSet[name] = true
		}
	}

	loadedMap := make(map[string]*plugin.PluginInstance)
	if t.settings.deps.Plugin != nil {
		for _, p := range t.settings.deps.Plugin.ListPlugins() {
			loadedMap[p.Name] = p
		}
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			items = append(items, components.ListItem{
				Label:    "(no plugins installed)",
				Disabled: true,
			})
			t.list.SetItems(items)
			return
		}
		items = append(items, components.ListItem{
			Label:    "failed to read plugin directory",
			Detail:   err.Error(),
			Disabled: true,
		})
		t.list.SetItems(items)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name()[0] == '.' {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0111 == 0 {
			continue
		}
		if ext := filepath.Ext(entry.Name()); ext != "" {
			continue
		}

		name := entry.Name()
		disabled := excludeSet[name] || !globalEnabled
		status := "enabled"
		if disabled {
			status = "excluded"
		}

		if p, ok := loadedMap[name]; ok {
			items = append(items, components.ListItem{
				Label:    p.Name,
				Detail:   p.Version + " · " + p.Description,
				Disabled: disabled,
				Data:     pluginData{name: p.Name},
			})
		} else {
			items = append(items, components.ListItem{
				Label:    name,
				Detail:   status,
				Disabled: disabled,
				Data:     pluginData{name: name},
			})
		}
	}

	if len(items) == 0 {
		items = append(items, components.ListItem{
			Label:    "(no plugins installed)",
			Disabled: true,
		})
	}

	t.list.SetItems(items)
}

func (t *pluginsTab) toggle(name string) {
	_, err := config.TogglePlugin(name)
	if err != nil {
		t.settings.errMsg = "failed to toggle plugin: " + err.Error()
		return
	}
	t.settings.errMsg = ""
	if fresh, err := config.Load(); err == nil {
		t.settings.deps.Config = fresh
	}
	t.rebuild()
}

func (t *pluginsTab) HandleKey(ev *tcell.EventKey) bool {
	return t.list.HandleKey(ev)
}

func (t *pluginsTab) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	return t.list.HandleMouse(x, y, buttons)
}

func (t *pluginsTab) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	t.list.Draw(s, bounds, focused)
}
