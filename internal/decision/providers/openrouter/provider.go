package openrouter

import (
	"context"
	"fmt"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/utils/http"
)

const (
	ProviderName   = "openrouter"
	DefaultBaseURL = "https://openrouter.ai"
	DecisionsPath  = "/api/alpha/decisions"
	DefaultModel   = "typesafe/jev-1.13"
)

func init() {
	decision.RegisterProvider(ProviderName, NewFromConfig)
	decision.RegisterDefaultModel(ProviderName, DefaultModel)
}

type Service struct {
	httpClient *http.Client
	name       string
}

func NewFromConfig(cfg config.LLMConfig) (decision.Provider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	opts := []http.Option{http.WithTimeout(time.Duration(cfg.Timeout) * time.Second)}
	if cfg.APIKey != "" {
		opts = append(opts, http.WithAPIKey(cfg.APIKey))
	}
	defaultHeaders := map[string]string{
		"HTTP-Referer": config.AppUrl,
		"X-Title":      config.AppName,
	}
	for k, v := range defaultHeaders {
		if _, ok := cfg.Headers[k]; ok {
			continue
		}
		opts = append(opts, http.WithHeader(k, v))
	}
	for k, v := range cfg.Headers {
		opts = append(opts, http.WithHeader(k, v))
	}

	return &Service{
		httpClient: http.NewClient(baseURL, opts...),
		name:       ProviderName,
	}, nil
}

func (s *Service) Name() string { return s.name }

func (s *Service) Decide(ctx context.Context, req *decision.Request) (*decision.Response, error) {
	if req.Model == "" {
		req.Model = DefaultModel
	}
	var resp decision.Response
	if err := s.httpClient.Do(ctx, "POST", DecisionsPath, req, &resp); err != nil {
		return nil, mapError(err)
	}
	return &resp, nil
}

func mapError(err error) error {
	httpErr, ok := err.(*http.HTTPError)
	if !ok {
		return err
	}
	msg := summariseErrorBody(httpErr.Body, httpErr.StatusCode)
	return &decision.ProviderError{
		StatusCode: httpErr.StatusCode,
		Message:    msg,
		Body:       httpErr.Body,
	}
}

func summariseErrorBody(body string, status int) string {
	if body == "" {
		return fmt.Sprintf("API error (HTTP %d)", status)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err == nil {
		if msg, ok := extractErrorMessage(parsed); ok && msg != "" {
			return fmt.Sprintf("API error (HTTP %d): %s", status, msg)
		}
	}
	const max = 400
	if len(body) > max {
		body = body[:max] + "…"
	}
	return fmt.Sprintf("API error (HTTP %d): %s", status, body)
}

func extractErrorMessage(parsed map[string]any) (string, bool) {
	if v, ok := parsed["error"].(map[string]any); ok {
		if m, ok := v["message"].(string); ok {
			return m, true
		}
	}
	if m, ok := parsed["message"].(string); ok {
		return m, true
	}
	return "", false
}
