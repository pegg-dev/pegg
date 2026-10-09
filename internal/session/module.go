package session

import (
	"errors"
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type DriverFactory func(cfg config.SessionConfig) (Store, error)

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

func NewFromConfig(cfg config.SessionConfig) (Store, error) {
	driversMu.RLock()
	factory, ok := drivers[cfg.Driver]
	driversMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session: no driver registered for %q", cfg.Driver)
	}

	return factory(cfg)
}

func SessionModule(cfg config.SessionConfig, bus event.Bus, log *logger.Logger) (*Manager, error) {
	if cfg.Driver == "" {
		return nil, errors.New("session: driver name is empty")
	}
	store, err := NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewManager(store, bus, log), nil
}
