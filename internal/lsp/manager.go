package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/lsp/diagnostic"
	"github.com/peggco/pegg/internal/vfs"
)

type Manager struct {
	mu               sync.Mutex
	servers          map[string]*Server
	cfg              map[string]config.LanguageServerConfig
	diags            map[string]diagEntry
	fs               *vfs.VFS
	log              *logger.Logger
	closed           bool
	nextDoc          int
	idleTimeout      time.Duration
	done             chan struct{}
	transportFactory func(StdioOptions) (Transport, error)
}

const DefaultIdleTimeout = 10 * time.Minute

type diagEntry struct {
	version int
	diags   []diagnostic.Diagnostic
}

func NewManager(fs *vfs.VFS, log *logger.Logger) *Manager {
	m := &Manager{
		servers: make(map[string]*Server),
		cfg:     make(map[string]config.LanguageServerConfig),
		diags:   make(map[string]diagEntry),
		fs:      fs,
		log:     log,
		transportFactory: func(opts StdioOptions) (Transport, error) {
			return NewStdioTransport(opts)
		},
		idleTimeout: DefaultIdleTimeout,
		done:        make(chan struct{}),
	}
	m.registerHooks()
	m.startReaper()
	return m
}

func (m *Manager) SetTransportFactory(fn func(StdioOptions) (Transport, error)) {
	m.transportFactory = fn
}

func (m *Manager) SetIdleTimeout(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d > 0 {
		m.idleTimeout = d
	} else {
		m.idleTimeout = DefaultIdleTimeout
	}
}

func (m *Manager) startReaper() {
	go func() {
		for {
			select {
			case <-m.done:
				return
			case <-time.After(m.reapTick()):
				m.reapIdle()
			}
		}
	}()
}

func (m *Manager) reapTick() time.Duration {
	m.mu.Lock()
	d := m.idleTimeout / 4
	m.mu.Unlock()
	if d < 100*time.Millisecond {
		d = 100 * time.Millisecond
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (m *Manager) idleLimit() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idleTimeout
}

func (m *Manager) reapIdle() {
	for _, srv := range m.snapshotServers() {
		if !srv.IsRunning() {
			continue
		}
		if srv.IdleFor() < m.idleLimit() {
			continue
		}
		if m.log != nil {
			m.log.Fdebug("lsp: closing idle server %q", srv.name)
		}
		srv.stop()
	}
}

func (m *Manager) Start(servers map[string]config.LanguageServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, cfg := range servers {
		m.cfg[name] = cfg
	}
}

func (m *Manager) registerHooks() {
	m.fs.OnBeforeRead(func(path, content string) string {
		m.touch(path)
		diags := m.Diagnostics(path)
		if len(diags) == 0 {
			return content
		}
		content += "\n<file_diagnostics>\n"
		for _, d := range diags {
			content += fmt.Sprintf("%s:%d:%d: %s [severity=%d]\n", path, d.Line(), d.Column(), d.Message, d.Severity)
		}
		content += "</file_diagnostics>\n"
		return content
	})
	m.fs.OnAfterWrite(func(path, content string) string {
		m.touch(path)
		diags := m.notifyDiagnostics(path)
		if len(diags) == 0 {
			return content
		}
		content += "\n<file_diagnostics>\n"
		for _, d := range diags {
			content += fmt.Sprintf("%s:%d:%d: %s [severity=%d]\n", path, d.Line(), d.Column(), d.Message, d.Severity)
		}
		content += "</file_diagnostics>\n"
		return content
	})
	m.fs.OnFileDelete(func(ev vfs.FileDelete) vfs.FileDelete {
		m.deleteDoc(ev.Path)
		return ev
	})
}

func (m *Manager) touch(path string) {
	for name, cfg := range m.cfg {
		srv := m.lookup(name)
		if srv != nil && srv.IsFailed() {
			continue
		}
		if srv != nil && !srv.Matches(path) {
			continue
		}
		if srv == nil && !matchesConfig(cfg, path) {
			continue
		}
		m.ensureRunning(name, path)
	}
}

func (m *Manager) ensureRunning(name, path string) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	srv := m.servers[name]
	if srv == nil {
		srv = NewServer(name, m.cfg[name])
		m.servers[name] = srv
		srv.mu.Lock()
		srv.starting = true
		srv.mu.Unlock()
		m.mu.Unlock()
		go m.startServer(srv, path)
		return
	}
	srv.mu.Lock()
	if srv.starting {
		srv.mu.Unlock()
		m.mu.Unlock()
		return
	}
	if srv.client == nil && !srv.failed {
		srv.starting = true
		srv.mu.Unlock()
		m.mu.Unlock()
		go m.startServer(srv, path)
		return
	}
	srv.mu.Unlock()
	m.mu.Unlock()
	srv.Touch()
	m.openIfNeeded(srv, path)
}

