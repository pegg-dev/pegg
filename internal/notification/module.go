package notification

import (
	"errors"
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type DriverFactory func(cfg config.NotificationConfig) (Driver, error)

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

func NewFromConfig(cfg config.NotificationConfig) ([]Driver, error) {
	if len(cfg.Drivers) == 0 {
		return nil, errors.New("notification: no drivers configured")
	}

	seen := make(map[string]bool, len(cfg.Drivers))
	out := make([]Driver, 0, len(cfg.Drivers))
	for _, name := range cfg.Drivers {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		driversMu.RLock()
		factory, ok := drivers[name]
		driversMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("notification: no driver registered for %q", name)
		}

		d, err := factory(cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}

	if len(out) == 0 {
		return nil, errors.New("notification: no drivers configured")
	}
	return out, nil
}

func NotificationModule(cfg config.NotificationConfig, bus event.Bus, log *logger.Logger) (*Notifier, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	ds, err := NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	n := New(bus, log, cfg.Enabled, ds...)
	if err := n.Start(); err != nil {
		return nil, err
	}
	return n, nil
}
