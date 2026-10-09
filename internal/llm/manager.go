package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type SessionResolver func() (provider, model string, ok bool)

type entry struct {
	cfg      config.LLMConfig
	provider Provider
	models   []Model
	err      error
	system   bool
}

type Manager struct {
	bus             event.Bus
	log             *logger.Logger
	cacheStore      cache.Cache
	mu              sync.RWMutex
	entries         map[string]*entry
	sessionResolver SessionResolver
	pricesOnce      sync.Once
	prices          map[string]ModelConfig
	wg              sync.WaitGroup
	ready           chan struct{}
	readyOnce       sync.Once
	syncStarted     atomic.Bool
}

func NewManager(bus event.Bus, log *logger.Logger, cacheStore cache.Cache) *Manager {
	ready := make(chan struct{})
	close(ready)
	return &Manager{
		bus:        bus,
		log:        log,
		cacheStore: cacheStore,
		entries:    make(map[string]*entry),
		ready:      ready,
	}
}

func (m *Manager) SetSessionResolver(fn SessionResolver) {
	m.sessionResolver = fn
}

func (m *Manager) RegisterSystemProvider(name string, prov Provider, models []Model) {
	if name == "" || prov == nil {
		return
	}
	m.mu.Lock()
	m.entries[name] = &entry{cfg: config.LLMConfig{Provider: name}, provider: prov, models: models, system: true}
	m.mu.Unlock()
}

func (m *Manager) Start() error {
	m.ready = make(chan struct{})
	m.readyOnce = sync.Once{}
	m.syncStarted.Store(false)

	if err := m.bus.Subscribe(event.TopicAppMounted, m.handleAppMounted); err != nil {
		return fmt.Errorf("llm manager: subscribe %s: %w", event.TopicAppMounted, err)
	}
	if err := m.bus.SubscribeAsync(event.TopicProviderAdded, m.handleProviderAdded, false); err != nil {
		return fmt.Errorf("llm manager: subscribe %s: %w", event.TopicProviderAdded, err)
	}
	if err := m.bus.Subscribe(event.TopicProviderRemoved, m.handleProviderRemoved); err != nil {
		return fmt.Errorf("llm manager: subscribe %s: %w", event.TopicProviderRemoved, err)
	}
	if err := m.bus.Subscribe(event.TopicModelSelect, m.handleModelSelect); err != nil {
		return fmt.Errorf("llm manager: subscribe %s: %w", event.TopicModelSelect, err)
	}
	m.log.Info("llm manager started")
	return nil
}

func (m *Manager) Shutdown() {
	_ = m.bus.Unsubscribe(event.TopicAppMounted, m.handleAppMounted)
	_ = m.bus.Unsubscribe(event.TopicProviderAdded, m.handleProviderAdded)
	_ = m.bus.Unsubscribe(event.TopicProviderRemoved, m.handleProviderRemoved)
	_ = m.bus.Unsubscribe(event.TopicModelSelect, m.handleModelSelect)
	m.markReady()
	m.log.Debug("llm manager stopped")
}