func (m *Manager) startServer(srv *Server, path string) {
	srv.mu.Lock()
	srv.starting = false
	srv.lastUsed = time.Now()
	srv.mu.Unlock()

	root, err := m.findRoot(path, srv.cfg.RootMarkers)
	if err != nil {
		m.markFailed(srv, err)
		return
	}
	binary, err := ResolveBinary(srv.name, srv.cfg)
	if err != nil {
		m.markFailed(srv, err)
		return
	}
	tr, err := m.transportFactory(StdioOptions{
		Command: binary,
		Args:    srv.cfg.Args,
		Env:     srv.cfg.Env,
		Dir:     root,
		Stderr:  m.log.Writer(logger.LevelDebug, "lsp["+srv.name+"]: "),
	})
	if err != nil {
		m.markFailed(srv, err)
		return
	}

	client := NewClient(tr, ClientOptions{Name: srv.name, OnDiag: m.onDiagnostics})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		m.markFailed(srv, err)
		return
	}

	srv.mu.Lock()
	srv.client = client
	srv.started = true
	srv.starting = false
	srv.mu.Unlock()

	if m.log != nil {
		m.log.Finfo("lsp: server %q started (root %s)", srv.name, root)
	}
	m.openIfNeeded(srv, path)
}

func (m *Manager) openIfNeeded(srv *Server, path string) {
	srv.mu.Lock()
	if srv.client == nil || !srv.started {
		srv.mu.Unlock()
		return
	}
	if srv.opened[path] {
		srv.mu.Unlock()
		return
	}
	srv.opened[path] = true
	client := srv.client
	srv.mu.Unlock()

	item := m.buildTextDocument(path)
	if item != nil {
		go func() { _ = client.DidOpen(context.Background(), *item) }()
	}
}

