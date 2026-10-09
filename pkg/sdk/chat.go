package sdk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type ChatRequest struct {
	Input           string
	SessionID       string
	History         []Message
	Provider        string
	Model           string
	Files           []string
	Attachments     []Attachment
	Tools           []string
	SystemPrompt    string
	MaxIterations   int
	ReasoningEffort string
	Timeout         time.Duration
}

type ChatResponse struct {
	Output       string
	Usage        Usage
	Iterations   int
	FinishReason FinishReason
	AgentID      string
	AgentName    string
	SessionID    string
	History      []Message
}

type ChatEvent struct {
	Type       ChatEventType
	AgentID    string
	AgentName  string
	Model      Model
	Content    string
	Reasoning  string
	ToolCall   *ToolCall
	ToolOutput string
	ToolErr    error
	Usage      *Usage
	Strategy   string
	Messages   int
	Tokens     int
}

type ChatEventType string

const (
	EventToken      ChatEventType = "token"
	EventReasoning  ChatEventType = "reasoning"
	EventToolCall   ChatEventType = "tool_call"
	EventToolResult ChatEventType = "tool_result"
	EventCompaction ChatEventType = "compaction"
	EventDone       ChatEventType = "done"
)

func (e *Engine) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return e.chat(ctx, req, nil)
}

func (e *Engine) ChatStream(ctx context.Context, req ChatRequest, handler func(ChatEvent) error) (ChatResponse, error) {
	if handler == nil {
		return e.chat(ctx, req, nil)
	}
	return e.chat(ctx, req, handler)
}

