package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

func registerTestDriver(t *testing.T, name string) {
	t.Helper()
	llm.RegisterDriver(name, func(config.LLMConfig) (llm.Provider, error) {
		return &cliTestProvider{name: name}, nil
	})
}

func stubPrompts(sync func(config.LLMConfig) error) providerAddPrompts {
	if sync == nil {
		sync = func(config.LLMConfig) error { return nil }
	}
	return providerAddPrompts{
		selectProvider: func() (string, error) { return "", nil },
		selectDriver:   func() (string, error) { return "", nil },
		promptAPIKey:   func() (string, error) { return "", nil },
		promptString:   func(string) (string, error) { return "", nil },
		promptHeaders:  func() (map[string]string, error) { return nil, nil },
		sync:           sync,
	}
}

func TestProviderAddNamedFullFlags(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestProvider(t, "testprov-full")

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{
		"providers", "add",
		"--name", "testprov-full",
		"--api-key", "api-key",
		"--header", "Authorization=Bearer api-key",
		"--timeout", "60",
		"--max-retries", "3",
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-added:
		if cfg.Provider != "testprov-full" {
			t.Fatalf("event provider = %q, want testprov-full", cfg.Provider)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider.added event not published")
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.Provider != "testprov-full" || p.Driver != "" {
		t.Fatalf("provider = %+v", p)
	}
	if p.APIKey != "api-key" {
		t.Fatalf("api key = %q", p.APIKey)
	}
	if p.Headers["Authorization"] != "Bearer api-key" || len(p.Headers) != 1 {
		t.Fatalf("headers = %+v", p.Headers)
	}
	if p.Timeout != 60 || p.MaxRetries != 3 {
		t.Fatalf("timeout/retries = %d/%d", p.Timeout, p.MaxRetries)
	}
}

func TestProviderAddDriverOnly(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestDriver(t, "testdrv-full")

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{
		"providers", "add",
		"--driver", "testdrv-full",
		"--base-url", "http://127.0.0.1:9/v1",
		"--api-key", "ollama",
		"--header", "Authorization=Bearer ollama",
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-added:
		if cfg.Driver != "testdrv-full" || cfg.Provider != "" {
			t.Fatalf("event cfg = %+v, want driver-only", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider.added event not published")
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.Provider != "" || p.Driver != "testdrv-full" {
		t.Fatalf("provider = %+v", p)
	}
	if p.BaseURL != "http://127.0.0.1:9/v1" || p.APIKey != "ollama" {
		t.Fatalf("base url/api key = %q/%q", p.BaseURL, p.APIKey)
	}
	if p.Headers["Authorization"] != "Bearer ollama" || len(p.Headers) != 1 {
		t.Fatalf("headers = %+v", p.Headers)
	}
}

func TestProviderAddNamedBaseURLNote(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "testprov-note")

	var out bytes.Buffer
	f := &providerAddFlags{
		name:       "testprov-note",
		apiKey:     "api-key",
		baseURL:    "http://gateway.example/v1",
		apiKeySet:  true,
		headersSet: true,
	}
	if err := c.runProviderAdd(&out, f, stubPrompts(nil)); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "note:") {
		t.Fatalf("output = %q, want base_url note", out.String())
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].Provider != "testprov-note" {
		t.Fatalf("providers = %+v", loaded.Providers)
	}
}

func TestProviderAddReplace(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "testprov-rep")

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{
		Provider: "testprov-rep",
		APIKey:   "api-key",
		Headers:  map[string]string{"X-Old": "1"},
	}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	f := &providerAddFlags{
		name:       "testprov-rep",
		apiKey:     "api-key",
		apiKeySet:  true,
		headersSet: true,
	}
	if err := c.runProviderAdd(&out, f, stubPrompts(nil)); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "updated") {
		t.Fatalf("output = %q, want updated message", out.String())
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.APIKey != "api-key" {
		t.Fatalf("api key = %q, want replaced", p.APIKey)
	}
	if len(p.Headers) != 0 {
		t.Fatalf("headers = %+v, want replaced", p.Headers)
	}
}

func TestProviderAddDriverOnlyCollision(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestDriver(t, "testdrv-col")

	cfg := config.DefaultConfig()
	cfg.Providers = []config.LLMConfig{{
		Driver:  "olddrv",
		BaseURL: "http://old.example/v1",
	}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	f := &providerAddFlags{
		driver:     "testdrv-col",
		apiKey:     "k",
		baseURL:    "http://new.example/v1",
		apiKeySet:  true,
		headersSet: true,
	}
	if err := c.runProviderAdd(&out, f, stubPrompts(nil)); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "warning: replacing existing driver-only entry") {
		t.Fatalf("output = %q, want collision warning", out.String())
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.Driver != "testdrv-col" || p.BaseURL != "http://new.example/v1" {
		t.Fatalf("provider = %+v", p)
	}
}

func TestProviderAddCustomNamedEndpoint(t *testing.T) {
	c, bus := newTestCLI(t)
	registerTestDriver(t, "testdrv-gw")

	added := make(chan config.LLMConfig, 1)
	if err := bus.Subscribe(event.TopicProviderAdded, func(cfg config.LLMConfig) { added <- cfg }); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{
		"providers", "add",
		"--name", "my-gateway",
		"--driver", "testdrv-gw",
		"--base-url", "http://127.0.0.1:9/v1",
		"--api-key", "sk-gw",
		"--header", "Authorization=Bearer sk-gw",
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-added:
		if cfg.Provider != "my-gateway" || cfg.Driver != "testdrv-gw" {
			t.Fatalf("event cfg = %+v", cfg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider.added event not published")
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.Provider != "my-gateway" || p.Driver != "testdrv-gw" {
		t.Fatalf("provider = %+v", p)
	}
	if p.BaseURL != "http://127.0.0.1:9/v1" || p.APIKey != "sk-gw" {
		t.Fatalf("base url/api key = %q/%q", p.BaseURL, p.APIKey)
	}
	if p.Headers["Authorization"] != "Bearer sk-gw" || len(p.Headers) != 1 {
		t.Fatalf("headers = %+v", p.Headers)
	}
}

func TestProviderAddDriverOnlyNoHeaders(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestDriver(t, "testdrv-nohdr")

	if err := c.Execute([]string{
		"providers", "add",
		"--driver", "testdrv-nohdr",
		"--base-url", "http://127.0.0.1:9/v1",
		"--api-key", "x",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	p := loaded.Providers[0]
	if p.Driver != "testdrv-nohdr" || p.BaseURL != "http://127.0.0.1:9/v1" || p.APIKey != "x" {
		t.Fatalf("provider = %+v", p)
	}
	if len(p.Headers) != 0 {
		t.Fatalf("headers = %+v, want none", p.Headers)
	}
}

func TestProviderAddDriverMissingBaseURL(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestDriver(t, "testdrv-nobase")

	err := c.Execute([]string{
		"providers", "add",
		"--driver", "testdrv-nobase",
		"--api-key", "x",
	})
	if err == nil || !strings.Contains(err.Error(), "base-url is required") {
		t.Fatalf("err = %v, want base-url required error", err)
	}
}

func TestProviderAddCustomEndpointInteractive(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestDriver(t, "testdrv-int")

	p := stubPrompts(nil)
	p.selectProvider = func() (string, error) { return customEndpointOption, nil }
	p.selectDriver = func() (string, error) { return "testdrv-int", nil }
	p.promptString = func(label string) (string, error) {
		if label == "Base URL" {
			return "http://127.0.0.1:9/v1", nil
		}
		return "", nil
	}
	p.promptAPIKey = func() (string, error) { return "k", nil }
	p.promptHeaders = func() (map[string]string, error) {
		return map[string]string{"X-Test": "1"}, nil
	}

	var out bytes.Buffer
	f := &providerAddFlags{}
	if err := c.runProviderAdd(&out, f, p); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	pv := loaded.Providers[0]
	if pv.Provider != "" || pv.Driver != "testdrv-int" {
		t.Fatalf("provider = %+v", pv)
	}
	if pv.BaseURL != "http://127.0.0.1:9/v1" || pv.APIKey != "k" {
		t.Fatalf("base url/api key = %q/%q", pv.BaseURL, pv.APIKey)
	}
	if pv.Headers["X-Test"] != "1" || len(pv.Headers) != 1 {
		t.Fatalf("headers = %+v", pv.Headers)
	}
}

func TestProviderAddUnknownProvider(t *testing.T) {
	c, _ := newTestCLI(t)
	err := c.Execute([]string{"providers", "add", "--name", "nope-not-real", "--api-key", "k"})
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("err = %v, want unknown provider", err)
	}
}

func TestProviderAddUnknownDriver(t *testing.T) {
	c, _ := newTestCLI(t)
	err := c.Execute([]string{"providers", "add", "--driver", "banana", "--base-url", "http://x"})
	if err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("err = %v, want unknown driver", err)
	}
}

func TestProviderAddMissingNameInteractive(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "testprov-ip")

	p := stubPrompts(nil)
	p.selectProvider = func() (string, error) { return "testprov-ip", nil }

	var out bytes.Buffer
	f := &providerAddFlags{apiKey: "api-key", apiKeySet: true, headersSet: true}
	if err := c.runProviderAdd(&out, f, p); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "added") {
		t.Fatalf("output = %q, want added message", out.String())
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].Provider != "testprov-ip" {
		t.Fatalf("providers = %+v", loaded.Providers)
	}
}

func TestProviderAddDriverOnlyInteractive(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestDriver(t, "testdrv-ip")

	p := stubPrompts(nil)
	p.promptString = func(label string) (string, error) {
		if label == "Base URL" {
			return "http://127.0.0.1:9/v1", nil
		}
		return "", nil
	}

	var out bytes.Buffer
	f := &providerAddFlags{driver: "testdrv-ip", apiKey: "k", apiKeySet: true, headersSet: true}
	if err := c.runProviderAdd(&out, f, p); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1 entry", loaded.Providers)
	}
	pv := loaded.Providers[0]
	if pv.Provider != "" || pv.Driver != "testdrv-ip" || pv.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("provider = %+v", pv)
	}
}

func TestProviderAddNegativeTimeout(t *testing.T) {
	c, _ := newTestCLI(t)
	registerTestProvider(t, "testprov-neg")
	err := c.Execute([]string{"providers", "add", "--name", "testprov-neg", "--api-key", "k", "--timeout", "-5"})
	if err == nil || !strings.Contains(err.Error(), "timeout must be >= 0") {
		t.Fatalf("err = %v, want timeout validation", err)
	}
}
