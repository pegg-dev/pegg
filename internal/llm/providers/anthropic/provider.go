package anthropic

import (
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	claudedriver "github.com/peggco/pegg/internal/llm/drivers/claude"
)

const (
	ProviderName   = "anthropic"
	DefaultBaseURL = "https://api.anthropic.com"
)

func init() {
	llm.RegisterProvider(ProviderName, NewFromConfig)
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	return claudedriver.NewService(ProviderName, claudedriver.ServiceConfig{
		BaseURL: DefaultBaseURL,
		APIKey:  cfg.APIKey,
		Headers: cfg.Headers,
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}), nil
}
