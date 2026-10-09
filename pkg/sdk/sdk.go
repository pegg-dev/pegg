package sdk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/builtin"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/router"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"

	_ "github.com/peggco/pegg/internal/decision/providers"
	_ "github.com/peggco/pegg/internal/llm/drivers"
	_ "github.com/peggco/pegg/internal/llm/providers"
)

type Options struct {
	Config          *Config
	ConfigDir       string
	APIKeys         map[string]string
	Providers       []LLMConfig
	Workspace       string
	SessionDB       string
	SessionDir      string
	Cache           cache.Cache
	Logger          *Logger
	DisableBuiltins bool
	DisableRecorder bool
}

type Engine struct {
	opts      Options
	cfg       *Config
	bus       event.Bus
	log       *Logger
	llm       *llm.Manager
	decisions *decision.Manager
	memory    *memory.Manager
	router    *router.Router
	sessions  *session.Manager
	rec       *session.Recorder
	fs        *VFS
	store     cache.Cache
	mu        sync.Mutex
	closed    bool
}

var (
	engineMu     sync.Mutex
	engineOpen   bool
	builtinsOnce sync.Once
)

func Open(ctx context.Context, opts Options) (*Engine, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	engineMu.Lock()
	if engineOpen {
		engineMu.Unlock()
		return nil, ErrAlreadyOpen
	}
	engineOpen = true
	engineMu.Unlock()

	e := &Engine{opts: opts}
	if err := e.init(); err != nil {
		e.cleanup()
		engineMu.Lock()
		engineOpen = false
		engineMu.Unlock()
		return nil, err
	}
	return e, nil
}

func (e *Engine) init() error {
	cfg, err := e.loadConfig()
	if err != nil {
		return fmt.Errorf("sdk: load config: %w", err)
	}
	cfg = cloneConfig(cfg)
	applyAPIKeys(cfg, e.opts.APIKeys)
	cfg.Providers = append(cfg.Providers, e.opts.Providers...)
	e.cfg = cfg

	e.log = e.opts.Logger
	if e.log == nil {
		e.log = logger.New(logger.LevelWarn)
	}

	e.bus = event.New()

	e.store = e.opts.Cache
	if e.store == nil {
		e.store = NewMemoryCache()
	}

	e.llm = llm.NewManager(e.bus, e.log, e.store)
	if err := e.llm.Start(); err != nil {
		return fmt.Errorf("sdk: start llm manager: %w", err)
	}

	e.decisions = decision.NewManager(e.bus, e.log)
	if err := e.decisions.Start(); err != nil {
		return fmt.Errorf("sdk: start decision manager: %w", err)
	}

	e.router = router.New(router.Deps{Config: cfg, LLM: e.llm, Decision: e.decisions}, e.log)

	e.memory = memory.New(memory.Deps{
		Config:   cfg.Memory,
		LLM:      e.llm,
		Decision: e.decisions,
		Bus:      e.bus,
		Log:      e.log,
	})

	store, err := e.openSessionStore()
	if err != nil {
		return fmt.Errorf("sdk: open session store: %w", err)
	}
	e.sessions = session.NewManager(store, e.bus, e.log)

	projectDir := e.workspaceRoot()
	e.llm.SetSessionResolver(func() (string, string, bool) {
		return e.sessionResolver(projectDir)
	})

	if !e.opts.DisableRecorder {
		e.rec = session.NewRecorder(e.sessions, e.log)
		if err := e.rec.Start(e.bus); err != nil {
			return fmt.Errorf("sdk: start session recorder: %w", err)
		}
	}

	fs, err := vfs.New(e.workspaceRoot(), vfs.Options{Log: e.log})
	if err != nil {
		return fmt.Errorf("sdk: mount workspace: %w", err)
	}
	e.fs = fs

	if !e.opts.DisableBuiltins {
		builtinsOnce.Do(func() {
			_ = builtin.Create(fs, e.sessions, builtin.Options{LLM: e.llm, Decision: e.decisions, Config: cfg, Bus: e.bus, Memory: e.memory})
		})
	}

	_ = e.store.Set(llm.PricesCacheKey, []byte("{}"))

	e.bus.Publish(event.TopicAppMounted, cfg)

	return nil
}

func (e *Engine) cleanup() {
	if e == nil {
		return
	}
	if e.rec != nil {
		_ = e.rec.Stop(e.bus)
	}
	if e.memory != nil {
		e.memory.Stop()
	}
	if e.llm != nil {
		e.llm.Shutdown()
	}
	if e.decisions != nil {
		e.decisions.Shutdown()
	}
	if e.sessions != nil {
		_ = e.sessions.Close()
	}
	if e.store != nil {
		_ = e.store.Close()
	}
	if e.log != nil {
		e.log.Close()
	}
}

func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.cleanup()
	e.closed = true
	e.mu.Unlock()

	engineMu.Lock()
	engineOpen = false
	engineMu.Unlock()
	return nil
}

func (e *Engine) Config() *Config {
	return e.cfg
}

func (e *Engine) Workspace() *VFS {
	return e.fs
}

func (e *Engine) Bus() event.Bus {
	return e.bus
}

func (e *Engine) loadConfig() (*Config, error) {
	if e.opts.Config != nil {
		return e.opts.Config, nil
	}
	if e.opts.ConfigDir == "" {
		return config.Load()
	}
	path := filepath.Join(e.opts.ConfigDir, config.GlobalConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.DefaultConfig(), nil
		}
		return nil, fmt.Errorf("sdk: read config %q: %w", path, err)
	}
	cfg := config.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("sdk: parse config %q: %w", path, err)
	}
	return cfg, nil
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	cp.Providers = append([]LLMConfig(nil), cfg.Providers...)
	return &cp
}

func applyAPIKeys(cfg *Config, keys map[string]string) {
	for name, key := range keys {
		matched := false
		for i := range cfg.Providers {
			if cfg.Providers[i].Provider == name || cfg.Providers[i].Driver == name {
				cfg.Providers[i].APIKey = key
				matched = true
				break
			}
		}
		if !matched {
			cfg.Providers = append(cfg.Providers, LLMConfig{Provider: name, APIKey: key})
		}
	}
}

func (e *Engine) workspaceRoot() string {
	if e.opts.Workspace != "" {
		return e.opts.Workspace
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func (e *Engine) openSessionStore() (session.Store, error) {
	switch {
	case e.opts.SessionDB != "":
		return session.NewSQLiteStoreAt(e.opts.SessionDB)
	case e.opts.SessionDir != "":
		return session.NewJSONStoreAt(e.opts.SessionDir)
	default:
		dir, err := os.MkdirTemp("", "pegg-sessions")
		if err != nil {
			return nil, fmt.Errorf("sdk: create temp session dir: %w", err)
		}
		return session.NewJSONStoreAt(dir)
	}
}

func (e *Engine) sessionResolver(projectDir string) (string, string, bool) {
	msgs, _, err := e.sessions.List(sessionListQuery(projectDir))
	if err != nil {
		return "", "", false
	}
	for _, s := range msgs {
		if s.ParentID != "" {
			continue
		}
		if s.Provider == "" || s.Model == "" {
			continue
		}
		return s.Provider, s.Model, true
	}
	return "", "", false
}

func (e *Engine) checkOpen() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}
