package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/http"
)

type Service struct {
	httpClient *http.Client
	name       string
	cfg        ServiceConfig
}

type ServiceConfig struct {
	BaseURL string
	APIKey  string
	Headers map[string]string
	Timeout time.Duration

	AuthHeader string
	AuthToken  func(ctx context.Context) (string, error)
	Refresh    func(ctx context.Context) error

	SystemPrefix string

	Models []llm.Model
}

func NewService(name string, cfg ServiceConfig) *Service {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	opts := []http.Option{http.WithTimeout(timeout)}
	switch {
	case cfg.AuthToken != nil:
		header := cfg.AuthHeader
		if header == "" {
			header = "Authorization"
		}
		opts = append(opts, http.WithAuthHeader(header, cfg.AuthToken))
		if cfg.Refresh != nil {
			opts = append(opts, http.WithRefresh(cfg.Refresh))
		}
	case cfg.APIKey != "":
		opts = append(opts, http.WithHeader("x-api-key", cfg.APIKey))
	}
	opts = append(opts, http.WithHeader("anthropic-version", "2023-06-01"))
	opts = append(opts, http.WithHeader("HTTP-Referer", config.AppUrl))
	opts = append(opts, http.WithHeader("X-Title", config.AppName))
	for k, v := range cfg.Headers {
		opts = append(opts, http.WithHeader(k, v))
	}

	return &Service{
		httpClient: http.NewClient(cfg.BaseURL, opts...),
		name:       name,
		cfg:        cfg,
	}
}

func (s *Service) Name() string { return s.name }

func (s *Service) SkipModelCache() bool { return len(s.cfg.Models) > 0 }

func (s *Service) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := s.buildBody(req, false)

	var resp claudeResponse
	if err := s.httpClient.Do(ctx, "POST", "/v1/messages", body, &resp); err != nil {
		return nil, mapError(err)
	}
	return s.toLLMResponse(resp), nil
}

func (s *Service) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	body := s.buildBody(req, true)

	return s.httpClient.DoStream(ctx, "/v1/messages", body, func(line []byte) error {
		event, data := http.ParseSSEvent(line)
		if event == "done" || len(data) == 0 {
			return nil
		}
		if event != "data" {
			return nil
		}

		var evt claudeStreamEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("failed to unmarshal stream event: %w", err)
		}
		chunk := s.toStreamChunk(evt)
		if chunk != nil {
			return handler(*chunk)
		}
		return nil
	})
}

func (s *Service) ListModels(ctx context.Context) ([]llm.Model, error) {
	if len(s.cfg.Models) > 0 {
		return s.cfg.Models, nil
	}
	var resp claudeModelsResponse
	if err := s.httpClient.Do(ctx, "GET", "/v1/models", nil, &resp); err != nil {
		return nil, mapError(err)
	}

	models := make([]llm.Model, len(resp.Data))
	for i, m := range resp.Data {
		models[i] = llm.Model{
			ID:   m.ID,
			Name: m.ID,
		}
	}
	return models, nil
}

func (s *Service) buildBody(req *llm.Request, stream bool) any {
	cr := &claudeRequest{
		Model:       req.Model,
		MaxTokens:   8192,
		Stream:      stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	if req.MaxTokens > 0 {
		cr.MaxTokens = req.MaxTokens
	}

	var systemParts []string
	var messages []claudeMessage

	if s.cfg.SystemPrefix != "" {
		systemParts = append(systemParts, s.cfg.SystemPrefix)
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemParts = append(systemParts, messageText(msg))
		default:
			messages = append(messages, s.toClaudeMessage(msg))
		}
	}

	if len(systemParts) > 0 {
		cr.System = strings.Join(systemParts, "\n")
	}

	cr.Messages = messages

	if len(req.Tools) > 0 {
		cr.Tools = make([]claudeTool, len(req.Tools))
		for i, t := range req.Tools {
			schema, _ := json.Marshal(t.Function.Parameters)
			var inputSchema map[string]any
			json.Unmarshal(schema, &inputSchema)
			cr.Tools[i] = claudeTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: inputSchema,
			}
		}
	}

	if req.User != "" {
		cr.Metadata = &claudeMetadata{UserID: req.User}
	}

	return cr
}

