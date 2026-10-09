package openai

import (
	"context"
	"fmt"
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
	BaseURL            string
	APIKey             string
	Headers            map[string]string
	Timeout            time.Duration
	ModifyRequest      func(body map[string]any)
	ForceStream        bool
	IncludeStreamUsage *bool
	PathFor            func(endpoint, model string) string

	AuthHeader string
	AuthToken  func(ctx context.Context) (string, error)
	Refresh    func(ctx context.Context) error

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
		httpClient: http.NewClient(cfg.BaseURL, opts...),
		name:       name,
		cfg:        cfg,
	}
}

func (s *Service) Name() string { return s.name }

func (s *Service) SkipModelCache() bool { return len(s.cfg.Models) > 0 }

func (s *Service) chatPath(model string) string {
	if s.cfg.PathFor != nil {
		return s.cfg.PathFor("/chat/completions", model)
	}
	return "/chat/completions"
}

func (s *Service) modelsPath() string {
	if s.cfg.PathFor != nil {
		return s.cfg.PathFor("/models", "")
	}
	return "/models"
}

func (s *Service) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	if s.cfg.ForceStream {
		return s.chatAccumulated(ctx, req)
	}

	body, err := s.buildBody(req, false)
	if err != nil {
		return nil, err
	}

	var resp chatResponse
	if err := s.httpClient.Do(ctx, "POST", s.chatPath(req.Model), body, &resp); err != nil {
		return nil, mapError(err)
	}
	return s.toLLMResponse(resp), nil
}

func (s *Service) chatAccumulated(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	acc := newStreamAccumulator()
	if err := s.ChatStream(ctx, req, func(chunk llm.StreamChunk) error {
		acc.append(chunk)
		return nil
	}); err != nil {
		return nil, err
	}

	resp := &llm.Response{}
	msg := llm.AssistantMessage(acc.content)
	if acc.reasoning != "" {
		msg.Reasoning = acc.reasoning
	}
	if len(acc.order) > 0 {
		calls := make([]llm.ToolCall, 0, len(acc.order))
		for _, idx := range acc.order {
			c := acc.calls[idx]
			calls = append(calls, llm.ToolCall{
				ID:   c.id,
				Type: c.typ,
				Function: llm.Function{
					Name:      c.name,
					Arguments: c.arguments,
				},
			})
		}
		msg.ToolCalls = calls
	}
	resp.Choices = []llm.Choice{{Message: &msg, FinishReason: &acc.finish}}
	if acc.usage != nil {
		resp.Usage = *acc.usage
	}
	return resp, nil
}

type accToolCall struct {
	id        string
	name      string
	typ       string
	arguments string
}

type streamAccumulator struct {
	content   string
	reasoning string
	calls     map[int]*accToolCall
	order     []int
	finish    llm.FinishReason
	usage     *llm.Usage
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{calls: make(map[int]*accToolCall)}
}

func (a *streamAccumulator) append(chunk llm.StreamChunk) {
	a.content += chunk.Content
	a.reasoning += chunk.Reasoning
	for _, tc := range chunk.ToolCalls {
		acc, ok := a.calls[tc.Index]
		if !ok {
			acc = &accToolCall{id: tc.ID, typ: tc.Type}
			a.calls[tc.Index] = acc
			a.order = append(a.order, tc.Index)
		}
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Type != "" {
			acc.typ = tc.Type
		}
		if tc.Function.Name != "" {
			acc.name = tc.Function.Name
		}
		acc.arguments += tc.Function.Arguments
	}
	if chunk.FinishReason != "" {
		a.finish = chunk.FinishReason
	}
	if chunk.Usage != nil {
		a.usage = chunk.Usage
	}
}

func (s *Service) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	body, err := s.buildBody(req, true)
	if err != nil {
		return err
	}

	err = s.httpClient.DoStream(ctx, s.chatPath(req.Model), body, func(line []byte) error {
		event, data := http.ParseSSEvent(line)
		if event == "done" || len(data) == 0 {
			return nil
		}
		if event != "data" {
			return nil
		}

		var streamResp chatResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			return fmt.Errorf("failed to unmarshal stream response: %w", err)
		}
		return handler(s.toStreamChunk(streamResp))
	})

	if err != nil {
		return mapError(err)
	}
	return nil
}

