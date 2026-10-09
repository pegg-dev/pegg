package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

func TestLookupModelConfig(t *testing.T) {
	prices := map[string]ModelConfig{
		"openai/gpt-4o":      {MaxInputTokens: 128000, Family: "gpt"},
		"gpt-4o-mini":        {Family: "gpt"},
		"groq/llama-3.3-70b": {Family: "llama"},
	}

	t.Run("provider/model exact", func(t *testing.T) {
		cfg, err := lookupModelConfig(prices, "openai", "gpt-4o")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MaxInputTokens != 128000 {
			t.Fatalf("cfg = %+v", cfg)
		}
	})

	t.Run("model exact", func(t *testing.T) {
		cfg, err := lookupModelConfig(prices, "openai", "gpt-4o-mini")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Family != "gpt" {
			t.Fatalf("cfg = %+v", cfg)
		}
	})

	t.Run("contains fallback", func(t *testing.T) {
		cfg, err := lookupModelConfig(prices, "openai", "llama-3.3-70b")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Family != "llama" {
			t.Fatalf("cfg = %+v", cfg)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := lookupModelConfig(prices, "openai", "nope-model"); !errors.Is(err, ErrModelConfigNotFound) {
			t.Fatalf("want ErrModelConfigNotFound, got %v", err)
		}
	})
}

func TestFetchPricesFrom(t *testing.T) {
	body := `{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"models": {
				"gpt-4o": {
					"id": "gpt-4o",
					"name": "GPT-4o",
					"description": "Omni GPT",
					"family": "gpt",
					"reasoning": false,
					"tool_call": true,
					"structured_output": true,
					"temperature": true,
					"open_weights": false,
					"limit": {"context": 128000, "input": 128000, "output": 16384},
					"cost": {"input": 2.5, "output": 10, "cache_read": 1.25}
				},
				"o1": {
					"id": "o1",
					"name": "o1",
					"description": "Reasoning model",
					"family": "o-series",
					"reasoning": true,
					"reasoning_options": [{"type": "effort", "values": ["low", "medium", "high"]}],
					"tool_call": true,
					"temperature": false,
					"open_weights": false,
					"limit": {"context": 200000, "output": 100000},
					"cost": {"input": 15, "output": 60, "cache_read": 7.5}
				}
			}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prices, err := fetchPricesFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 4 {
		t.Fatalf("prices = %+v, want 4 entries (2 models x 2 key formats)", prices)
	}
	cfg, ok := prices["openai/gpt-4o"]
	if !ok || cfg.MaxInputTokens != 128000 {
		t.Fatalf("gpt-4o cfg = %+v", cfg)
	}
	if cfg.InputCostPerToken != 2.5/1_000_000 {
		t.Fatalf("gpt-4o input cost = %v, want %v", cfg.InputCostPerToken, 2.5/1_000_000)
	}
	o1, ok := prices["openai/o1"]
	if !ok {
		t.Fatal("o1 missing")
	}
	if !o1.SupportsReasoning {
		t.Fatal("o1 should support reasoning")
	}
	if !reflect.DeepEqual(o1.ReasoningOptions, []ReasoningOption{{Type: "effort", Values: []string{"low", "medium", "high"}}}) {
		t.Fatalf("o1 reasoning_options = %v", o1.ReasoningOptions)
	}

	if _, err := fetchPricesFrom(context.Background(), "http://127.0.0.1:1/nope"); err == nil {
		t.Fatal("want error for unreachable url")
	}
}

func TestFetchPricesFromBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"openai": {`))
	}))
	defer srv.Close()

	if _, err := fetchPricesFrom(context.Background(), srv.URL); err == nil {
		t.Fatal("want error for malformed json")
	}
}

func TestManagerPricesCachedAndLookup(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)
	prices := map[string]ModelConfig{
		"openai/gpt-4o":                {MaxInputTokens: 128000, Family: "gpt"},
		"groq/llama-3.3-70b-versatile": {Family: "llama"},
	}
	data, err := json.Marshal(prices)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(PricesCacheKey, data); err != nil {
		t.Fatal(err)
	}

	cfg, err := mgr.ModelConfigFor("openai", "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxInputTokens != 128000 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if _, err := mgr.ModelConfigFor("openai", "nope"); !errors.Is(err, ErrModelConfigNotFound) {
		t.Fatalf("want ErrModelConfigNotFound, got %v", err)
	}
}

func TestManagerEnrichesModelsWithConfig(t *testing.T) {
	mgr, _, c := newTestManagerWithCache(t)
	prices := map[string]ModelConfig{
		"openai/gpt-4o": {MaxInputTokens: 128000, Family: "gpt"},
		"openai/o1":     {SupportsReasoning: true, Family: "o-series"},
	}
	data, _ := json.Marshal(prices)
	if err := c.Set(PricesCacheKey, data); err != nil {
		t.Fatal(err)
	}

	registerMockProvider(t, "enrich-prov", []Model{{ID: "gpt-4o"}, {ID: "o1"}, {ID: "unknown-model"}})
	mgr.Sync(context.Background(), []config.LLMConfig{{Provider: "enrich-prov"}})

	models, err := mgr.Models("enrich-prov")
	if err != nil {
		t.Fatal(err)
	}
	if models[0].Config == nil || models[0].Config.MaxInputTokens != 128000 {
		t.Fatalf("models[0] = %+v", models[0])
	}
	if models[1].Config == nil || !models[1].Config.SupportsReasoning {
		t.Fatalf("models[1] = %+v", models[1])
	}
	if models[2].Config != nil {
		t.Fatalf("models[2] = %+v, want no config", models[2])
	}

	reloaded, ok := mgr.loadCachedModels("enrich-prov")
	if !ok || len(reloaded) != 3 || reloaded[0].Config == nil {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}
