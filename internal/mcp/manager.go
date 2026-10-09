package mcp

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
)

type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client
	log     *logger.Logger
	wg      sync.WaitGroup
	closed  bool
}

func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		log:     log,
	}
}

func (m *Manager) Start(servers map[string]config.MCPServerConfig) {
	for name, cfg := range servers {
		m.wg.Add(1)
		go m.connect(name, cfg)
	}
}

func (m *Manager) connect(name string, cfg config.MCPServerConfig) {
	defer m.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	transport, err := NewTransport(cfg, m.log.Writer(logger.LevelDebug, "mcp["+name+"]: "))
	if err != nil {
		m.log.Fwarn("mcp: server %q: %v", name, err)
		return
	}

	if err := transport.Connect(ctx); err != nil {
		_ = transport.Close()
		m.log.Fwarn("mcp: server %q: connect: %v", name, err)
		return
	}

	client := NewClient(name, transport, Options{Name: name})
	if _, err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		m.log.Fwarn("mcp: server %q: initialize: %v", name, err)
		return
	}

	specs, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		m.log.Fwarn("mcp: server %q: list tools: %v", name, err)
		return
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = client.Close()
		m.log.Fdebug("mcp: server %q: connect finished after shutdown", name)
		return
	}
	for _, spec := range specs {
		t := NewTool(name, spec, client)
		if err := tools.Register(t); err != nil {
			m.log.Fdebug("mcp: server %q: register tool %q: %v", name, t.Name(), err)
			continue
		}
	}
	m.clients[name] = client
	m.mu.Unlock()

	m.log.Finfo("mcp: server %q connected (%d tools)", name, len(specs))
}

func (m *Manager) Clients() map[string]*Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]*Client, len(m.clients))
	for name, c := range m.clients {
		out[name] = c
	}
	return out
}

func (m *Manager) Wait() {
	m.wg.Wait()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	for _, c := range clients {
		c.Close()
	}

	for _, t := range tools.List() {
		mt, ok := t.(*MCPTool)
		if !ok {
			continue
		}
		if c, exists := m.Clients()[mt.server]; exists && c != nil {
			tools.Unregister(t.Name())
		}
	}

	m.wg.Wait()
	m.log.Debug("mcp: all servers closed")
	return nil
}

func NewTransport(cfg config.MCPServerConfig, stderr io.Writer) (Duplex, error) {
	if cfg.URL != "" {
		return NewSSETransport(SSEOptions{
			URL:     cfg.URL,
			Headers: cfg.Headers,
		})
	}
	if cfg.Command != "" {
		return NewStdioTransport(StdioOptions{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
			Stderr:  stderr,
		})
	}
	return nil, ErrUnsupported
}
