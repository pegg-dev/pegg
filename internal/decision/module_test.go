package decision

import (
	"testing"

	"github.com/peggco/pegg/internal/core/config"
)

func TestRegisterAndListProviders(t *testing.T) {
	name := "module-test"
	RegisterProvider(name, func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	if !HasProvider(name) {
		t.Fatal("expected provider to be registered")
	}
	found := false
	for _, n := range ListProviders() {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("provider %q missing from list: %v", name, ListProviders())
	}
}

func TestRegisterIgnoresEmpty(t *testing.T) {
	before := len(ListProviders())
	RegisterProvider("", func(cfg config.LLMConfig) (Provider, error) { return nil, nil })
	RegisterProvider("ignored-nil", nil)
	if len(ListProviders()) != before {
		t.Fatal("empty registrations must be ignored")
	}
}

func TestResolveProvider(t *testing.T) {
	name := "module-resolve"
	RegisterProvider(name, func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: cfg.Provider}, nil
	})

	prov, err := resolveProvider(config.LLMConfig{Provider: name})
	if err != nil {
		t.Fatal(err)
	}
	if prov.Name() != name {
		t.Fatalf("name = %q", prov.Name())
	}

	if _, err := resolveProvider(config.LLMConfig{Provider: "module-nope"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if _, err := resolveProvider(config.LLMConfig{}); err == nil {
		t.Fatal("expected error for empty provider")
	}
}
