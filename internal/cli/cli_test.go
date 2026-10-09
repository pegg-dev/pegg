package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/credentials"
	"github.com/peggco/pegg/internal/llm/subscription"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func testLogger() *logger.Logger {
	return logger.New(logger.LevelDebug, discardHandler{})
}

func newTestCLI(t *testing.T) (*CLI, event.Bus) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	bus := event.New()

	cacheStore, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cacheStore.Close() })

	fs, err := vfs.New(t.TempDir(), vfs.Options{})
	if err != nil {
		t.Fatal(err)
	}

	mgr := llm.NewManager(bus, testLogger(), cacheStore)
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)

	cfg := config.DefaultConfig()
	cfg.Providers = nil
	cfg.Logger.Driver = "console"
	cfg.Cache.Driver = "json"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	sess, err := session.SessionModule(cfg.Session, bus, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	c := New(bus, cfg, testLogger(), fs, sess, mgr, nil, nil, nil, nil, nil, nil)
	return c, bus
}

func registerTestProvider(t *testing.T, name string, models ...llm.Model) {
	t.Helper()
	llm.RegisterProvider(name, func(config.LLMConfig) (llm.Provider, error) {
		return &cliTestProvider{name: name, models: models}, nil
	})
}

type cliTestProvider struct {
	name   string
	models []llm.Model
}

func (p *cliTestProvider) Name() string { return p.name }
func (p *cliTestProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}
func (p *cliTestProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return nil
}
func (p *cliTestProvider) ListModels(context.Context) ([]llm.Model, error) {
	return p.models, nil
}

func TestLoginSavesConfigAndPublishesEvent(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestProvider(t, "testprov")

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{"login", "--provider", "testprov", "--api-key", "sk-123"}); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-added:
		if cfg.Provider != "testprov" || cfg.APIKey != "sk-123" {
			t.Fatalf("unexpected event cfg: %+v", cfg)
		}
	default:
		t.Fatal("provider.added event not published")
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 ||
		loaded.Providers[0].Provider != "testprov" ||
		loaded.Providers[0].APIKey != "sk-123" {
		t.Fatalf("config providers: %+v", loaded.Providers)
	}
}

func TestLoginSubscriptionProvider(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestProvider(t, "fakesub")
	subscription.Register(subscription.Info{
		Provider: "fakesub",
		Hint:     "run fake-cli",
		Status:   func() credentials.Status { return credentials.Status{Provider: "fakesub", LoggedIn: true} },
	})

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	if err := c.runLogin("fakesub", "", func() (string, error) { return "", nil }, func() (string, error) { return "", nil }); err != nil {
		t.Fatal(err)
	}
	select {
	case cfg := <-added:
		if cfg.Provider != "fakesub" || cfg.APIKey != "" {
			t.Fatalf("cfg = %+v, want provider fakesub with no api key", cfg)
		}
	default:
		t.Fatal("provider.added not published")
	}

	if err := c.runLogout("fakesub"); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 0 {
		t.Fatalf("providers after logout = %+v, want none", loaded.Providers)
	}
}

func TestLoginSubscriptionNotSignedIn(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "fakesub2")
	subscription.Register(subscription.Info{
		Provider: "fakesub2",
		Hint:     "run fake-cli",
		Status:   func() credentials.Status { return credentials.Status{Provider: "fakesub2", LoggedIn: false} },
	})
	if err := c.runLogin("fakesub2", "", func() (string, error) { return "", nil }, func() (string, error) { return "", nil }); err == nil {
		t.Fatal("expected error when the subscription is not signed in")
	}
}

