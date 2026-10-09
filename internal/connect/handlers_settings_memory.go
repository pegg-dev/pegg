package connect

import (
	"context"

	"github.com/peggco/pegg/internal/core/config"
)

func (c *Connector) registerMemoryMethods() {
	c.dispatch.register("settings.memory.get", ScopeSettingsRead, c.handleMemoryGet)
	c.dispatch.register("settings.memory.update", ScopeSettingsWrite, c.handleMemoryUpdate)
	c.dispatch.register("settings.memory.entries", ScopeSettingsRead, c.handleMemoryEntries)
	c.dispatch.register("settings.memory.clear", ScopeSettingsWrite, c.handleMemoryClear)
}

func (c *Connector) handleMemoryGet(_ context.Context, _ *Request) (any, error) {
	m := (*config.MemoryConfig)(nil)
	if c.deps.Config != nil {
		m = c.deps.Config.Memory
	}
	enabled := true
	gate := config.GateAuto
	provider, model := "", ""
	if m != nil {
		enabled = m.Enabled
		gate = m.GateValue()
		provider = m.Provider
		model = m.Model
	}
	return map[string]any{
		"enabled":        enabled,
		"provider":       provider,
		"model":          model,
		"gate":           gate,
		"gates":          config.GateModes(),
		"threshold":      m.ThresholdValue(),
		"context_budget": m.BudgetValue(),
		"max_results":    m.MaxResultsValue(),
		"consolidate":    m.ConsolidateValue(),
		"skip_tools":     skipTools(m),
	}, nil
}

func skipTools(m *config.MemoryConfig) []string {
	if m == nil {
		return nil
	}
	return m.SkipTools
}

type memoryUpdateParams struct {
	Enabled       *bool    `json:"enabled"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	Gate          string   `json:"gate"`
	Threshold     *float64 `json:"threshold"`
	ContextBudget int      `json:"context_budget"`
	MaxResults    int      `json:"max_results"`
	Consolidate   *bool    `json:"consolidate"`
	SkipTools     []string `json:"skip_tools"`
}

func (c *Connector) handleMemoryUpdate(_ context.Context, req *Request) (any, error) {
	var p memoryUpdateParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Gate != "" {
		valid := false
		for _, g := range config.GateModes() {
			if g == p.Gate {
				valid = true
				break
			}
		}
		if !valid {
			return nil, &FrameError{Code: ErrCodeBadRequest, Message: "unknown gate: " + p.Gate}
		}
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	cfg := &config.MemoryConfig{
		Enabled:       enabled,
		Provider:      p.Provider,
		Model:         p.Model,
		Gate:          p.Gate,
		Threshold:     p.Threshold,
		ContextBudget: p.ContextBudget,
		MaxResults:    p.MaxResults,
		Consolidate:   p.Consolidate,
		SkipTools:     p.SkipTools,
	}
	if err := config.UpsertMemory(cfg); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save memory: " + err.Error()}
	}
	if c.deps.Memory != nil {
		c.deps.Memory.UpdateConfig(cfg)
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "memory"})
	return map[string]any{"saved": true}, nil
}

func (c *Connector) handleMemoryEntries(_ context.Context, _ *Request) (any, error) {
	if c.deps.Memory == nil || c.deps.Memory.Store() == nil {
		return map[string]any{"entries": []any{}, "stats": map[string]any{}}, nil
	}
	store := c.deps.Memory.Store()
	idx, err := store.ReadIndex()
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "read memory index: " + err.Error()}
	}
	entries := make([]map[string]any, 0, len(idx))
	for _, e := range idx {
		entries = append(entries, map[string]any{
			"id":       e.ID,
			"date":     e.Date,
			"type":     e.Type,
			"title":    e.Title,
			"keywords": e.Keywords,
			"file":     e.File,
		})
	}
	count, lastDate, lastFile := store.Stats()
	return map[string]any{
		"entries": entries,
		"stats": map[string]any{
			"count":     count,
			"last_date": lastDate,
			"last_file": lastFile,
		},
	}, nil
}

func (c *Connector) handleMemoryClear(_ context.Context, _ *Request) (any, error) {
	if err := c.requireConfirm("clear all memory", "memory"); err != nil {
		return nil, err
	}
	if c.deps.Memory == nil || c.deps.Memory.Store() == nil {
		return nil, &FrameError{Code: ErrCodeUnavailable, Message: "memory manager unavailable"}
	}
	if err := c.deps.Memory.Store().Clear(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "clear memory: " + err.Error()}
	}
	Audit("memory.clear", nil)
	c.emit(eventSettings, "", "", map[string]any{"group": "memory", "cleared": true})
	return map[string]any{"cleared": true}, nil
}
