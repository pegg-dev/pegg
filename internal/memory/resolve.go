package memory

import (
	"errors"

	"github.com/peggco/pegg/internal/core/config"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
)

var (
	errNoLLM   = errors.New("memory: no llm manager available")
	errNoModel = errors.New("memory: no model available")
)

func (m *Manager) observerModel() (llm.Provider, llm.Model, error) {
	if m.deps.LLM == nil {
		return nil, llm.Model{}, errNoLLM
	}
	cfg := m.cfg
	if cfg != nil && cfg.Provider != "" && cfg.Model != "" && !decisionapi.HasProvider(cfg.Provider) {
		prov, err := m.deps.LLM.Provider(cfg.Provider)
		if err == nil {
			res := m.deps.LLM.Select(llm.SelectRequest{
				Mode:     llm.SelectModeExact,
				Provider: cfg.Provider,
				Model:    cfg.Model,
			})
			if res.Err == nil && res.Model.ID != "" {
				return prov, res.Model, nil
			}
		}
	}
	res := m.deps.LLM.Select(llm.SelectRequest{Mode: llm.SelectModePreferred})
	if res.Err != nil || res.Model.ID == "" {
		return nil, llm.Model{}, errNoModel
	}
	prov, err := m.deps.LLM.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}, errNoModel
	}
	return prov, res.Model, nil
}

func (m *Manager) decider() (decisionapi.Provider, string, bool) {
	if m.deps.Decision == nil {
		return nil, "", false
	}
	cfg := m.cfg
	if cfg != nil && cfg.Provider != "" && decisionapi.HasProvider(cfg.Provider) {
		prov, err := m.deps.Decision.Provider(cfg.Provider)
		if err != nil {
			return nil, "", false
		}
		return prov, cfg.Model, true
	}
	prov, err := m.deps.Decision.Preferred()
	if err != nil {
		return nil, "", false
	}
	return prov, "", true
}

func defaultCfg() *config.MemoryConfig {
	return &config.MemoryConfig{
		Enabled:       true,
		ContextBudget: config.DefaultMemoryBudget,
		MaxResults:    config.DefaultMemoryMaxResults,
	}
}