func (s *Service) ListModels(ctx context.Context) ([]llm.Model, error) {
	if len(s.cfg.Models) > 0 {
		return s.cfg.Models, nil
	}
	var resp chatModelsResponse
	if err := s.httpClient.Do(ctx, "GET", s.modelsPath(), nil, &resp); err != nil {
		return nil, mapError(err)
	}

	models := make([]llm.Model, len(resp.Data))
	for i, m := range resp.Data {
		models[i] = llm.Model{
			ID:      m.ID,
			Name:    m.Name,
			Object:  m.Object,
			Created: m.Created,
			OwnedBy: m.OwnedBy,
		}
	}
	return models, nil
}

func (s *Service) buildBody(req *llm.Request, stream bool) (any, error) {
	cr := s.toChatRequest(req, stream)

	raw, err := json.Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var rawBody map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawBody); err != nil {
		return nil, fmt.Errorf("failed to normalise request: %w", err)
	}

	body := make(map[string]any, len(rawBody)+1)
	for k, v := range rawBody {
		body[k] = v
	}

	if len(req.Tools) > 0 {
		tools := make([]any, len(req.Tools))
		for i, t := range req.Tools {
			b, _ := json.Marshal(t)
			tools[i] = json.RawMessage(b)
		}
		body["tools"] = tools
	}

	if s.cfg.ModifyRequest != nil {
		s.cfg.ModifyRequest(body)
	}
	return body, nil
}

func (s *Service) toChatRequest(req *llm.Request, stream bool) *chatRequest {
	cr := &chatRequest{
		Model:             req.Model,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxTokens:         req.MaxTokens,
		Stream:            stream,
		ToolChoice:        req.ToolChoice,
		ParallelToolCalls: req.ParallelToolCalls,
		N:                 req.N,
		PresencePenalty:   req.PresencePenalty,
		FrequencyPenalty:  req.FrequencyPenalty,
		User:              req.User,
		ReasoningEffort:   req.ReasoningEffort,
	}

	if stream {
		include := true
		if s.cfg.IncludeStreamUsage != nil {
			include = *s.cfg.IncludeStreamUsage
		}
		if include {
			cr.StreamOptions = &streamOptions{IncludeUsage: true}
		}
	}

	if req.ResponseFormat != nil {
		b, _ := json.Marshal(req.ResponseFormat)
		cr.ResponseFormat = b
	}

	for _, msg := range req.Messages {
		cr.Messages = append(cr.Messages, toChatMessage(msg))
	}
	return cr
}

