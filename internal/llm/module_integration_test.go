package llm_test

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/core/config"
	openaidriver "github.com/peggco/pegg/internal/llm/drivers/openai"
	"github.com/peggco/pegg/internal/llm/providers/groq"
	"github.com/peggco/pegg/internal/llm/providers/openai"
	"github.com/peggco/pegg/internal/llm/providers/openrouter"
)

func TestOpenAIProviderFactory(t *testing.T) {
	p, err := openai.NewFromConfig(config.LLMConfig{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "openai" {
		t.Fatalf("Name = %q, want openai", p.Name())
	}
}

func TestGroqProviderFactory(t *testing.T) {
	p, err := groq.NewFromConfig(config.LLMConfig{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "groq" {
		t.Fatalf("Name = %q, want groq", p.Name())
	}
}

func TestOpenRouterProviderFactory(t *testing.T) {
	p, err := openrouter.NewFromConfig(config.LLMConfig{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "openrouter" {
		t.Fatalf("Name = %q, want openrouter", p.Name())
	}
}

func TestOpenAIDriverFactory(t *testing.T) {
	p, err := openaidriver.NewFromConfig(config.LLMConfig{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "openai" {
		t.Fatalf("Name = %q, want openai", p.Name())
	}
}

func TestOpenAIDriverRequiresBaseURL(t *testing.T) {
	_, err := openaidriver.NewFromConfig(config.LLMConfig{APIKey: "test"})
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("unexpected error: %v", err)
	}
}
