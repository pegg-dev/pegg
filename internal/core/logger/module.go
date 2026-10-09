package logger

import (
	"errors"
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
)

type DriverFactory func(cfg config.LoggerConfig) (Handler, error)

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

func NewFromConfig(cfg config.LoggerConfig) (*Logger, error) {
	driversMu.RLock()
	factory, ok := drivers[cfg.Driver]
	driversMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("logger: no driver registered for %q", cfg.Driver)
	}

	h, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	return New(LevelDebug, h), nil
}

func LoggerModule(cfg config.LoggerConfig) (*Logger, error) {
	if cfg.Driver == "" {
		return nil, errors.New("logger: driver name is empty")
	}
	return NewFromConfig(cfg)
}
