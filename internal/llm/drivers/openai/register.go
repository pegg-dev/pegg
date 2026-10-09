package openai

import (
	"fmt"
	"time"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

const DriverName = "openai"

func init() {
	llm.RegisterDriver(DriverName, NewFromConfig)
}

func NewFromConfig(cfg config.LLMConfig) (llm.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai: base_url is required for %q driver", DriverName)
	}

	return NewService(DriverName, ServiceConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Headers: cfg.Headers,
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}), nil
}
