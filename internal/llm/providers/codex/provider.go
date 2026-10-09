package codex

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/credentials"
	codexdriver "github.com/peggco/pegg/internal/llm/drivers/codex"
	"github.com/peggco/pegg/internal/llm/subscription"
)

const (
	ProviderName   = "codex"
	DefaultBaseURL = "https://chatgpt.com/backend-api/codex"
)

func init() {
	llm.RegisterProvider(ProviderName, NewFromConfig)
	subscription.Register(subscription.Info{
		Provider: ProviderName,
		Hint:     "run `codex` to sign in, then retry",
		Status:   func() credentials.Status { return credentials.NewCodex().Status() },
	})
}

var subscriptionModels = []llm.Model{
	{ID: "gpt-5.6-terra", Name: "GPT-5.6 Terra"},
	{ID: "gpt-5.6-luna", Name: "GPT-5.6 Luna"},
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	cred := credentials.NewCodex()
	if !cred.Status().LoggedIn {
		return nil, fmt.Errorf("codex: not signed in — run `codex` to log in, then retry")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	headers := map[string]string{
		"ChatGPT-Account-Id": cred.AccountID(),
		"originator":         "codex_cli_rs",
		"user-agent":         "pegg/" + config.AppVersion,
		"Accept":             "text/event-stream",
		"session_id":         uuid.NewString(),
	}
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	return codexdriver.NewService(ProviderName, codexdriver.ServiceConfig{
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
		Refresh: cred.Refresh,
		Models:  subscriptionModels,
	}), nil
}
