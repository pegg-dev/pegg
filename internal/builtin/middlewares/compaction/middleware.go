package compaction

import (
	"context"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

type Middleware struct {
	middleware.BaseMiddleware
	cfg *config.CompactionConfig
	llm *llm.Manager

	mu               sync.Mutex
	needsCompact     bool
	lastPromptTokens int

	summarizer *summarizer
}

type Deps struct {
	Config *config.CompactionConfig
	LLM    *llm.Manager
}

func New(deps Deps) *Middleware {
	return &Middleware{
		cfg:        deps.Config,
		llm:        deps.LLM,
		summarizer: newSummarizer(),
	}
}

func (m *Middleware) hasStrategy(name string) bool {
	if m.cfg == nil {
		return false
	}
	for _, s := range m.cfg.Strategy {
		if s == name {
			return true
		}
	}
	return false
}

func (m *Middleware) threshold() float64 {
	if m.cfg == nil || m.cfg.Threshold <= 0 {
		return 80
	}
	return m.cfg.Threshold
}

func (m *Middleware) publish(ctx context.Context, topic string, evt Event) {
	a := agent.FromContext(ctx)
	if a == nil || a.Bus == nil {
		return
	}
	evt.AgentID = a.ID
	evt.AgentName = a.Name
	a.Bus.Publish(topic, evt)
	if handler := agent.StreamFrom(ctx); handler != nil {
		_ = handler(agent.StreamEvent{
			Type:      agent.StreamCompaction,
			AgentID:   a.ID,
			AgentName: a.Name,
			Strategy:  evt.Strategy,
			Messages:  evt.Messages,
			Tokens:    evt.Tokens,
		})
	}
}

func (m *Middleware) AfterLLM(ctx context.Context, req *llm.Request, resp *llm.Response) error {
	if m.cfg == nil || !m.cfg.Enabled {
		return nil
	}

	a := agent.FromContext(ctx)
	if a == nil || a.Model.Config == nil {
		return nil
	}

	maxInput := a.Model.MaxInputTokens()
	if maxInput <= 0 {
		return nil
	}

	pct := float64(resp.Usage.PromptTokens) / float64(maxInput) * 100
	if pct >= m.threshold() {
		m.mu.Lock()
		m.needsCompact = true
		m.lastPromptTokens = resp.Usage.PromptTokens
		m.mu.Unlock()
	}
	return nil
}

func (m *Middleware) BeforeLLM(ctx context.Context, req *llm.Request) error {
	if m.cfg == nil || !m.cfg.Enabled {
		return nil
	}

	m.mu.Lock()
	needs := m.needsCompact
	promptTokens := m.lastPromptTokens
	m.needsCompact = false
	m.lastPromptTokens = 0
	m.mu.Unlock()

	if !needs {
		if m.hasStrategy("tool-clearing") {
			m.truncateToolMessages(req)
		}
		m.checkResumeOverflow(ctx, req)
		return nil
	}

	ref := agent.HistoryRefFrom(ctx)
	if ref == nil || len(*ref) == 0 {
		return nil
	}

	beforeCount := len(req.Messages)

	if m.hasStrategy("summarization") {
		if err := m.compactSummarization(ctx, req, ref); err == nil && len(req.Messages) < beforeCount {
			compacted := make([]llm.Message, len(req.Messages))
			copy(compacted, req.Messages)
			m.publish(ctx, TopicCompactionFinished, Event{
				Strategy:          "summarization",
				Messages:          len(req.Messages),
				Tokens:            promptTokens,
				CompactedMessages: compacted,
			})
			return nil
		}
	}

	if m.hasStrategy("sliding-window") {
		preWindow := len(req.Messages)
		m.compactSlidingWindow(ctx, req, ref, promptTokens)
		removed := preWindow - len(req.Messages)
		if removed > 0 {
			compacted := make([]llm.Message, len(req.Messages))
			copy(compacted, req.Messages)
			m.publish(ctx, TopicCompactionFinished, Event{
				Strategy:          "sliding-window",
				Messages:          len(req.Messages),
				Tokens:            promptTokens,
				CompactedMessages: compacted,
			})
		}
		return nil
	}

	return nil
}

func (m *Middleware) compactSummarization(ctx context.Context, req *llm.Request, ref *[]llm.Message) error {
	history := *ref
	if len(history) <= 2 {
		return nil
	}

	a := agent.FromContext(ctx)
	s := m.summarizer.get(m.cfg, m.llm, a)
	if s == nil {
		return nil
	}

	summary, err := m.summarizer.run(history)
	if err != nil {
		return err
	}

	systemMsg := history[0]
	var lastUserMsg llm.Message
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			lastUserMsg = history[i]
			break
		}
	}

	newHistory := make([]llm.Message, 0, 3)
	newHistory = append(newHistory, systemMsg)
	newHistory = append(newHistory, llm.SystemMessage("Previous conversation summary:\n"+summary))
	if lastUserMsg.Content != nil {
		newHistory = append(newHistory, lastUserMsg)
	}

	*ref = newHistory
	req.Messages = newHistory
	return nil
}

func (m *Middleware) compactSlidingWindow(ctx context.Context, req *llm.Request, ref *[]llm.Message, promptTokens int) {
	history := *ref
	if len(history) == 0 {
		return
	}

	a := agent.FromContext(ctx)
	maxInput := 0
	if a != nil && a.Model.Config != nil {
		maxInput = a.Model.MaxInputTokens()
	}

	keepCount := 0
	if maxInput > 0 && promptTokens > 0 {
		avgTokensPerMsg := promptTokens / len(history)
		if avgTokensPerMsg > 0 {
			targetTokens := int(float64(maxInput) * m.threshold() / 100)
			keepCount = targetTokens/avgTokensPerMsg - 2
		}
	}

	if keepCount <= 0 {
		keepCount = m.cfg.MaxMessages
	}
	if keepCount <= 0 {
		keepCount = 50
	}

	if len(history) <= keepCount+1 {
		m.truncateToolMessages(req)
		return
	}

	keepStart := len(history) - keepCount
	compacted := make([]llm.Message, 0, keepCount+2)
	compacted = append(compacted, history[0])
	compacted = append(compacted, llm.SystemMessage("[Context compacted: older messages removed]"))
	compacted = append(compacted, history[keepStart:]...)

	*ref = compacted
	req.Messages = compacted
}

func (m *Middleware) checkResumeOverflow(ctx context.Context, req *llm.Request) {
	a := agent.FromContext(ctx)
	if a == nil || a.Model.Config == nil {
		return
	}
	maxInput := a.Model.MaxInputTokens()
	if maxInput <= 0 {
		return
	}
	est := estimateTokens(req.Messages)
	if est <= 0 {
		return
	}
	pct := float64(est) / float64(maxInput) * 100
	if pct >= m.threshold() {
		m.mu.Lock()
		m.needsCompact = true
		m.lastPromptTokens = est
		m.mu.Unlock()
	}
}

func estimateTokens(msgs []llm.Message) int {
	total := 0
	for _, msg := range msgs {
		total += len(llm.MessageText(msg))
	}
	return total / 4
}

func (m *Middleware) truncateToolMessages(req *llm.Request) {
	maxChars := m.cfg.MaxToolOutputChars
	if maxChars <= 0 {
		maxChars = 4000
	}

	for i, msg := range req.Messages {
		if msg.Role != llm.RoleTool {
			continue
		}
		text := llm.MessageText(msg)
		if len(text) <= maxChars {
			continue
		}
		truncated := text[:maxChars] + "\n... [truncated]"
		req.Messages[i].Content = truncated
	}
}
