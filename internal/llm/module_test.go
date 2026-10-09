package llm

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestResolveProviderNeitherSet(t *testing.T) {
	_, err := resolveProvider(config.LLMConfig{})
	if err == nil {
		t.Fatal("expected error when neither provider nor driver is set")
	}
	if !strings.Contains(err.Error(), "neither provider nor driver") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveProviderUnknown(t *testing.T) {
	_, err := resolveProvider(config.LLMConfig{Provider: "nope"})
	if err == nil || !strings.Contains(err.Error(), "no provider registered") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = resolveProvider(config.LLMConfig{Driver: "nope"})
	if err == nil || !strings.Contains(err.Error(), "no driver registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveProviderProviderWins(t *testing.T) {
	RegisterProvider("both", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "provider"}, nil
	})
	RegisterDriver("both", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "driver"}, nil
	})

	p, err := resolveProvider(config.LLMConfig{Provider: "both", Driver: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "provider" {
		t.Fatalf("Name = %q, want provider (provider must win)", p.Name())
	}
}

func TestResolveProviderDriver(t *testing.T) {
	RegisterDriver("testdriver", func(cfg config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: cfg.Driver}, nil
	})

	p, err := resolveProvider(config.LLMConfig{Driver: "testdriver"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "testdriver" {
		t.Fatalf("Name = %q, want testdriver", p.Name())
	}
}

func TestResolveProviderCustomNamed(t *testing.T) {
	RegisterDriver("customdrv", func(cfg config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: cfg.Provider}, nil
	})

	p, err := resolveProvider(config.LLMConfig{Provider: "my-gateway", Driver: "customdrv"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "my-gateway" {
		t.Fatalf("Name = %q, want my-gateway", p.Name())
	}
}

func TestResolveProviderCustomNamedUnknownDriver(t *testing.T) {
	_, err := resolveProvider(config.LLMConfig{Provider: "my-gateway", Driver: "nope"})
	if err == nil || !strings.Contains(err.Error(), "no driver registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterDriverEmpty(t *testing.T) {
	RegisterDriver("", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "x"}, nil
	})
	RegisterDriver("nilfactory", nil)
}

func TestListProviders(t *testing.T) {
	RegisterProvider("list-b", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "list-b"}, nil
	})
	RegisterProvider("list-a", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "list-a"}, nil
	})

	names := ListProviders()
	if !slices.Contains(names, "list-a") || !slices.Contains(names, "list-b") {
		t.Fatalf("ListProviders = %v, want to contain list-a and list-b", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("ListProviders = %v, not sorted", names)
		}
	}
}

func TestHasProvider(t *testing.T) {
	RegisterProvider("has-a", func(config.LLMConfig) (Provider, error) {
		return &namedMockProvider{name: "has-a"}, nil
	})

	if !HasProvider("has-a") {
		t.Error("HasProvider(has-a) = false, want true")
	}
	if HasProvider("nope") {
		t.Error("HasProvider(nope) = true, want false")
	}
}

var errModuleTest = &moduleTestError{}

type moduleTestError struct{}

func (e *moduleTestError) Error() string { return "module test error" }

type namedMockProvider struct {
	name   string
	models []Model
}

func (m *namedMockProvider) Name() string { return m.name }
func (m *namedMockProvider) Chat(context.Context, *Request) (*Response, error) {
	return &Response{}, nil
}
func (m *namedMockProvider) ChatStream(context.Context, *Request, StreamHandler) error {
	return nil
}
func (m *namedMockProvider) ListModels(context.Context) ([]Model, error) {
	return m.models, nil
}