func (m *Manager) handleAppMounted(cfg *config.Config) {
	if cfg == nil {
		m.log.Warn("llm: app.mounted received nil config")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.EnsurePricesCached(ctx); err != nil {
		m.log.Fwarn("llm: cache model prices: %v", err)
	}
	m.syncStarted.Store(true)
	go m.runSyncAsync(cfg.Providers)
}

func (m *Manager) runSyncAsync(cfgs []config.LLMConfig) {
	m.log.Fdebug("llm: syncing %d providers async", len(cfgs))
	for _, cfg := range cfgs {
		m.wg.Add(1)
		go func(c config.LLMConfig) {
			defer m.wg.Done()
			m.loadProvider(context.Background(), c)
		}(cfg)
	}
	m.wg.Wait()
	m.markReady()
	m.log.Info("llm: all providers synced")
}

func (m *Manager) markReady() {
	m.readyOnce.Do(func() {
		close(m.ready)
	})
}

func (m *Manager) WaitUntilReady() {
	<-m.ready
}

func (m *Manager) EnsurePricesCached(ctx context.Context) error {
	if m.cacheStore == nil {
		return errors.New("llm: no cache store available")
	}
	if _, err := m.cacheStore.Get(PricesCacheKey); err == nil {
		m.log.Fdebug("llm: model prices already cached")
		return nil
	}
	prices, err := FetchPrices(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(prices)
	if err != nil {
		return fmt.Errorf("llm: marshal prices: %w", err)
	}
	if err := m.cacheStore.Set(PricesCacheKey, data); err != nil {
		return fmt.Errorf("llm: cache prices: %w", err)
	}
	m.pricesOnce = sync.Once{}
	m.prices = prices
	m.log.Finfo("llm: cached %d model price entries", len(prices))
	return nil
}

func (m *Manager) ModelConfigFor(provider, model string) (*ModelConfig, error) {
	prices, err := m.loadPrices()
	if err != nil {
		return nil, err
	}
	return lookupModelConfig(prices, provider, model)
}

func (m *Manager) loadPrices() (map[string]ModelConfig, error) {
	m.pricesOnce.Do(func() {
		if m.cacheStore == nil {
			return
		}
		data, err := m.cacheStore.Get(PricesCacheKey)
		if err != nil {
			return
		}
		var prices map[string]ModelConfig
		if err := json.Unmarshal(data, &prices); err != nil {
			m.log.Fwarn("llm: unmarshal prices: %v", err)
			return
		}
		m.prices = prices
	})
	if m.prices == nil {
		return nil, errors.New("llm: prices not cached")
	}
	return m.prices, nil
}

func (m *Manager) enrichWithConfig(provider string, models []Model) []Model {
	prices, err := m.loadPrices()
	if err != nil {
		m.log.Fdebug("llm: enrichWithConfig: no prices cached: %v", err)
		return models
	}
	for i := range models {
		if cfg, err := lookupModelConfig(prices, provider, models[i].ID); err == nil {
			models[i].Config = cfg
			if len(cfg.ReasoningOptions) > 0 {
				m.log.Fdebug("llm: model %q has %d reasoning options", models[i].ID, len(cfg.ReasoningOptions))
			}
		} else {
			m.log.Fdebug("llm: enrichWithConfig: model %q not found in prices (provider=%q): %v", models[i].ID, provider, err)
		}
	}
	return models
}

func (m *Manager) Sync(ctx context.Context, cfgs []config.LLMConfig) {
	m.log.Fdebug("llm: syncing %d providers", len(cfgs))
	m.syncStarted.Store(true)

	var wg sync.WaitGroup
	for _, cfg := range cfgs {
		wg.Add(1)

		go func(c config.LLMConfig) {
			defer wg.Done()
			m.loadProvider(ctx, c)
		}(cfg)
	}

	wg.Wait()
	m.markReady()
}

func (m *Manager) handleProviderAdded(cfg config.LLMConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.loadProvider(ctx, cfg)
}

func (m *Manager) handleProviderRemoved(name string) {
	if name == "" {
		return
	}
	m.mu.Lock()
	delete(m.entries, name)
	m.mu.Unlock()

	if m.cacheStore != nil {
		_ = m.cacheStore.Delete(name)
	}
	m.log.Finfo("llm: provider %q removed", name)
}

func (m *Manager) loadProvider(ctx context.Context, cfg config.LLMConfig) {
	name, _, models, err := m.resolveAndLoad(ctx, cfg)
	if err != nil {
		if name == "" {
			m.log.Warn("llm: skipping provider with empty name")
			return
		}
		m.log.Ferror("llm: load provider %q: %v", name, err)
		m.recordError(name, cfg, err)
		return
	}

	m.log.Finfo("llm: provider %q loaded %d models", name, len(models))
	m.publishModelsLoaded(name, len(models), nil)
}

func (m *Manager) resolveAndLoad(ctx context.Context, cfg config.LLMConfig) (string, Provider, []Model, error) {
	name := providerName(cfg)
	if name == "" {
		return "", nil, nil, errors.New("llm: empty provider name")
	}

	prov, err := resolveProvider(cfg)
	if err != nil {
		return name, nil, nil, err
	}

	skipCache := false
	if s, ok := prov.(interface{ SkipModelCache() bool }); ok {
		skipCache = s.SkipModelCache()
	}

	if !skipCache {
		if cached, ok := m.loadCachedModels(name); ok {
			cached = m.enrichWithConfig(name, cached)
			m.storeEntry(name, cfg, prov, cached)
			m.log.Fdebug("llm: provider %q loaded %d models from cache", name, len(cached))
			return name, prov, cached, nil
		}
	}
	models, err := prov.ListModels(ctx)
	if err != nil {
		return name, prov, nil, err
	}

	models = m.enrichWithConfig(name, models)
	m.storeEntry(name, cfg, prov, models)
	m.saveCachedModels(name, models)
	return name, prov, models, nil
}

func (m *Manager) storeEntry(name string, cfg config.LLMConfig, prov Provider, models []Model) {
	m.mu.Lock()
	m.entries[name] = &entry{cfg: cfg, provider: prov, models: models}
	m.mu.Unlock()
}

func (m *Manager) saveCachedModels(name string, models []Model) {
	if m.cacheStore == nil {
		return
	}
	data, err := json.Marshal(models)
	if err != nil {
		m.log.Ferror("llm: marshal models for cache %q: %v", name, err)
		return
	}
	if err := m.cacheStore.Set(name, data); err != nil {
		m.log.Ferror("llm: cache models for provider %q: %v", name, err)
	}
}

func (m *Manager) loadCachedModels(name string) ([]Model, bool) {
	if m.cacheStore == nil {
		return nil, false
	}
	data, err := m.cacheStore.Get(name)
	if err != nil {
		return nil, false
	}
	var models []Model
	if err := json.Unmarshal(data, &models); err != nil {
		m.log.Fwarn("llm: unmarshal cached models for %q: %v", name, err)
		return nil, false
	}
	return models, true
}

func (m *Manager) recordError(name string, cfg config.LLMConfig, err error) {
	m.mu.Lock()
	if e, ok := m.entries[name]; ok {
		e.err = err
	} else {
		m.entries[name] = &entry{cfg: cfg, err: err}
	}
	m.mu.Unlock()
	m.publishModelsLoaded(name, 0, err)
}

func (m *Manager) publishModelsLoaded(name string, count int, err error) {
	m.bus.Publish(event.TopicModelsLoaded, ModelsLoaded{
		Provider: name,
		Count:    count,
		Err:      err,
	})
	m.log.Fdebug("llm: models loaded for %q (model count=%d, err=%v)", name, count, err)
}

func (m *Manager) handleModelSelect(req SelectRequest) {
	m.log.Fdebug("llm: model select request provider=%q model=%q mode=%s", req.Provider, req.Model, req.Mode)

	res := m.Select(req)
	if res.Err != nil {
		m.log.Fwarn("llm: model select failed: %v", res.Err)
	}
	if req.ReplyTopic != "" {
		m.bus.Publish(req.ReplyTopic, res)
	}
}

func (m *Manager) Select(req SelectRequest) SelectResult {
	if m.syncStarted.Load() {
		m.WaitUntilReady()
	}
	switch req.Mode {
	case SelectModePreferred:
		return m.preferred(req.Provider)
	default:
		return m.exact(req.Provider, req.Model)
	}
}

func (m *Manager) exact(provider, model string) SelectResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, e := range m.entries {
		if provider != "" && name != provider {
			continue
		}
		for _, mdl := range e.models {
			if mdl.ID == model || mdl.Name == model {
				return SelectResult{Provider: name, Model: mdl}
			}
		}
	}
	return SelectResult{Err: fmt.Errorf("llm: model %q not found", model)}
}

func (m *Manager) preferred(provider string) SelectResult {
	if m.sessionResolver != nil {
		if prov, model, ok := m.sessionResolver(); ok {
			if res := m.exact(prov, model); res.Err == nil {
				return res
			}
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, e := range m.entries {
		if e.system {
			continue
		}
		if provider != "" && name != provider {
			continue
		}
		for _, mdl := range e.models {
			if isFreeModel(mdl) {
				return SelectResult{Provider: name, Model: mdl}
			}
		}
	}

	for name, e := range m.entries {
		if e.system {
			continue
		}
		if provider != "" && name != provider {
			continue
		}
		if len(e.models) > 0 {
			return SelectResult{Provider: name, Model: e.models[0]}
		}
	}
	return SelectResult{Err: errors.New("llm: no models cached")}
}

func isFreeModel(m Model) bool {
	return strings.Contains(strings.ToLower(m.Name), "free") ||
		strings.Contains(strings.ToLower(m.ID), "free")
}

func (m *Manager) Provider(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[name]
	if !ok {
		return nil, fmt.Errorf("llm: provider %q not loaded", name)
	}
	if e.provider == nil {
		return nil, fmt.Errorf("llm: provider %q has no instance", name)
	}
	return e.provider, nil
}

func (m *Manager) Models(name string) ([]Model, error) {
	if m.syncStarted.Load() {
		m.WaitUntilReady()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[name]
	if !ok {
		return nil, fmt.Errorf("llm: provider %q not loaded", name)
	}
	return e.models, nil
}

func providerName(cfg config.LLMConfig) string {
	if cfg.Provider != "" {
		return cfg.Provider
	}
	return cfg.Driver
}
