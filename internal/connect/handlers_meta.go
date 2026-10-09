package connect

import (
	"context"
	"time"

	"github.com/peggco/pegg/internal/llm"
)

func (c *Connector) registerMetaMethods() {
	c.dispatch.register("status.get", ScopeRead, c.handleStatusGet)
	c.dispatch.register("capabilities.get", ScopeRead, c.handleCapabilities)
	c.dispatch.register("kill", ScopeAdmin, c.handleKill)
	c.dispatch.register("models.list", ScopeRead, c.handleModelsList)
	c.dispatch.register("providers.list", ScopeRead, c.handleProvidersList)
}

func (c *Connector) handleStatusGet(_ context.Context, _ *Request) (any, error) {
	sessionTotal := 0
	if c.deps.Sessions != nil {
		if _, total, err := c.deps.Sessions.List(listQuery(1, 1)); err == nil {
			sessionTotal = total
		}
	}
	return map[string]any{
		"connected":       c.Connected(),
		"device_id":       c.identity.DeviceID,
		"device_name":     c.identity.Name,
		"workspace":       c.workspace(),
		"version":         c.deps.Version,
		"protocol":        ProtocolVersion,
		"uptime_secs":     int(time.Since(c.startedAt).Seconds()),
		"relay":           c.cfg.RelayURL,
		"running_runs":    len(c.runs.List()),
		"max_concurrency": maxConcurrency(c.cfg),
		"sessions":        sessionTotal,
		"scopes":          c.grantedScopes(),
	}, nil
}

func (c *Connector) handleCapabilities(_ context.Context, _ *Request) (any, error) {
	return map[string]any{
		"protocol":        ProtocolVersion,
		"version":         c.deps.Version,
		"methods":         c.dispatch.Names(),
		"scopes":          []string{ScopeRead, ScopeSessionsRead, ScopeSessionsWrite, ScopeChat, ScopeSettingsRead, ScopeSettingsWrite, ScopeExec, ScopeAdmin},
		"capabilities":    c.capabilities(),
		"auto_approve":    c.cfg.AutoApprove,
		"allowed_methods": c.cfg.AllowedMethods,
	}, nil
}

func (c *Connector) handleKill(_ context.Context, _ *Request) (any, error) {
	runs := c.runs.List()
	c.runs.cancelAll()
	Audit("kill", map[string]any{"runs": len(runs)})
	return map[string]any{"canceled": len(runs)}, nil
}

func (c *Connector) handleModelsList(_ context.Context, _ *Request) (any, error) {
	var models []ModelInfo
	if c.deps.LLM != nil && c.deps.Config != nil {
		c.deps.LLM.WaitUntilReady()
		for _, p := range c.deps.Config.Providers {
			list, err := c.deps.LLM.Models(p.Provider)
			if err != nil {
				continue
			}
			for _, m := range list {
				models = append(models, ModelInfo{
					Provider:         p.Provider,
					ID:               m.ID,
					Name:             m.Name,
					ReasoningOptions: reasoningOptions(m),
				})
			}
		}
	}
	return map[string]any{"models": models}, nil
}

func reasoningOptions(m llm.Model) []llm.ReasoningOption {
	if m.Config == nil {
		return nil
	}
	return m.Config.ReasoningOptions
}

func (c *Connector) handleProvidersList(_ context.Context, _ *Request) (any, error) {
	var configured []map[string]any
	if c.deps.Config != nil {
		for _, p := range c.deps.Config.Providers {
			configured = append(configured, map[string]any{
				"provider":   p.Provider,
				"driver":     p.Driver,
				"base_url":   p.BaseURL,
				"has_key":    p.APIKey != "",
				"configured": true,
			})
		}
	}
	return map[string]any{
		"configured": configured,
		"available":  availableProviders(),
	}, nil
}