func TestLoginUnknownProvider(t *testing.T) {
	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"login", "--provider", "nope", "--api-key", "x"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestLoginInteractive(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestProvider(t, "interactiveprov")

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	selectProvider := func() (string, error) { return "interactiveprov", nil }
	promptAPIKey := func() (string, error) { return "mykey", nil }
	if err := c.runLogin("", "", selectProvider, promptAPIKey); err != nil {
		t.Fatal(err)
	}

	cfg := <-added
	if cfg.Provider != "interactiveprov" || cfg.APIKey != "mykey" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoginInteractiveEmptyAPIKey(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "emptyprov")

	selectProvider := func() (string, error) { return "emptyprov", nil }
	promptAPIKey := func() (string, error) { return "", nil }
	if err := c.runLogin("", "", selectProvider, promptAPIKey); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 ||
		loaded.Providers[0].Provider != "emptyprov" ||
		loaded.Providers[0].APIKey != "" {
		t.Fatalf("config providers: %+v", loaded.Providers)
	}
}

func TestOnRegisterCommandHook(t *testing.T) {
	c, _ := newTestCLI(t)
	c.OnRegisterCommand(func(cmds []*cobra.Command) []*cobra.Command {
		return append(cmds, &cobra.Command{
			Use: "hello",
			RunE: func(_ *cobra.Command, _ []string) error {
				return nil
			},
		})
	})

	if err := c.Execute([]string{"hello"}); err != nil {
		t.Fatal(err)
	}
}

type errTestProvider struct{}

func (e *errTestProvider) Name() string { return "badprov" }
func (e *errTestProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}
func (e *errTestProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return nil
}
func (e *errTestProvider) ListModels(context.Context) ([]llm.Model, error) {
	return nil, errors.New("api error: invalid api key")
}

func TestLoginSyncFailureDoesNotSave(t *testing.T) {
	c, _ := newTestCLI(t)

	llm.RegisterProvider("badprov", func(config.LLMConfig) (llm.Provider, error) {
		return &errTestProvider{}, nil
	})

	err := c.Execute([]string{"login", "--provider", "badprov", "--api-key", "bad"})
	if err == nil {
		t.Fatal("expected error for failing provider sync")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 0 {
		t.Fatalf("providers = %+v, want none saved", loaded.Providers)
	}
}

func TestLogsConsoleBuffer(t *testing.T) {
	c, _ := newTestCLI(t)
	c.log.Info("hello from test")

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"logs"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello from test") {
		t.Fatalf("output = %q, want hello from test", buf.String())
	}
}

func TestLogsSQLite(t *testing.T) {
	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.Logger = config.LoggerConfig{Driver: "sqlite", MaxLogCount: 100}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	log, err := logger.LoggerModule(cfg.Logger)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hello sqlite")
	log.Error("boom happened")
	log.Close()

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"logs", "--search", "boom"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "boom happened") {
		t.Fatalf("output = %q, want boom happened", buf.String())
	}
	if strings.Contains(buf.String(), "hello sqlite") {
		t.Fatalf("output = %q, should not contain hello sqlite (filtered by search)", buf.String())
	}
}

func TestFilesCommand(t *testing.T) {
	c, _ := newTestCLI(t)

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"files"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, label := range []string{"config", "logs", "cache", "sessions"} {
		if !strings.Contains(out, label) {
			t.Fatalf("output missing %q: %q", label, out)
		}
	}
}

