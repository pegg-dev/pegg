package decision

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

type ProviderFactory func(cfg config.LLMConfig) (Provider, error)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]ProviderFactory)

	defaultModelsMu sync.RWMutex
	defaultModels   = make(map[string]string)
)

func RegisterProvider(name string, factory ProviderFactory) {
	if name == "" || factory == nil {
		return
	}
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = factory
}

func RegisterDefaultModel(name, model string) {
	if name == "" || model == "" {
		return
	}
	defaultModelsMu.Lock()
	defer defaultModelsMu.Unlock()
	defaultModels[name] = model
}

func DefaultModel(name string) string {
	defaultModelsMu.RLock()
	defer defaultModelsMu.RUnlock()
	return defaultModels[name]
}

func ListProviders() []string {
	providersMu.RLock()
	defer providersMu.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func HasProvider(name string) bool {
	providersMu.RLock()
	defer providersMu.RUnlock()

	_, ok := providers[name]
	return ok
}

func resolveProvider(cfg config.LLMConfig) (Provider, error) {
	if cfg.Provider == "" {
		return nil, errors.New("decision: empty provider name")
	}
	providersMu.RLock()
	factory, ok := providers[cfg.Provider]
	providersMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("decision: no provider registered for %q", cfg.Provider)
	}
	return factory(cfg)
}
