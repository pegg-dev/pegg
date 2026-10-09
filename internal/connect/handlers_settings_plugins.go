package connect

import (
	"context"
	"os"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/plugin"
)

func (c *Connector) registerPluginMethods() {
	c.dispatch.register("settings.plugins.list", ScopeSettingsRead, c.handlePluginsList)
	c.dispatch.register("settings.plugins.toggle", ScopeSettingsWrite, c.handlePluginToggle)
}

func (c *Connector) handlePluginsList(_ context.Context, _ *Request) (any, error) {
	dir, err := plugin.GetPluginDir()
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "plugin dir: " + err.Error()}
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "read plugins: " + err.Error()}
	}

	globalEnabled := true
	excluded := map[string]bool{}
	var cfg *config.Config
	if c.deps.Config != nil {
		cfg = c.deps.Config
		globalEnabled = cfg.Plugins.Enabled
		for _, name := range cfg.Plugins.Exclude {
			excluded[name] = true
		}
	}

	loaded := map[string]bool{}
	if c.deps.Plugin != nil {
		for _, p := range c.deps.Plugin.ListPlugins() {
			loaded[p.Name] = true
		}
	}

	var out []map[string]any
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		out = append(out, map[string]any{
			"name":    name,
			"enabled": globalEnabled && !excluded[name],
			"loaded":  loaded[name],
		})
	}
	return map[string]any{"enabled": globalEnabled, "plugins": out}, nil
}

type pluginToggleParams struct {
	Name string `json:"name"`
}

func (c *Connector) handlePluginToggle(_ context.Context, req *Request) (any, error) {
	var p pluginToggleParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Name == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "name is required"}
	}
	enabled, err := config.TogglePlugin(p.Name)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "toggle plugin: " + err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	if c.deps.Plugin != nil && c.deps.Config != nil {
		c.deps.Plugin.SetExcludedPlugins(c.deps.Config.Plugins.Exclude)
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "plugins", "name": p.Name, "enabled": enabled})
	return map[string]any{"name": p.Name, "enabled": enabled}, nil
}