func (s *Service) toLLMResponse(resp chatResponse) *llm.Response {
	choices := make([]llm.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		var msg *llm.Message
		if choice.Message != nil {
			msg = &llm.Message{
				Role:      llm.Role(choice.Message.Role),
				Content:   parseContent(choice.Message.Content),
				Reasoning: parseRaw(firstReasoning(choice.Message)),
				Name:      choice.Message.Name,
			}
			if len(choice.Message.ToolCalls) > 0 {
				msg.ToolCalls = make([]llm.ToolCall, len(choice.Message.ToolCalls))
				for j, tc := range choice.Message.ToolCalls {
					msg.ToolCalls[j] = llm.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: llm.Function{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
			}
		}

		var finishReason llm.FinishReason
		if choice.FinishReason != nil {
			finishReason = llm.FinishReason(*choice.FinishReason)
		}

		choices[i] = llm.Choice{
			Index:        choice.Index,
			Message:      msg,
			FinishReason: &finishReason,
		}
	}

	return &llm.Response{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		Choices:           choices,
		Usage:             toLLMUsage(resp.Usage),
		SystemFingerprint: resp.SystemFingerprint,
	}
}

func (s *Service) toStreamChunk(resp chatResponse) llm.StreamChunk {
	chunk := llm.StreamChunk{IsDone: false}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Delta != nil {
			if choice.Delta.Content != nil {
				chunk.Content = *choice.Delta.Content
			}
			if choice.Delta.Reasoning != nil {
				chunk.Reasoning = *choice.Delta.Reasoning
			} else if choice.Delta.ReasoningContent != nil {
				chunk.Reasoning = *choice.Delta.ReasoningContent
			}
			for _, tc := range choice.Delta.ToolCalls {
				chunk.ToolCalls = append(chunk.ToolCalls, llm.ToolCall{
					Index: tc.Index,
					ID:    tc.ID,
					Type:  tc.Type,
					Function: llm.Function{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}

		if choice.FinishReason != nil && *choice.FinishReason != string(llm.FinishReasonNull) && *choice.FinishReason != "" {
			chunk.FinishReason = llm.FinishReason(*choice.FinishReason)
			chunk.IsDone = true
		}
	}

	if resp.Usage.TotalTokens > 0 {
		u := toLLMUsage(resp.Usage)
		chunk.Usage = &u
	}
	return chunk
}

func toChatMessage(msg llm.Message) chatMessage {
	cm := chatMessage{
		Role:       string(msg.Role),
		Name:       msg.Name,
		ToolCallID: msg.ToolCallID,
	}

	switch content := msg.Content.(type) {
	case string:
		cm.Content = json.RawMessage(mustJSONString(content))
	case llm.Content:
		cm.Content = contentToWire(content)
	case nil:
		cm.Content = json.RawMessage(`""`)
	default:
		b, _ := json.Marshal(content)
		cm.Content = b
	}

	if r, ok := msg.Reasoning.(string); ok {
		cm.Reasoning = json.RawMessage(mustJSONString(r))
		cm.ReasoningContent = json.RawMessage(mustJSONString(r))
	} else if msg.Reasoning != nil {
		b, _ := json.Marshal(msg.Reasoning)
		cm.Reasoning = b
		cm.ReasoningContent = b
	}

	if len(msg.ToolCalls) > 0 {
		cm.ToolCalls = make([]chatToolCall, len(msg.ToolCalls))
		for j, tc := range msg.ToolCalls {
			cm.ToolCalls[j] = chatToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: chatFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}
	return cm
}

func contentToWire(content llm.Content) json.RawMessage {
	var blocks []map[string]any

	if content.Text != "" {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": content.Text,
		})
	}

	for _, att := range content.Attachments {
		switch att.Type {
		case llm.AttachmentTypeImage:
			var url string
			if att.URL != "" {
				url = att.URL
			} else if att.Data != "" {
				url = "data:" + att.MediaType + ";base64," + att.Data
			}
			if url != "" {
				blocks = append(blocks, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": url},
				})
			}
		case llm.AttachmentTypeFile:
			var fileData string
			if att.URL != "" {
				fileData = att.URL
			} else if att.Data != "" {
				fileData = "data:" + att.MediaType + ";base64," + att.Data
			}
			if fileData != "" {
				file := map[string]any{"file_data": fileData}
				if att.FileName != "" {
					file["filename"] = att.FileName
				}
				blocks = append(blocks, map[string]any{
					"type": "input_file",
					"file": file,
				})
			}
		}
	}

	if len(blocks) == 0 {
		return json.RawMessage(`""`)
	}
	b, _ := json.Marshal(blocks)
	return b
}

func parseContent(raw json.RawMessage) any {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var any any
	if err := json.Unmarshal(raw, &any); err == nil {
		return any
	}
	return string(raw)
}

func parseRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var any any
	if err := json.Unmarshal(raw, &any); err == nil {
		return any
	}
	return string(raw)
}

func firstReasoning(m *chatMessage) json.RawMessage {
	if len(m.Reasoning) > 0 {
		return m.Reasoning
	}
	return m.ReasoningContent
}

func toLLMUsage(u chatUsage) llm.Usage {
	out := llm.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		Cost:             u.Cost,
		IsByok:           u.IsByok,
	}
	if u.PromptTokensDetails != nil {
		out.PromptTokensDetails = &llm.PromptTokensDetails{
			CachedTokens:     u.PromptTokensDetails.CachedTokens,
			CacheWriteTokens: u.PromptTokensDetails.CacheWriteTokens,
			AudioTokens:      u.PromptTokensDetails.AudioTokens,
			VideoTokens:      u.PromptTokensDetails.VideoTokens,
		}
	}
	if u.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &llm.CompletionTokensDetails{
			ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens,
			ImageTokens:     u.CompletionTokensDetails.ImageTokens,
			AudioTokens:     u.CompletionTokensDetails.AudioTokens,
		}
	}
	if u.CostDetails != nil {
		out.CostDetails = &llm.CostDetails{
			UpstreamInferenceCost:            u.CostDetails.UpstreamInferenceCost,
			UpstreamInferencePromptCost:      u.CostDetails.UpstreamInferencePromptCost,
			UpstreamInferenceCompletionsCost: u.CostDetails.UpstreamInferenceCompletionsCost,
		}
	}
	return out
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

func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