func (s *Service) toClaudeMessage(msg llm.Message) claudeMessage {
	cm := claudeMessage{
		Role: string(msg.Role),
	}

	switch content := msg.Content.(type) {
	case string:
		if len(msg.ToolCalls) > 0 {
			var blocks []map[string]any
			if content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": content})
			}
			for _, tc := range msg.ToolCalls {
				var input map[string]any
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			b, _ := json.Marshal(blocks)
			cm.Content = b
		} else {
			cm.Content = json.RawMessage(mustJSONString(content))
		}
	case llm.Content:
		var blocks []map[string]any
		if content.Text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": content.Text})
		}
		for _, att := range content.Attachments {
			if att.Type == llm.AttachmentTypeImage {
				var url string
				if att.URL != "" {
					url = att.URL
				} else if att.Data != "" {
					url = "data:" + att.MediaType + ";base64," + att.Data
				}
				if url != "" {
					blocks = append(blocks, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": att.MediaType,
							"data":       att.Data,
						},
					})
				}
			}
		}
		if len(blocks) == 0 {
			blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		}
		b, _ := json.Marshal(blocks)
		cm.Content = b
	default:
		b, _ := json.Marshal(content)
		cm.Content = b
	}

	if msg.Role == llm.RoleTool {
		cm.Content = json.RawMessage(mustJSONString(messageText(msg)))
	}

	return cm
}

func (s *Service) toLLMResponse(resp claudeResponse) *llm.Response {
	var textContent string
	var toolCalls []llm.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.Function{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	finishReason := llm.FinishReasonStop
	if resp.StopReason != nil {
		switch *resp.StopReason {
		case "end_turn":
			finishReason = llm.FinishReasonStop
		case "max_tokens":
			finishReason = llm.FinishReasonLength
		case "tool_use":
			finishReason = llm.FinishReasonToolCalls
		case "stop_sequence":
			finishReason = llm.FinishReasonStop
		}
	}

	msg := &llm.Message{
		Role:      llm.RoleAssistant,
		Content:   textContent,
		ToolCalls: toolCalls,
	}

	return &llm.Response{
		ID:      resp.ID,
		Object:  "chat.completion",
		Model:   resp.Model,
		Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: &finishReason}},
		Usage: llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func (s *Service) toStreamChunk(evt claudeStreamEvent) *llm.StreamChunk {
	switch evt.Type {
	case "message_start":
		return nil
	case "content_block_start":
		return nil
	case "content_block_delta":
		var delta claudeStreamDelta
		if err := json.Unmarshal(evt.Delta, &delta); err != nil {
			return nil
		}
		chunk := &llm.StreamChunk{}
		switch delta.Type {
		case "text_delta":
			chunk.Content = delta.Text
		case "input_json_delta":
			chunk.ToolCalls = []llm.ToolCall{{
				Index: evt.Index,
				Function: llm.Function{
					Arguments: delta.PartialJSON,
				},
			}}
		}
		return chunk
	case "content_block_stop":
		return nil
	case "message_delta":
		var delta claudeStreamDelta
		if err := json.Unmarshal(evt.Delta, &delta); err != nil {
			return nil
		}
		chunk := &llm.StreamChunk{IsDone: true}
		switch delta.StopReason {
		case "end_turn", "stop_sequence":
			chunk.FinishReason = llm.FinishReasonStop
		case "max_tokens":
			chunk.FinishReason = llm.FinishReasonLength
		case "tool_use":
			chunk.FinishReason = llm.FinishReasonToolCalls
		}
		if evt.Usage != nil {
			u := llm.Usage{
				PromptTokens:     evt.Usage.InputTokens,
				CompletionTokens: evt.Usage.OutputTokens,
				TotalTokens:      evt.Usage.InputTokens + evt.Usage.OutputTokens,
			}
			chunk.Usage = &u
		}
		return chunk
	case "message_stop":
		return &llm.StreamChunk{IsDone: true}
	case "ping":
		return nil
	default:
		return nil
	}
}

func messageText(msg llm.Message) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case llm.Content:
		return c.Text
	default:
		b, _ := json.Marshal(c)
		return string(b)
	}
}

func mapError(err error) error {
	httpErr, ok := err.(*http.HTTPError)
	if !ok {
		return err
	}
	msg := summariseErrorBody(httpErr.Body, httpErr.StatusCode)
	return &llm.ProviderError{
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
		if v, ok := parsed["error"].(map[string]any); ok {
			if m, ok := v["message"].(string); ok && m != "" {
				return fmt.Sprintf("API error (HTTP %d): %s", status, m)
			}
		}
	}
	const max = 400
	if len(body) > max {
		body = body[:max] + "…"
	}
	return fmt.Sprintf("API error (HTTP %d): %s", status, body)
}

func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
