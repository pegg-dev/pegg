package gemini

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
	apiKey     string
}

type ServiceConfig struct {
	BaseURL string
	APIKey  string
	Headers map[string]string
	Timeout time.Duration
}

func NewService(name string, cfg ServiceConfig) *Service {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	opts := []http.Option{http.WithTimeout(timeout)}
	opts = append(opts, http.WithHeader("HTTP-Referer", config.AppUrl))
	opts = append(opts, http.WithHeader("X-Title", config.AppName))
	for k, v := range cfg.Headers {
		opts = append(opts, http.WithHeader(k, v))
	}

	return &Service{
		httpClient: http.NewClient(cfg.BaseURL, opts...),
		name:       name,
		cfg:        cfg,
		apiKey:     cfg.APIKey,
	}
}

func (s *Service) Name() string { return s.name }

func (s *Service) modelPath(model, method string) string {
	if s.apiKey != "" {
		return fmt.Sprintf("/models/%s:%s?key=%s", model, method, s.apiKey)
	}
	return fmt.Sprintf("/models/%s:%s", model, method)
}

func (s *Service) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	body := s.buildBody(req, false)

	var resp geminiResponse
	path := s.modelPath(req.Model, "generateContent")
	if err := s.httpClient.Do(ctx, "POST", path, body, &resp); err != nil {
		return nil, mapError(err)
	}
	return s.toLLMResponse(req.Model, resp), nil
}

func (s *Service) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	body := s.buildBody(req, true)

	path := s.modelPath(req.Model, "streamGenerateContent?alt=sse")
	return s.httpClient.DoStream(ctx, path, body, func(line []byte) error {
		event, data := http.ParseSSEvent(line)
		if event == "done" || len(data) == 0 {
			return nil
		}
		if event != "data" {
			return nil
		}

		var resp geminiResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			return fmt.Errorf("failed to unmarshal stream response: %w", err)
		}
		chunk := s.toStreamChunk(resp)
		if chunk != nil {
			return handler(*chunk)
		}
		return nil
	})
}

func (s *Service) ListModels(ctx context.Context) ([]llm.Model, error) {
	var resp geminiModelsResponse
	path := "/models"
	if s.apiKey != "" {
		path = fmt.Sprintf("/models?key=%s", s.apiKey)
	}
	if err := s.httpClient.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, mapError(err)
	}

	models := make([]llm.Model, len(resp.GeminiModels))
	for i, m := range resp.GeminiModels {
		name := strings.TrimPrefix(m.Name, "models/")
		models[i] = llm.Model{
			ID:   name,
			Name: m.DisplayName,
		}
	}
	return models, nil
}

func (s *Service) buildBody(req *llm.Request, stream bool) any {
	gr := &geminiRequest{}

	var systemParts []string
	var contents []geminiContent

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemParts = append(systemParts, messageText(msg))
		case llm.RoleUser:
			contents = append(contents, s.toGeminiContent("user", msg))
		case llm.RoleAssistant:
			contents = append(contents, s.toGeminiContent("model", msg))
		case llm.RoleTool:
			contents = append(contents, s.toToolResponse(msg))
		}
	}

	if len(systemParts) > 0 {
		gr.SystemInstruction = &geminiContent{
			Role:  "user",
			Parts: []geminiPart{{Text: strings.Join(systemParts, "\n")}},
		}
	}

	gr.Contents = contents

	genConfig := &geminiGenerationConfig{}
	if req.Temperature > 0 {
		genConfig.Temperature = req.Temperature
	}
	if req.TopP > 0 {
		genConfig.TopP = req.TopP
	}
	if req.MaxTokens > 0 {
		genConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case llm.ResponseFormatJSONObject:
			genConfig.ResponseMimeType = "application/json"
		case llm.ResponseFormatJSONSchema:
			genConfig.ResponseMimeType = "application/json"
			if req.ResponseFormat.JSONSchema != nil {
				genConfig.ResponseSchema = req.ResponseFormat.JSONSchema.Schema
			}
		}
	}
	if genConfig.Temperature > 0 || genConfig.TopP > 0 || genConfig.MaxOutputTokens > 0 || genConfig.ResponseMimeType != "" {
		gr.GenerationConfig = genConfig
	}

	if len(req.Tools) > 0 {
		funcDecls := make([]geminiFuncDecl, len(req.Tools))
		for i, t := range req.Tools {
			schema, _ := json.Marshal(t.Function.Parameters)
			var params map[string]any
			json.Unmarshal(schema, &params)
			funcDecls[i] = geminiFuncDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  params,
			}
		}
		gr.Tools = []geminiTool{{FunctionDeclarations: funcDecls}}
	}

	return gr
}

