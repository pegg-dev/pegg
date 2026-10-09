package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/credentials"
	claudedriver "github.com/peggco/pegg/internal/llm/drivers/claude"
	"github.com/peggco/pegg/internal/llm/subscription"
)

const (
	ProviderName   = "claude"
	DefaultBaseURL = "https://api.anthropic.com"

	claudeCodeIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
)

func init() {
	llm.RegisterProvider(ProviderName, NewFromConfig)
	subscription.Register(subscription.Info{
		Provider: ProviderName,
		Hint:     "run `claude` to sign in, then retry",
		Status:   func() credentials.Status { return credentials.NewClaude().Status() },
	})
}

var subscriptionModels = []llm.Model{
	{ID: "claude-opus-4-5", Name: "Claude Opus 4.5"},
	{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
	{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5"},
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	cred := credentials.NewClaude()
	if !cred.Status().LoggedIn {
		return nil, fmt.Errorf("claude: not signed in — run `claude` to log in, then retry")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	headers := map[string]string{
		"anthropic-beta": "oauth-2025-04-20,claude-code-20250219",
		"x-app":          "cli",
		"user-agent":     "pegg/" + config.AppVersion,
	}
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	return claudedriver.NewService(ProviderName, claudedriver.ServiceConfig{
		BaseURL:    baseURL,
		Headers:    headers,
		Timeout:    time.Duration(cfg.Timeout) * time.Second,
		AuthHeader: "Authorization",
		AuthToken: func(ctx context.Context) (string, error) {
			token, err := cred.AccessToken(ctx)
			if err != nil {
				return "", err
			}
			return "Bearer " + token, nil
		},
		Refresh:      cred.Refresh,
		SystemPrefix: claudeCodeIdentity,
		Models:       subscriptionModels,
	}), nil
}