func TestCacheClear(t *testing.T) {
	c, _ := newTestCLI(t)

	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("groq", []byte(`[{"id":"m1"}]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openai", []byte(`[{"id":"o1"}]`)); err != nil {
		t.Fatal(err)
	}
	store.Close()

	if err := c.Execute([]string{"cache", "clear"}); err != nil {
		t.Fatal(err)
	}

	reopened, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Get("groq"); err != cache.ErrNotFound {
		t.Fatalf("groq cache not cleared: %v", err)
	}
	if _, err := reopened.Get("openai"); err != cache.ErrNotFound {
		t.Fatalf("openai cache not cleared: %v", err)
	}
}

func TestCacheClearByProvider(t *testing.T) {
	c, _ := newTestCLI(t)

	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("groq", []byte(`[{"id":"m1"}]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("openai", []byte(`[{"id":"o1"}]`)); err != nil {
		t.Fatal(err)
	}
	store.Close()

	if err := c.Execute([]string{"cache", "clear", "--provider", "groq"}); err != nil {
		t.Fatal(err)
	}

	reopened, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Get("groq"); err != cache.ErrNotFound {
		t.Fatalf("groq cache not cleared: %v", err)
	}
	if _, err := reopened.Get("openai"); err != nil {
		t.Fatalf("openai cache should remain: %v", err)
	}
}

func TestProviderListCommand(t *testing.T) {
	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{Provider: "groq", APIKey: "gsk-secret12345"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("groq", []byte(`[{"id":"llama-3.3-70b-versatile"}]`)); err != nil {
		t.Fatal(err)
	}
	store.Close()

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"providers", "list"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "groq") || !strings.Contains(out, "gsk-****2345") {
		t.Fatalf("output = %q", out)
	}
}

func TestProviderRemove(t *testing.T) {
	c, bus := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{Provider: "groq", APIKey: "k"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	removed := make(chan string, 1)
	if err := bus.Subscribe(event.TopicProviderRemoved, func(name string) { removed <- name }); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{"providers", "remove", "groq"}); err != nil {
		t.Fatal(err)
	}
	if name := <-removed; name != "groq" {
		t.Fatalf("removed = %q, want groq", name)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 0 {
		t.Fatalf("providers = %+v, want none", loaded.Providers)
	}
}

func TestProviderRemoveMissing(t *testing.T) {
	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"providers", "remove", "nope"}); err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestModelsCommand(t *testing.T) {
	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{Provider: "groq", APIKey: "k"}}
	cfg.Cache.Driver = "json"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := cache.NewJSONCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("groq", []byte(`[{"id":"llama-3.3-70b-versatile"},{"id":"mixtral-8x7b"}]`)); err != nil {
		t.Fatal(err)
	}
	store.Close()

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"models"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "llama-3.3-70b-versatile") || !strings.Contains(out, "mixtral-8x7b") {
		t.Fatalf("output = %q", out)
	}
}

func TestDoctorCommand(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "docprov", llm.Model{ID: "m1"})

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{Provider: "docprov", APIKey: "k"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Fatalf("output = %q, want OK", buf.String())
	}
}

func TestConfigShowMasksAPIKey(t *testing.T) {
	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{Provider: "groq", APIKey: "gsk-secret12345"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"config", "show"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "gsk-****2345") {
		t.Fatalf("output = %q, want masked key", out)
	}
	if strings.Contains(out, "gsk-secret12345") {
		t.Fatalf("output = %q, must not contain raw key", out)
	}
}

func TestVersionCommand(t *testing.T) {
	c, _ := newTestCLI(t)

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"version"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), config.AppVersion) {
		t.Fatalf("output = %q, want %q", buf.String(), config.AppVersion)
	}
}

func TestSessionCommand(t *testing.T) {
	c, _ := newTestCLI(t)

	s, err := c.sessions.Create(session.CreateOptions{Title: "hello", Provider: "groq", Model: "m1", ProjectDir: "/proj"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.UserMessage("hi")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.sessions.AppendMessage(s.ID, llm.AssistantMessage("hello")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)

	if err := c.Execute([]string{"sessions", "list", "--search", "hello"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), s.ID) {
		t.Fatalf("list default should be project-scoped, got %q", buf.String())
	}
	buf.Reset()

	if err := c.Execute([]string{"sessions", "list", "--all", "--search", "hello"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), s.ID) {
		t.Fatalf("list --all output = %q", buf.String())
	}
	buf.Reset()

	if err := c.Execute([]string{"sessions", "show", s.ID}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[assistant] hello") {
		t.Fatalf("show output = %q", buf.String())
	}
	buf.Reset()

	if err := c.Execute([]string{"sessions", "show", "nope"}); err == nil {
		t.Fatal("expected error for missing session")
	}
}
