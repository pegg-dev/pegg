package decision

import (
	"context"
	"errors"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
)

type mockProvider struct {
	name   string
	decide func(ctx context.Context, req *Request) (*Response, error)
}

func (p *mockProvider) Name() string { return p.name }
func (p *mockProvider) Decide(ctx context.Context, req *Request) (*Response, error) {
	if p.decide != nil {
		return p.decide(ctx, req)
	}
	return &Response{Model: req.Model, Answers: map[string]Answer{}}, nil
}

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func newTestManager(t *testing.T) (*Manager, event.Bus) {
	t.Helper()
	bus := event.New()
	mgr := NewManager(bus, logger.New(logger.LevelError, discardHandler{}))
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Shutdown)
	return mgr, bus
}

func TestManagerLoadsRegisteredProvidersFromAppMounted(t *testing.T) {
	RegisterProvider("mgr-test", func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	mgr, bus := newTestManager(t)
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{
			{Provider: "mgr-test"},
			{Provider: "no-factory"},
		},
	})

	prov, err := mgr.Provider("mgr-test")
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if prov.Name() != "mgr-test" {
		t.Fatalf("name = %q", prov.Name())
	}

	if _, err := mgr.Provider("no-factory"); err == nil {
		t.Fatal("expected error for provider without a registered factory")
	}
}

func TestManagerProviderAddedAndRemoved(t *testing.T) {
	RegisterProvider("mgr-dyn", func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	mgr, bus := newTestManager(t)
	if _, err := mgr.Provider("mgr-dyn"); err == nil {
		t.Fatal("expected provider to be unloaded before app.mounted")
	}

	bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "mgr-dyn"})
	bus.WaitAsync()
	if _, err := mgr.Provider("mgr-dyn"); err != nil {
		t.Fatalf("provider after add: %v", err)
	}

	bus.Publish(event.TopicProviderRemoved, "mgr-dyn")
	if _, err := mgr.Provider("mgr-dyn"); err == nil {
		t.Fatal("expected provider to be removed")
	}
}

func TestManagerSkipsUnregisteredProviderAdded(t *testing.T) {
	mgr, bus := newTestManager(t)
	bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "mgr-unknown"})
	bus.WaitAsync()
	if _, err := mgr.Provider("mgr-unknown"); err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}

func TestManagerSurfacesFactoryError(t *testing.T) {
	factoryErr := errors.New("boom")
	RegisterProvider("mgr-bad", func(cfg config.LLMConfig) (Provider, error) {
		return nil, factoryErr
	})

	mgr, bus := newTestManager(t)
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: "mgr-bad"}},
	})

	if _, err := mgr.Provider("mgr-bad"); !errors.Is(err, factoryErr) {
		t.Fatalf("expected factory error, got %v", err)
	}
}

func TestManagerProviderWithoutManagerEntries(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := mgr.Provider("missing"); err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestManagerPreferred(t *testing.T) {
	RegisterProvider("mgr-pref", func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	mgr, bus := newTestManager(t)
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{{Provider: "mgr-pref"}},
	})

	if _, err := mgr.Preferred(); err == nil {
		t.Fatal("expected no preferred provider without an api key")
	}

	bus.Publish(event.TopicProviderAdded, config.LLMConfig{Provider: "mgr-pref", APIKey: "sk-1"})
	bus.WaitAsync()
	prov, err := mgr.Preferred()
	if err != nil {
		t.Fatalf("preferred: %v", err)
	}
	if prov.Name() != "mgr-pref" {
		t.Fatalf("name = %q", prov.Name())
	}
}

func TestManagerPreferredSorted(t *testing.T) {
	RegisterProvider("mgr-pref-zzz", func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})
	RegisterProvider("mgr-pref-aaa", func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	mgr, bus := newTestManager(t)
	bus.Publish(event.TopicAppMounted, &config.Config{
		Providers: []config.LLMConfig{
			{Provider: "mgr-pref-zzz", APIKey: "sk-z"},
			{Provider: "mgr-pref-aaa", APIKey: "sk-a"},
		},
	})

	prov, err := mgr.Preferred()
	if err != nil {
		t.Fatalf("preferred: %v", err)
	}
	if prov.Name() != "mgr-pref-aaa" {
		t.Fatalf("name = %q, want deterministic alphabetical pick", prov.Name())
	}
}
