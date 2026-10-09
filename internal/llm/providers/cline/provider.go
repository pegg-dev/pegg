package cline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/credentials"
	openaidriver "github.com/peggco/pegg/internal/llm/drivers/openai"
	"github.com/peggco/pegg/internal/llm/subscription"
)

const (
	ProviderName     = "cline"
	ProviderPassName = "cline-pass"
	DefaultBaseURL   = "https://api.cline.bot/api/v1"
)

func init() {
	llm.RegisterProvider(ProviderName, NewFromConfig)
	llm.RegisterProvider(ProviderPassName, NewPassFromConfig)
	subscription.Register(subscription.Info{
		Provider: ProviderName,
		Hint:     "run `cline auth` to sign in, then retry",
		Status:   func() credentials.Status { return credentials.NewCline(ProviderName).Status() },
	})
	subscription.Register(subscription.Info{
		Provider: ProviderPassName,
		Hint:     "run `cline auth` and subscribe to ClinePass, then retry",
		Status:   func() credentials.Status { return credentials.NewCline(ProviderPassName).Status() },
	})
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	return newProvider(ProviderName, cfg)
}

func NewPassFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	return newProvider(ProviderPassName, cfg)
}

func clineAuthHeader(token string) string {
	if !strings.HasPrefix(token, "workos:") {
		token = "workos:" + token
	}
	return "Bearer " + token
}

var clinePassModels = []llm.Model{
	{ID: "cline-pass/glm-5.3", Name: "GLM-5.3"},
	{ID: "cline-pass/glm-5.3-flash", Name: "GLM-5.3 Flash"},
	{ID: "cline-pass/kimi-k3", Name: "Kimi K3"},
	{ID: "cline-pass/deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
	{ID: "cline-pass/deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash"},
	{ID: "cline-pass/mimo-v2.5", Name: "MiMo-V2.5"},
	{ID: "cline-pass/mimo-v2.5-pro", Name: "MiMo-V2.5-Pro"},
	{ID: "cline-pass/minimax-m3", Name: "MiniMax M3"},
	{ID: "cline-pass/muse-spark-1.3-contributor", Name: "Muse Spark 1.3 Contributor"},
	{ID: "cline-pass/qwen3.8-max", Name: "Qwen3.8 Max"},
	{ID: "cline-pass/qwen3.7-max", Name: "Qwen3.7 Max"},
	{ID: "cline-pass/qwen3.7-plus", Name: "Qwen3.7 Plus"},
}

func newProvider(name string, cfg config.LLMConfig) (llm.Provider, error) {
	cred := credentials.NewCline(name)
	if !cred.Status().LoggedIn {
		return nil, fmt.Errorf("%s: not signed in — run `cline auth` to log in, then retry", name)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	headers := map[string]string{
		"HTTP-Referer": "https://cline.bot",
		"X-Title":      "Cline",
	}
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	svcCfg := openaidriver.ServiceConfig{
		BaseURL:    baseURL,
		Headers:    headers,
		Timeout:    time.Duration(cfg.Timeout) * time.Second,
		AuthHeader: "Authorization",
		AuthToken: func(ctx context.Context) (string, error) {
			token, err := cred.AccessToken(ctx)
			if err != nil {
				return "", err
			}
			return clineAuthHeader(token), nil
		},
		Refresh: cred.Refresh,
	}
	if name == ProviderPassName {
		svcCfg.Models = clinePassModels
	}

	return openaidriver.NewService(name, svcCfg), nil
}
