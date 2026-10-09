package codex

import (
	"context"
	"fmt"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/utils/http"
)

const DriverName = "codex"

type Service struct {
	httpClient *http.Client
	name       string
	cfg        ServiceConfig
}

type ServiceConfig struct {
	BaseURL string
	Path    string
	Headers map[string]string
	Timeout time.Duration

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
	if cfg.AuthToken != nil {
		header := cfg.AuthHeader
		if header == "" {
			header = "Authorization"
		}
		opts = append(opts, http.WithAuthHeader(header, cfg.AuthToken))
		if cfg.Refresh != nil {
			opts = append(opts, http.WithRefresh(cfg.Refresh))
		}
	}
	for k, v := range cfg.Headers {
		if v == "" {
			continue
		}
		opts = append(opts, http.WithHeader(k, v))
	}

	return &Service{
		httpClient: http.NewClient(cfg.BaseURL, opts...),
		name:       name,
		cfg:        cfg,
	}
}

func (s *Service) Name() string { return s.name }

func (s *Service) SkipModelCache() bool { return true }

func (s *Service) path() string {
	if s.cfg.Path != "" {
		return s.cfg.Path
	}
	return "/responses"
}

func (s *Service) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	acc := newAccumulator()
	if err := s.ChatStream(ctx, req, acc.append); err != nil {
		return nil, err
	}
	return acc.response(), nil
}

func (s *Service) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	body := s.buildRequest(req)
	acc := newAccumulator()

	err := s.httpClient.DoStream(ctx, s.path(), body, func(line []byte) error {
		event, data := http.ParseSSEvent(line)
		if event != "data" || len(data) == 0 {
			return nil
		}
		chunk, emit, err := s.parseEvent([]byte(data), acc)
		if err != nil {
			return err
		}
		if !emit {
			return nil
		}
		return handler(chunk)
	})
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (s *Service) ListModels(context.Context) ([]llm.Model, error) {
	return s.cfg.Models, nil
}

type codexEvent struct {
	Type     string          `json:"type"`
	Delta    json.RawMessage `json:"delta"`
	Item     json.RawMessage `json:"item"`
	Response json.RawMessage `json:"response"`
	Error    *codexError     `json:"error"`
}

type codexError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *Service) parseEvent(data []byte, acc *accumulator) (llm.StreamChunk, bool, error) {
	var ev codexEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return llm.StreamChunk{}, false, nil
	}

	switch ev.Type {
	case "response.output_text.delta":
		text := rawString(ev.Delta)
		if text == "" {
			return llm.StreamChunk{}, false, nil
		}
		acc.streamedText = true
		return llm.StreamChunk{Content: text}, true, nil

	case "response.content_part.delta":
		text := deltaText(ev.Delta)
		if text == "" {
			return llm.StreamChunk{}, false, nil
		}
		acc.streamedText = true
		return llm.StreamChunk{Content: text}, true, nil

	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		text := rawString(ev.Delta)
		if text == "" {
			return llm.StreamChunk{}, false, nil
		}
		return llm.StreamChunk{Reasoning: text}, true, nil

	case "response.output_item.done":
		return s.parseItem(ev.Item, acc)

	case "response.completed":
		acc.finish = llm.FinishReasonStop
		usage := parseUsage(ev.Response)
		chunk := llm.StreamChunk{IsDone: true, FinishReason: llm.FinishReasonStop}
		if usage != nil {
			chunk.Usage = usage
		}
		return chunk, true, nil

	case "response.failed":
		if ev.Error != nil {
			return llm.StreamChunk{}, false, fmt.Errorf("codex: response failed: %s: %s", ev.Error.Code, ev.Error.Message)
		}
		if err := parseResponseError(ev.Response); err != nil {
			return llm.StreamChunk{}, false, err
		}
		return llm.StreamChunk{}, false, fmt.Errorf("codex: response failed")

	case "error":
		if ev.Error != nil {
			return llm.StreamChunk{}, false, fmt.Errorf("codex: %s: %s", ev.Error.Code, ev.Error.Message)
		}
	}
	return llm.StreamChunk{}, false, nil
}

type codexItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
}

