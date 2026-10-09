package opencodezen

import (
	"fmt"
	"maps"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	openaidriver "github.com/peggco/pegg/internal/llm/drivers/openai"
	"github.com/peggco/pegg/internal/utils/random"
)

const (
	ProviderName   = "opencode-zen"
	DefaultBaseURL = "https://opencode.ai/zen/v1"
)

func init() {
	llm.RegisterProvider(ProviderName, NewFromConfig)
}

var gateTools = []any{
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "bash",
			"description": "Execute a shell command",
			"parameters":  map[string]any{"type": "object"},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "read",
			"description": "Read a file",
			"parameters":  map[string]any{"type": "object"},
		},
	},
}

func satisfyGate(body map[string]any) {
	body["stream"] = true

	existing := map[string]bool{}
	tools, _ := body["tools"].([]any)
	for _, t := range tools {
		var tool struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		switch v := t.(type) {
		case json.RawMessage:
			_ = json.Unmarshal(v, &tool)
		case map[string]any:
			if b, err := json.Marshal(v); err == nil {
				_ = json.Unmarshal(b, &tool)
			}
		}
		if tool.Function.Name != "" {
			existing[tool.Function.Name] = true
		}
	}
	if existing["bash"] && existing["read"] {
		return
	}

	merged := append([]any{}, tools...)
	for _, gt := range gateTools {
		name := ""
		if m, ok := gt.(map[string]any); ok {
			if fn, ok := m["function"].(map[string]any); ok {
				if n, ok := fn["name"].(string); ok {
					name = n
				}
			}
		}
		if name != "" && !existing[name] {
			merged = append(merged, gt)
		}
	}
	body["tools"] = merged
	if _, ok := body["tool_choice"]; !ok {
		body["tool_choice"] = "none"
	}
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	defaultHeaders := map[string]string{
		"User-Agent":         "opencode/1.18.31 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14",
		"x-opencode-client":  "cli",
		"x-opencode-project": random.GenerateHex(20),
		"x-opencode-request": fmt.Sprintf("msg_%s", random.GenerateAlphanumeric(26)),
		"x-opencode-session": fmt.Sprintf("ses_%s%s", random.GenerateHex(12), random.GenerateAlphanumeric(14)),
	}
	maps.Copy(defaultHeaders, cfg.Headers)
	return openaidriver.NewService(ProviderName, openaidriver.ServiceConfig{
		BaseURL:       DefaultBaseURL,
		APIKey:        cfg.APIKey,
		Headers:       defaultHeaders,
		ModifyRequest: satisfyGate,
		ForceStream:   true,
		Timeout:       time.Duration(cfg.Timeout) * time.Second,
	}), nil
}