func (e *Engine) chat(ctx context.Context, req ChatRequest, handler func(ChatEvent) error) (ChatResponse, error) {
	if err := e.checkOpen(); err != nil {
		return ChatResponse{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	orch, err := agents.New("orchestrator")
	if err != nil {
		return ChatResponse{}, fmt.Errorf("sdk: create orchestrator: %w", err)
	}
	orch.Bus = e.bus

	prov, mdl, err := e.resolveModel(ctx, req.Provider, req.Model)
	if err != nil {
		return ChatResponse{}, err
	}
	orch.SetModelProvider(mdl, prov)

	if req.SystemPrompt != "" {
		orch.SystemPrompt = req.SystemPrompt
	}
	if req.MaxIterations > 0 {
		orch.MaxIterations = req.MaxIterations
	}
	if req.ReasoningEffort != "" {
		orch.ReasoningEffort = req.ReasoningEffort
	}
	if len(req.Tools) > 0 {
		orch.ToolNames = req.Tools
	}
	if len(req.Attachments) > 0 {
		orch.Attachments = append(orch.Attachments, req.Attachments...)
	}
	if len(req.Files) > 0 {
		atts, err := loadFileAttachments(req.Files)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("sdk: load attachments: %w", err)
		}
		orch.Attachments = append(orch.Attachments, atts...)
	}

	history := append([]Message(nil), req.History...)
	if req.SessionID != "" {
		msgs, err := e.sessions.Messages(req.SessionID)
		if err != nil {
			return ChatResponse{}, ErrNoSession
		}
		history = append(history, session.MessagesToLLM(msgs)...)
		e.bus.Publish(session.TopicSessionResume, session.SessionResume{
			AgentID:   orch.ID,
			SessionID: req.SessionID,
		})
	}

	sessCh := make(chan string, 1)
	attachHandler := func(ev session.SessionAttached) {
		if ev.AgentID == orch.ID {
			select {
			case sessCh <- ev.SessionID:
			default:
			}
		}
	}
	if err := e.bus.Subscribe(session.TopicSessionAttached, attachHandler); err == nil {
		defer e.bus.Unsubscribe(session.TopicSessionAttached, attachHandler)
	}

	stream := func(ev agent.StreamEvent) error {
		if handler == nil {
			return nil
		}
		return handler(mapStreamEvent(ev))
	}

	var result *agent.RunResult
	if len(history) > 0 {
		result, err = orch.ResumeStream(ctx, req.Input, history, stream)
	} else {
		result, err = orch.RunStream(ctx, req.Input, stream)
	}
	if err != nil {
		return ChatResponse{}, fmt.Errorf("sdk: run: %w", err)
	}

	sessID := ""
	select {
	case sessID = <-sessCh:
	default:
	}

	resp := ChatResponse{
		Output:       result.Output,
		Usage:        result.Usage,
		Iterations:   result.Iterations,
		FinishReason: result.FinishReason,
		AgentID:      orch.ID,
		AgentName:    orch.Name,
		SessionID:    sessID,
		History:      result.History,
	}
	return resp, nil
}

func mapStreamEvent(ev agent.StreamEvent) ChatEvent {
	out := ChatEvent{
		Type:       ChatEventType(ev.Type),
		AgentID:    ev.AgentID,
		AgentName:  ev.AgentName,
		Model:      ev.Model,
		Content:    ev.Content,
		Reasoning:  ev.Reasoning,
		ToolOutput: ev.ToolOutput,
		ToolErr:    ev.ToolErr,
		Usage:      ev.Usage,
		Strategy:   ev.Strategy,
		Messages:   ev.Messages,
		Tokens:     ev.Tokens,
	}
	if ev.ToolCall != nil {
		tc := *ev.ToolCall
		out.ToolCall = &tc
	}
	return out
}

func (e *Engine) resolveModel(ctx context.Context, provider, model string) (llm.Provider, llm.Model, error) {
	if provider != "" && model != "" {
		models, err := e.providerModels(ctx, provider)
		if err != nil {
			return nil, llm.Model{}, err
		}
		p, err := e.llm.Provider(provider)
		if err != nil {
			return nil, llm.Model{}, fmt.Errorf("sdk: %w", err)
		}
		for _, m := range models {
			if m.ID == model || m.Name == model {
				return p, m, nil
			}
		}
		return nil, llm.Model{}, fmt.Errorf("%w: model %q not found in provider %q", ErrNoModels, model, provider)
	}

	reply := "sdk.select.reply." + uuid.NewString()
	resultCh := make(chan llm.SelectResult, 1)
	handler := func(res llm.SelectResult) {
		select {
		case resultCh <- res:
		default:
		}
	}
	if err := e.bus.SubscribeOnce(reply, handler); err != nil {
		return nil, llm.Model{}, err
	}
	defer e.bus.Unsubscribe(reply, handler)

	mode := llm.SelectModePreferred
	if provider != "" {
		mode = llm.SelectModeExact
	}
	e.bus.Publish(event.TopicModelSelect, llm.SelectRequest{
		Mode:       mode,
		Provider:   provider,
		Model:      model,
		ReplyTopic: reply,
	})

	select {
	case res := <-resultCh:
		if res.Err != nil {
			if provider == "" && e.noEntries() {
				return nil, llm.Model{}, ErrNotConfigured
			}
			return nil, llm.Model{}, fmt.Errorf("sdk: select model: %w", res.Err)
		}
		p, err := e.llm.Provider(res.Provider)
		if err != nil {
			return nil, llm.Model{}, fmt.Errorf("sdk: %w", err)
		}
		return p, res.Model, nil
	case <-ctx.Done():
		return nil, llm.Model{}, fmt.Errorf("%w: %v", ErrModelTimeout, ctx.Err())
	}
}

func (e *Engine) noEntries() bool {
	return len(e.cfg.Providers) == 0
}

func (e *Engine) providerModels(ctx context.Context, name string) ([]Model, error) {
	type result struct {
		models []Model
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		m, err := e.llm.Models(name)
		ch <- result{m, err}
	}()
	select {
	case r := <-ch:
		return r.models, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrModelTimeout, ctx.Err())
	}
}

func loadFileAttachments(paths []string) ([]llm.Attachment, error) {
	var out []llm.Attachment
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", p, err)
		}
		mediaType := mediaTypeFor(p)
		attType := llm.AttachmentTypeFile
		if isImageMediaType(mediaType) {
			attType = llm.AttachmentTypeImage
		}
		att := llm.NewAttachmentFromBase64(attType, mediaType, llm.EncodeFileToBase64(data))
		att.FileName = filepath.Base(p)
		out = append(out, att)
	}
	return out, nil
}

func mediaTypeFor(path string) string {
	switch filepath.Ext(path) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return "image/" + filepath.Ext(path)[1:]
	default:
		return "application/octet-stream"
	}
}

func isImageMediaType(mt string) bool {
	return len(mt) >= 6 && mt[:6] == "image/"
}