func (s *Service) parseItem(raw json.RawMessage, acc *accumulator) (llm.StreamChunk, bool, error) {
	var item codexItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return llm.StreamChunk{}, false, nil
	}
	switch item.Type {
	case "function_call":
		id := item.CallID
		if id == "" {
			id = item.ID
		}
		idx := acc.toolIndex(id)
		return llm.StreamChunk{ToolCalls: []llm.ToolCall{{
			Index: idx,
			ID:    id,
			Type:  "function",
			Function: llm.Function{
				Name:      item.Name,
				Arguments: item.Arguments,
			},
		}}}, true, nil
	case "message":
		if acc.streamedText {
			return llm.StreamChunk{}, false, nil
		}
		var text string
		for _, part := range item.Content {
			if part.Type == "output_text" || part.Type == "text" {
				text += part.Text
			}
		}
		if text == "" {
			return llm.StreamChunk{}, false, nil
		}
		return llm.StreamChunk{Content: text}, true, nil
	case "reasoning":
		var text string
		for _, part := range item.Summary {
			text += part.Text
		}
		if text == "" {
			return llm.StreamChunk{}, false, nil
		}
		return llm.StreamChunk{Reasoning: text}, true, nil
	}
	return llm.StreamChunk{}, false, nil
}

type accumulator struct {
	content      string
	reasoning    string
	calls        map[string]llm.ToolCall
	order        []string
	finish       llm.FinishReason
	usage        *llm.Usage
	streamedText bool
}

func newAccumulator() *accumulator {
	return &accumulator{calls: map[string]llm.ToolCall{}}
}

func (a *accumulator) toolIndex(id string) int {
	if _, ok := a.calls[id]; !ok {
		a.order = append(a.order, id)
	}
	return indexOf(a.order, id)
}

func (a *accumulator) append(chunk llm.StreamChunk) error {
	a.content += chunk.Content
	a.reasoning += chunk.Reasoning
	for _, tc := range chunk.ToolCalls {
		key := tc.ID
		if key == "" {
			key = fmt.Sprintf("#%d", tc.Index)
		}
		existing := a.calls[key]
		existing.Index = tc.Index
		if tc.ID != "" {
			existing.ID = tc.ID
		}
		if tc.Type != "" {
			existing.Type = tc.Type
		}
		if tc.Function.Name != "" {
			existing.Function.Name = tc.Function.Name
		}
		existing.Function.Arguments += tc.Function.Arguments
		a.calls[key] = existing
	}
	if chunk.FinishReason != "" {
		a.finish = chunk.FinishReason
	}
	if chunk.Usage != nil {
		a.usage = chunk.Usage
	}
	return nil
}

func (a *accumulator) response() *llm.Response {
	msg := llm.AssistantMessage(a.content)
	if a.reasoning != "" {
		msg.Reasoning = a.reasoning
	}
	for _, key := range a.order {
		msg.ToolCalls = append(msg.ToolCalls, a.calls[key])
	}
	finish := a.finish
	if finish == "" {
		finish = llm.FinishReasonStop
	}
	resp := &llm.Response{
		Object:  "chat.completion",
		Choices: []llm.Choice{{Message: &msg, FinishReason: &finish}},
	}
	if a.usage != nil {
		resp.Usage = *a.usage
	}
	return resp
}

func indexOf(list []string, value string) int {
	for i, v := range list {
		if v == value {
			return i
		}
	}
	return -1
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func deltaText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s := rawString(raw); s != "" {
		return s
	}
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Text
	}
	return ""
}

func parseUsage(raw json.RawMessage) *llm.Usage {
	if len(raw) == 0 {
		return nil
	}
	var resp struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Usage == nil {
		return nil
	}
	u := &llm.Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

func parseResponseError(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var resp struct {
		Error *codexError `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err == nil && resp.Error != nil {
		return fmt.Errorf("codex: %s: %s", resp.Error.Code, resp.Error.Message)
	}
	return nil
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
		if m, ok := parsed["detail"].(string); ok && m != "" {
			return fmt.Sprintf("API error (HTTP %d): %s", status, m)
		}
	}
	const max = 400
	if len(body) > max {
		body = body[:max] + "…"
	}
	return fmt.Sprintf("API error (HTTP %d): %s", status, body)
}
