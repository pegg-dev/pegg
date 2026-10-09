package usage

import (
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

const (
	messageInterval = 20
	pctStep         = 20
)

type agentState struct {
	agent        *agent.Agent
	messageCount int
	lastPct      int
	maxTokens    int
	totalTokens  int
}

type Reminder struct {
	mu     sync.Mutex
	states map[string]*agentState
}

func New() *Reminder {
	return &Reminder{
		states: make(map[string]*agentState),
	}
}

func (r *Reminder) Start(bus event.Bus) error {
	subs := []struct {
		topic string
		fn    any
	}{
		{agent.TopicAgentStarted, r.handleStarted},
		{agent.TopicAgentMessage, r.handleMessage},
		{agent.TopicAgentUsage, r.handleUsage},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
	}
	for _, s := range subs {
		if err := bus.Subscribe(s.topic, s.fn); err != nil {
			return fmt.Errorf("usage reminder: subscribe %s: %w", s.topic, err)
		}
	}
	return nil
}

func (r *Reminder) Stop(bus event.Bus) error {
	subs := []struct {
		topic string
		fn    any
	}{
		{agent.TopicAgentStarted, r.handleStarted},
		{agent.TopicAgentMessage, r.handleMessage},
		{agent.TopicAgentUsage, r.handleUsage},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
	}
	var firstErr error
	for _, s := range subs {
		if err := bus.Unsubscribe(s.topic, s.fn); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("usage reminder: unsubscribe %s: %w", s.topic, err)
		}
	}
	return firstErr
}

func (r *Reminder) handleStarted(e agent.AgentStarted) {
	if e.Agent == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.states[e.AgentID]
	if !ok {
		s = &agentState{}
		r.states[e.AgentID] = s
	}
	s.agent = e.Agent
}

func (r *Reminder) getOrCreate(agentID string, maxTokens int) *agentState {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.states[agentID]
	if !ok {
		s = &agentState{}
		r.states[agentID] = s
	}
	if maxTokens > 0 {
		s.maxTokens = maxTokens
	}
	return s
}

func (r *Reminder) handleMessage(e agent.AgentMessage) {
	maxTokens := e.Model.MaxInputTokens()
	s := r.getOrCreate(e.AgentID, maxTokens)
	s.messageCount++
	if s.messageCount%messageInterval == 0 {
		r.queueNotification(e.AgentID, s)
	}
}

func (r *Reminder) handleUsage(e agent.AgentUsage) {
	maxTokens := e.Model.MaxInputTokens()
	s := r.getOrCreate(e.AgentID, maxTokens)
	s.totalTokens = e.Usage.TotalTokens
	if s.maxTokens <= 0 {
		return
	}
	pct := int(float64(e.Usage.TotalTokens) / float64(s.maxTokens) * 100)
	nextThreshold := (s.lastPct/pctStep + 1) * pctStep
	if pct >= nextThreshold && nextThreshold <= 100 {
		s.lastPct = nextThreshold
		r.queueUsageNotification(e.AgentID, e.Usage, s.maxTokens, nextThreshold)
	}
}

func (r *Reminder) handleFinished(e agent.AgentFinished) {
	r.cleanup(e.AgentID)
}

func (r *Reminder) handleError(e agent.AgentError) {
	r.cleanup(e.AgentID)
}

func (r *Reminder) cleanup(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.states, agentID)
}

func (r *Reminder) queueNotification(agentID string, s *agentState) {
	r.mu.Lock()
	a := s.agent
	maxTokens := s.maxTokens
	used := s.totalTokens
	r.mu.Unlock()
	if a == nil {
		return
	}
	remaining := maxTokens - used
	if remaining < 0 {
		remaining = 0
	}
	content := fmt.Sprintf("Token usage: %s/%s; %s remaining",
		llm.FormatTokens(used),
		llm.FormatTokens(maxTokens),
		llm.FormatTokens(remaining))
	a.QueueNotification(reminder.New("usage", content))
}

func (r *Reminder) queueUsageNotification(agentID string, usage llm.Usage, maxTokens, pct int) {
	r.mu.Lock()
	s := r.states[agentID]
	var a *agent.Agent
	if s != nil {
		a = s.agent
	}
	r.mu.Unlock()
	if a == nil {
		return
	}
	remaining := maxTokens - usage.TotalTokens
	if remaining < 0 {
		remaining = 0
	}
	content := fmt.Sprintf("Token usage: %s/%s; %s remaining",
		llm.FormatTokens(usage.TotalTokens),
		llm.FormatTokens(maxTokens),
		llm.FormatTokens(remaining))
	a.QueueNotification(reminder.New("usage", content))
}
