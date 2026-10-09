package sdk

import (
	"context"
	"time"

	"github.com/peggco/pegg/internal/llm"
)

type CompleteRequest struct {
	Provider        string
	Model           string
	Messages        []Message
	Temperature     float64
	TopP            float64
	MaxTokens       int
	Tools           []LLMTool
	ToolChoice      any
	ReasoningEffort string
	User            string
	ResponseFormat  *ResponseFormat
	Timeout         time.Duration
}

func (e *Engine) Complete(ctx context.Context, req CompleteRequest) (*Response, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	p, mdl, err := e.resolveModel(ctx, req.Provider, req.Model)
	if err != nil {
		return nil, err
	}

	r := llm.NewRequest(mdl.ID, req.Messages)
	r.Temperature = req.Temperature
	r.TopP = req.TopP
	r.MaxTokens = req.MaxTokens
	r.Tools = req.Tools
	r.ToolChoice = req.ToolChoice
	r.ReasoningEffort = req.ReasoningEffort
	r.User = req.User
	if req.ResponseFormat != nil {
		r.ResponseFormat = req.ResponseFormat
	}

	return p.Chat(ctx, r)
}

func (e *Engine) CompleteStream(ctx context.Context, req CompleteRequest, handler func(StreamChunk) error) error {
	if err := e.checkOpen(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	p, mdl, err := e.resolveModel(ctx, req.Provider, req.Model)
	if err != nil {
		return err
	}

	r := llm.NewRequest(mdl.ID, req.Messages)
	r.Temperature = req.Temperature
	r.TopP = req.TopP
	r.MaxTokens = req.MaxTokens
	r.Tools = req.Tools
	r.ToolChoice = req.ToolChoice
	r.ReasoningEffort = req.ReasoningEffort
	r.User = req.User
	if req.ResponseFormat != nil {
		r.ResponseFormat = req.ResponseFormat
	}
	r.Stream = true

	return p.ChatStream(ctx, r, llm.StreamHandler(handler))
}

func (e *Engine) Models(ctx context.Context, provider string) ([]Model, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.llm.Models(provider)
}

func (e *Engine) Provider(ctx context.Context, name string) (Provider, error) {
	if err := e.checkOpen(); err != nil {
		return nil, err
	}
	return e.llm.Provider(name)
}

func (e *Engine) Providers() []string {
	return llm.ListProviders()
}
