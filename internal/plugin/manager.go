package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/plugin/shared"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Manager struct {
	mu      sync.RWMutex
	log     *logger.Logger
	cfg     *config.Config
	plugins map[string]*PluginInstance
	clients map[string]*goplugin.Client
	exclude map[string]bool
}

type logWriter struct {
	log *logger.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.log.Finfo("plugin: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}

type PluginInstance struct {
	Name        string
	Path        string
	Version     string
	Description string
	Client      *goplugin.Client
	Plugin      shared.Plugin
}

func NewManager(
	log *logger.Logger,
	cfg *config.Config,
	_ event.Bus,
	_ cache.Cache,
	_ *llm.Manager,
	_ *session.Manager,
	_ *vfs.VFS,
	_ *mcp.Manager,
	_ *lsp.Manager,
) *Manager {
	return &Manager{
		log:     log,
		cfg:     cfg,
		plugins: make(map[string]*PluginInstance),
		clients: make(map[string]*goplugin.Client),
		exclude: make(map[string]bool),
	}
}

func (m *Manager) SetExcludedPlugins(exclude []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exclude = make(map[string]bool)
	for _, name := range exclude {
		m.exclude[name] = true
	}
}

func (m *Manager) LoadPlugins() error {
	pluginDir, err := GetPluginDir()
	if err != nil {
		return fmt.Errorf("get plugin dir: %w", err)
	}

	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	pluginPaths, err := scanPlugins(pluginDir)
	if err != nil {
		return fmt.Errorf("scan plugins: %w", err)
	}

	m.log.Finfo("plugin: found %d plugins in %s", len(pluginPaths), pluginDir)

	for _, path := range pluginPaths {
		if err := m.loadPlugin(path); err != nil {
			m.log.Fwarn("plugin: failed to load %s: %v", filepath.Base(path), err)
			continue
		}
	}

	return nil
}

func (m *Manager) loadPlugin(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := filepath.Base(path)

	if m.exclude[name] {
		m.log.Finfo("plugin: skipping excluded plugin %s", name)
		return nil
	}

	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s already loaded", name)
	}

	m.log.Finfo("plugin: loading %s", name)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  shared.Handshake,
		Plugins:          shared.PluginMap,
		Cmd:              pluginCmd(path),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolNetRPC},
		Logger: hclog.New(&hclog.LoggerOptions{
			Output: &logWriter{log: m.log},
			Level:  hclog.Info,
		}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("connect to plugin: %w", err)
	}

	raw, err := rpcClient.Dispense("pegg")
	if err != nil {
		client.Kill()
		return fmt.Errorf("dispense plugin: %w", err)
	}

	pluginImpl, ok := raw.(shared.Plugin)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin does not implement Plugin interface")
	}

	instance := &PluginInstance{
		Name:        pluginImpl.Name(),
		Path:        path,
		Version:     pluginImpl.Version(),
		Description: pluginImpl.Description(),
		Client:      client,
		Plugin:      pluginImpl,
	}

	deps := shared.Deps{
		Config: m.cfg,
	}
	if err := pluginImpl.Boot(deps); err != nil {
		client.Kill()
		return fmt.Errorf("boot plugin: %w", err)
	}

	m.plugins[instance.Name] = instance
	m.clients[instance.Name] = client

	m.log.Finfo("plugin: loaded %s v%s (%s)", instance.Name, instance.Version, instance.Description)

	return nil
}

func (m *Manager) GetPlugin(name string) (*PluginInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	return p, ok
}

func (m *Manager) ListPlugins() []*PluginInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]*PluginInstance, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	return plugins
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		m.log.Finfo("plugin: shutting down %s", name)
		client.Kill()
	}

	m.plugins = make(map[string]*PluginInstance)
	m.clients = make(map[string]*goplugin.Client)
}

func GetPluginDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".pegg", "plugins"), nil
}

func pluginCmd(path string) *exec.Cmd {
	return exec.Command(path)
}