func (s *Service) toGeminiContent(role string, msg llm.Message) geminiContent {
	gc := geminiContent{Role: role}

	switch content := msg.Content.(type) {
	case string:
		parts := []geminiPart{{Text: content}}
		for _, tc := range msg.ToolCalls {
			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFuncCall{
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		gc.Parts = parts
	case llm.Content:
		var parts []geminiPart
		if content.Text != "" {
			parts = append(parts, geminiPart{Text: content.Text})
		}
		for _, att := range content.Attachments {
			if att.Type == llm.AttachmentTypeImage && att.Data != "" {
				parts = append(parts, geminiPart{
					InlineData: &geminiBlob{
						MimeType: att.MediaType,
						Data:     att.Data,
					},
				})
			}
		}
		if len(parts) == 0 {
			parts = []geminiPart{{Text: ""}}
		}
		gc.Parts = parts
	default:
		b, _ := json.Marshal(content)
		gc.Parts = []geminiPart{{Text: string(b)}}
	}

	return gc
}

func (s *Service) toToolResponse(msg llm.Message) geminiContent {
	var name string
	var response map[string]any

	if len(msg.ToolCalls) > 0 {
		name = msg.ToolCalls[0].Function.Name
	}

	content := messageText(msg)
	if content != "" {
		json.Unmarshal([]byte(content), &response)
	}
	if response == nil {
		response = map[string]any{"result": content}
	}

	return geminiContent{
		Role: "user",
		Parts: []geminiPart{{
			FunctionResponse: &geminiFuncResp{
				Name:     name,
				Response: response,
			},
		}},
	}
}

func (s *Service) toLLMResponse(model string, resp geminiResponse) *llm.Response {
	if len(resp.Candidates) == 0 {
		return &llm.Response{
			ID:      fmt.Sprintf("gemini-%d", time.Now().UnixMilli()),
			Object:  "chat.completion",
			Model:   model,
			Choices: []llm.Choice{{Index: 0, Message: &llm.Message{Role: llm.RoleAssistant, Content: ""}}},
		}
	}

	candidate := resp.Candidates[0]
	var textContent string
	var toolCalls []llm.ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textContent += part.Text
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   fmt.Sprintf("call_%d", len(toolCalls)),
				Type: "function",
				Function: llm.Function{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}

	finishReason := llm.FinishReasonStop
	switch candidate.FinishReason {
	case "MAX_TOKENS":
		finishReason = llm.FinishReasonLength
	case "SAFETY":
		finishReason = llm.FinishReasonContentFilter
	case "RECITATION":
		finishReason = llm.FinishReasonContentFilter
	case "OTHER":
		finishReason = llm.FinishReasonStop
	case "FINISH_REASON_UNSPECIFIED", "":
		finishReason = llm.FinishReasonStop
	}

	msg := &llm.Message{
		Role:      llm.RoleAssistant,
		Content:   textContent,
		ToolCalls: toolCalls,
	}

	var usage llm.Usage
	if resp.UsageMetadata != nil {
		usage = llm.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return &llm.Response{
		ID:      fmt.Sprintf("gemini-%d", time.Now().UnixMilli()),
		Object:  "chat.completion",
		Model:   model,
		Choices: []llm.Choice{{Index: 0, Message: msg, FinishReason: &finishReason}},
		Usage:   usage,
	}
}

func (s *Service) toStreamChunk(resp geminiResponse) *llm.StreamChunk {
	if len(resp.Candidates) == 0 {
		return nil
	}

	candidate := resp.Candidates[0]
	chunk := &llm.StreamChunk{}

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			chunk.Content = part.Text
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			chunk.ToolCalls = append(chunk.ToolCalls, llm.ToolCall{
				ID:   fmt.Sprintf("call_%d", len(chunk.ToolCalls)),
				Type: "function",
				Function: llm.Function{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}

	switch candidate.FinishReason {
	case "STOP", "FINISH_REASON_UNSPECIFIED":
		if candidate.FinishReason == "STOP" {
			chunk.FinishReason = llm.FinishReasonStop
			chunk.IsDone = true
		}
	case "MAX_TOKENS":
		chunk.FinishReason = llm.FinishReasonLength
		chunk.IsDone = true
	case "SAFETY", "RECITATION":
		chunk.FinishReason = llm.FinishReasonContentFilter
		chunk.IsDone = true
	}

	if resp.UsageMetadata != nil {
		u := llm.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
		chunk.Usage = &u
	}

	return chunk
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