func (m *Manager) notifyDiagnostics(path string) []diagnostic.Diagnostic {
	before := m.diagVersion(path)
	notified := false
	for _, srv := range m.snapshotServers() {
		if !srv.IsRunning() || !srv.Matches(path) {
			continue
		}
		if content, ok := m.readContent(path); ok {
			m.sendSaveChange(srv, path, content)
			notified = true
		}
	}
	if !notified {
		deadline := time.After(5 * time.Second)
		for {
			diags := m.Diagnostics(path)
			if len(diags) > 0 {
				return diags
			}
			select {
			case <-deadline:
				return m.Diagnostics(path)
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return m.waitDiagnostics(path, before)
}

func (m *Manager) sendSaveChange(srv *Server, path, content string) {
	uri := m.fileURI(path)
	version := m.nextDocVersion()
	ctx := context.Background()
	srv.mu.Lock()
	client := srv.client
	srv.mu.Unlock()
	if client == nil {
		return
	}
	go func() {
		_ = client.DidChange(ctx, uri, version, content)
		_ = client.DidSave(ctx, uri, content)
	}()
}

func (m *Manager) deleteDoc(path string) {
	m.mu.Lock()
	delete(m.diags, path)
	m.mu.Unlock()
	for _, srv := range m.snapshotServers() {
		if !srv.IsRunning() || !srv.Matches(path) {
			continue
		}
		uri := m.fileURI(path)
		srv.mu.Lock()
		client := srv.client
		srv.mu.Unlock()
		if client == nil {
			continue
		}
		go func() { _ = client.DidClose(context.Background(), uri) }()
	}
}

func (m *Manager) onDiagnostics(uri string, _ int, diags []diagnostic.Diagnostic) {
	path := m.virtualPath(uri)
	if path == "" {
		return
	}
	m.mu.Lock()
	var entry diagEntry
	if existing, ok := m.diags[path]; ok {
		entry = existing
	}
	entry.version++
	entry.diags = diags
	m.diags[path] = entry
	m.mu.Unlock()
}

func (m *Manager) Diagnostics(path string) []diagnostic.Diagnostic {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.diags[path]
	if !ok {
		return nil
	}
	out := make([]diagnostic.Diagnostic, len(entry.diags))
	copy(out, entry.diags)
	return out
}

func (m *Manager) Running() []string {
	var out []string
	for _, srv := range m.snapshotServers() {
		if srv.IsRunning() {
			out = append(out, srv.name)
		}
	}
	return out
}

func (m *Manager) diagVersion(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.diags[path]; ok {
		return entry.version
	}
	return 0
}

func (m *Manager) waitDiagnostics(path string, before int) []diagnostic.Diagnostic {
	deadline := time.After(2 * time.Second)
	for {
		m.mu.Lock()
		entry, ok := m.diags[path]
		changed := ok && entry.version > before
		m.mu.Unlock()
		if changed {
			return m.Diagnostics(path)
		}
		select {
		case <-deadline:
			return m.Diagnostics(path)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (m *Manager) findRoot(path string, markers []string) (string, error) {
	phys, err := m.fs.Resolve(path)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(phys)
	root := m.fs.Root()
	for {
		if hasAnyMarker(dir, markers) {
			return dir, nil
		}
		if dir == root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return root, nil
}

func (m *Manager) buildTextDocument(path string) *TextDocumentItem {
	phys, err := m.fs.Resolve(path)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(phys)
	if err != nil {
		return nil
	}
	return &TextDocumentItem{
		URI:        m.fileURI(path),
		LanguageID: languageID(path),
		Version:    1,
		Text:       string(data),
	}
}

func (m *Manager) readContent(path string) (string, bool) {
	phys, err := m.fs.Resolve(path)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(phys)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func (m *Manager) fileURI(path string) string {
	phys, err := m.fs.Resolve(path)
	if err != nil {
		return "file://" + filepath.ToSlash(path)
	}
	return "file://" + filepath.ToSlash(phys)
}

func (m *Manager) virtualPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		uri = uri[len("file://"):]
	}
	uri = filepath.FromSlash(uri)
	return m.fs.Virtual(uri)
}

func (m *Manager) nextDocVersion() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextDoc++
	return m.nextDoc
}

func (m *Manager) snapshotServers() []*Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Server, 0, len(m.servers))
	for _, srv := range m.servers {
		out = append(out, srv)
	}
	return out
}

func (m *Manager) lookup(name string) *Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.servers[name]
}

func (m *Manager) markFailed(srv *Server, err error) {
	srv.mu.Lock()
	srv.failed = true
	srv.starting = false
	srv.mu.Unlock()
	if m.log != nil {
		m.log.Fwarn("lsp: server %q failed to start: %v", srv.name, err)
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.done)
	servers := make([]*Server, 0, len(m.servers))
	for _, srv := range m.servers {
		servers = append(servers, srv)
	}
	m.mu.Unlock()

	for _, srv := range servers {
		srv.stop()
	}
	if m.log != nil {
		m.log.Debug("lsp: all servers closed")
	}
	return nil
}

func matchesConfig(cfg config.LanguageServerConfig, path string) bool {
	ext := extensionOf(path)
	for _, ft := range cfg.FileTypes {
		if ft == ext {
			return true
		}
	}
	return false
}

func hasAnyMarker(dir string, markers []string) bool {
	if len(markers) == 0 {
		return false
	}
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func languageID(path string) string {
	switch extensionOf(path) {
	case "go":
		return "go"
	case "ts", "tsx":
		return "typescript"
	case "py":
		return "python"
	}
	return ""
}
