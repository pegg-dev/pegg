package connect

import (
	"context"
	"sort"
	"sync"

	json "github.com/goccy/go-json"
)

type Request struct {
	ID        string
	SessionID string
	RunID     string
	Params    json.RawMessage
}

func (r *Request) Decode(v any) error {
	if len(r.Params) == 0 {
		return nil
	}
	return json.Unmarshal(r.Params, v)
}

type HandlerFunc func(ctx context.Context, req *Request) (any, error)

type methodSpec struct {
	scope string
	fn    HandlerFunc
}

type Dispatcher struct {
	mu      sync.RWMutex
	methods map[string]methodSpec
}

func newDispatcher() *Dispatcher {
	return &Dispatcher{methods: make(map[string]methodSpec)}
}

func (d *Dispatcher) register(name, scope string, fn HandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.methods[name] = methodSpec{scope: scope, fn: fn}
}

func (d *Dispatcher) lookup(name string) (methodSpec, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	m, ok := d.methods[name]
	return m, ok
}

func (d *Dispatcher) Names() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	names := make([]string, 0, len(d.methods))
	for name := range d.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func hasScope(granted []string, need string) bool {
	if need == "" {
		return true
	}
	for _, s := range granted {
		if s == ScopeAdmin || s == need {
			return true
		}
	}
	return false
}
