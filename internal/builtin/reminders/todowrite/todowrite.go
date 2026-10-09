package todowrite

import (
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/core/event"
)

const (
	todoWriteTool = "todowrite"
	eventInterval = 30
)

const reminderContent = `The TodoWrite tool hasn't been used recently. If you're working on tasks that would benefit from tracking progress, consider using the TodoWrite tool to track progress. Also consider cleaning up the todo list if it has become stale and no longer matches what you are working on. Only use it if it's relevant to the current work. This is just a gentle reminder - ignore if not applicable.`

type agentState struct {
	agent *agent.Agent
	count int
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
		{agent.TopicAgentToolCall, r.handleToolCall},
		{agent.TopicAgentToolResult, r.handleToolResult},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
	}
	for _, s := range subs {
		if err := bus.Subscribe(s.topic, s.fn); err != nil {
			return fmt.Errorf("todowrite reminder: subscribe %s: %w", s.topic, err)
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
		{agent.TopicAgentToolCall, r.handleToolCall},
		{agent.TopicAgentToolResult, r.handleToolResult},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
	}
	var firstErr error
	for _, s := range subs {
		if err := bus.Unsubscribe(s.topic, s.fn); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("todowrite reminder: unsubscribe %s: %w", s.topic, err)
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

func (r *Reminder) handleToolCall(e agent.AgentToolCall) {
	if e.Call.Function.Name == todoWriteTool {
		r.reset(e.AgentID)
		return
	}
	r.bump(e.AgentID)
}

func (r *Reminder) handleToolResult(e agent.AgentToolResult) {
	if e.ToolName == todoWriteTool {
		r.reset(e.AgentID)
		return
	}
	r.bump(e.AgentID)
}

func (r *Reminder) bump(agentID string) {
	r.mu.Lock()
	s, ok := r.states[agentID]
	if !ok || s.agent == nil {
		r.mu.Unlock()
		return
	}
	s.count++
	fire := s.count >= eventInterval
	if fire {
		s.count = 0
	}
	a := s.agent
	r.mu.Unlock()

	if fire {
		a.QueueNotification(reminder.New("todowrite", reminderContent))
	}
}

func (r *Reminder) reset(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.states[agentID]; ok {
		s.count = 0
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
