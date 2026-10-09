package agents

import (
	"fmt"
	"sort"
	"sync"

	"github.com/peggco/pegg/internal/agent"
)

type Factory func() (*agent.Agent, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

func Register(f Factory) error {
	if f == nil {
		return ErrNilFactory
	}
	name, err := factoryName(f)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := factories[name]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicate, name)
	}
	factories[name] = f
	return nil
}

func factoryName(f Factory) (string, error) {
	a, err := f()
	if err != nil {
		return "", fmt.Errorf("agents: register: %w", err)
	}
	if a == nil {
		return "", ErrNilAgent
	}
	if a.Name == "" {
		return "", ErrEmptyName
	}
	return a.Name, nil
}

func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Has(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := factories[name]
	return ok
}

func New(name string) (*agent.Agent, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return f()
}
