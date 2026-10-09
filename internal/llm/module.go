package llm

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

type DriverFactory func(cfg config.LLMConfig) (Provider, error)

type ProviderFactory func(cfg config.LLMConfig) (Provider, error)

var (
	driversMu   sync.RWMutex
	providersMu sync.RWMutex
	drivers     = make(map[string]DriverFactory)
	providers   = make(map[string]ProviderFactory)
)

func RegisterDriver(name string, factory DriverFactory) {
	if name == "" || factory == nil {
		return
	}
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[name] = factory
}

func RegisterProvider(name string, factory ProviderFactory) {
	if name == "" || factory == nil {
		return
	}
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = factory
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

func ListDrivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()

	names := make([]string, 0, len(drivers))
	for name := range drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func HasDriver(name string) bool {
	driversMu.RLock()
	defer driversMu.RUnlock()

	_, ok := drivers[name]
	return ok
}

func resolveProvider(cfg config.LLMConfig) (Provider, error) {
	if cfg.Provider != "" {
		providersMu.RLock()
		factory, ok := providers[cfg.Provider]
		providersMu.RUnlock()

		if ok {
			return factory(cfg)
		}
		if cfg.Driver == "" {
			return nil, fmt.Errorf("llm: no provider registered for %q", cfg.Provider)
		}
	}

	if cfg.Driver != "" {
		driversMu.RLock()
		factory, ok := drivers[cfg.Driver]
		driversMu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("llm: no driver registered for %q", cfg.Driver)
		}
		return factory(cfg)
	}

	return nil, errors.New("llm: neither provider nor driver configured")
}
