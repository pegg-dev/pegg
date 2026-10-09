package connect

import (
	"context"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/tui/styles"
)

func (c *Connector) reloadConfig() error {
	fresh, err := config.Load()
	if err != nil {
		return err
	}
	c.deps.Config = fresh
	return nil
}

func (c *Connector) registerSettingsMethods() {
	c.dispatch.register("settings.general.get", ScopeSettingsRead, c.handleSettingsGeneralGet)
	c.dispatch.register("settings.general.update", ScopeSettingsWrite, c.handleSettingsGeneralUpdate)

	c.dispatch.register("settings.providers.upsert", ScopeSettingsWrite, c.handleSettingsProviderUpsert)
	c.dispatch.register("settings.providers.remove", ScopeSettingsWrite, c.handleSettingsProviderRemove)

	c.registerPermissionSettings()
	c.registerCompactionMemorySettings()
	c.registerMCPSkillRulePluginSettings()
}

func (c *Connector) handleSettingsGeneralGet(_ context.Context, _ *Request) (any, error) {
	theme := ""
	var providers []string
	if c.deps.Config != nil {
		theme = c.deps.Config.Theme
		for _, p := range c.deps.Config.Providers {
			providers = append(providers, p.Provider)
		}
	}
	return map[string]any{
		"theme":     theme,
		"themes":    styles.Names(),
		"providers": providers,
	}, nil
}

type generalUpdateParams struct {
	Theme string `json:"theme"`
}

func (c *Connector) handleSettingsGeneralUpdate(_ context.Context, req *Request) (any, error) {
	var p generalUpdateParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Theme == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "theme is required"}
	}
	valid := false
	for _, name := range styles.Names() {
		if name == p.Theme {
			valid = true
			break
		}
	}
	if !valid {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "unknown theme: " + p.Theme}
	}
	if err := config.SaveTheme(p.Theme); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save theme: " + err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "general", "theme": p.Theme})
	return map[string]any{"theme": p.Theme}, nil
}

type providerUpsertParams struct {
	Provider string            `json:"provider"`
	Driver   string            `json:"driver"`
	APIKey   string            `json:"api_key"`
	BaseURL  string            `json:"base_url"`
	Headers  map[string]string `json:"headers"`
}

func (c *Connector) handleSettingsProviderUpsert(_ context.Context, req *Request) (any, error) {
	var p providerUpsertParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Provider == "" && p.Driver == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "provider or driver is required"}
	}
	if p.APIKey != "" {
		if err := c.requireConfirm("write provider API key", p.Provider); err != nil {
			return nil, err
		}
	}

	cfg := config.LLMConfig{Provider: p.Provider, Driver: p.Driver, APIKey: p.APIKey, BaseURL: p.BaseURL, Headers: p.Headers}
	if p.APIKey == "" && c.deps.Config != nil {
		for _, existing := range c.deps.Config.Providers {
			if existing.Provider == p.Provider {
				cfg.APIKey = existing.APIKey
				break
			}
		}
	}
	if err := config.UpsertProvider(cfg); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save provider: " + err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	if c.deps.Bus != nil {
		c.deps.Bus.Publish(event.TopicProviderAdded, cfg)
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "providers", "upserted": cfg.Provider})
	return map[string]any{"provider": cfg.Provider, "saved": true}, nil
}

type providerRemoveParams struct {
	Provider string `json:"provider"`
}

func (c *Connector) handleSettingsProviderRemove(_ context.Context, req *Request) (any, error) {
	var p providerRemoveParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Provider == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "provider is required"}
	}
	if err := config.RemoveProvider(p.Provider); err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	if c.deps.Bus != nil {
		c.deps.Bus.Publish(event.TopicProviderRemoved, p.Provider)
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "providers", "removed": p.Provider})
	return map[string]any{"provider": p.Provider, "removed": true}, nil
}
