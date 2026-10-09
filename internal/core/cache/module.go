package cache

import (
	"errors"
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

type DriverFactory func(cfg config.CacheConfig) (Cache, error)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]DriverFactory)
)

func RegisterDriver(name string, factory DriverFactory) {
	if name == "" || factory == nil {
		return
	}
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[name] = factory
}

func NewFromConfig(cfg config.CacheConfig) (Cache, error) {
	driversMu.RLock()
	factory, ok := drivers[cfg.Driver]
	driversMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cache: no driver registered for %q", cfg.Driver)
	}

	return factory(cfg)
}

func CacheModule(cfg config.CacheConfig) (Cache, error) {
	if cfg.Driver == "" {
		return nil, errors.New("cache: driver name is empty")
	}
	return NewFromConfig(cfg)
}
