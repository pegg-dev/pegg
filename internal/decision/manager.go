package decision

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type entry struct {
	cfg      config.LLMConfig
	provider Provider
	err      error
}

type Manager struct {
	bus     event.Bus
	log     *logger.Logger
	mu      sync.RWMutex
	entries map[string]*entry
}

func NewManager(bus event.Bus, log *logger.Logger) *Manager {
	return &Manager{
		bus:     bus,
		log:     log,
		entries: make(map[string]*entry),
	}
}

func (m *Manager) Start() error {
	if err := m.bus.Subscribe(event.TopicAppMounted, m.handleAppMounted); err != nil {
		return fmt.Errorf("decision manager: subscribe %s: %w", event.TopicAppMounted, err)
	}
	if err := m.bus.SubscribeAsync(event.TopicProviderAdded, m.handleProviderAdded, false); err != nil {
		return fmt.Errorf("decision manager: subscribe %s: %w", event.TopicProviderAdded, err)
	}
	if err := m.bus.Subscribe(event.TopicProviderRemoved, m.handleProviderRemoved); err != nil {
		return fmt.Errorf("decision manager: subscribe %s: %w", event.TopicProviderRemoved, err)
	}
	m.log.Info("decision manager started")
	return nil
}

func (m *Manager) Shutdown() {
	_ = m.bus.Unsubscribe(event.TopicAppMounted, m.handleAppMounted)
	_ = m.bus.Unsubscribe(event.TopicProviderAdded, m.handleProviderAdded)
	_ = m.bus.Unsubscribe(event.TopicProviderRemoved, m.handleProviderRemoved)
	m.log.Debug("decision manager stopped")
}

func (m *Manager) handleAppMounted(cfg *config.Config) {
	if cfg == nil {
		m.log.Warn("decision: app.mounted received nil config")
		return
	}
	for _, c := range cfg.Providers {
		m.loadProvider(c)
	}
}

func (m *Manager) handleProviderAdded(cfg config.LLMConfig) {
	m.loadProvider(cfg)
}

func (m *Manager) handleProviderRemoved(name string) {
	if name == "" {
		return
	}
	m.mu.Lock()
	delete(m.entries, name)
	m.mu.Unlock()
	m.log.Finfo("decision: provider %q removed", name)
}

func (m *Manager) loadProvider(cfg config.LLMConfig) {
	if !HasProvider(cfg.Provider) {
		return
	}
	prov, err := resolveProvider(cfg)
	if err != nil {
		m.log.Ferror("decision: load provider %q: %v", cfg.Provider, err)
		m.mu.Lock()
		m.entries[cfg.Provider] = &entry{cfg: cfg, err: err}
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	m.entries[cfg.Provider] = &entry{cfg: cfg, provider: prov}
	m.mu.Unlock()
	m.log.Finfo("decision: provider %q ready", cfg.Provider)
}

func (m *Manager) Provider(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[name]
	if !ok {
		return nil, fmt.Errorf("decision: provider %q not loaded", name)
	}
	if e.err != nil {
		return nil, e.err
	}
	if e.provider == nil {
		return nil, fmt.Errorf("decision: provider %q has no instance", name)
	}
	return e.provider, nil
}

func (m *Manager) Preferred() (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for name, e := range m.entries {
		if e.provider == nil || e.err != nil || e.cfg.APIKey == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, errors.New("decision: no providers with an api key available")
	}
	sort.Strings(names)
	return m.entries[names[0]].provider, nil
}
