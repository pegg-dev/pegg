package session

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/builtin/middlewares/compaction"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

const chunksPerCommit = 10

type sessionInfo struct {
	sessionID       string
	provider        llm.Provider
	model           llm.Model
	subagent        bool
	parentSessionID string
	displayName     string
}

type Recorder struct {
	mgr      *Manager
	log      *logger.Logger
	mu       sync.Mutex
	sessions map[string]*sessionInfo
	resumed  map[string]string
	pending  map[string]*pendingMessage
}

type pendingMessage struct {
	content   string
	reasoning string
	chunks    int
	committed bool
}

func NewRecorder(mgr *Manager, log *logger.Logger) *Recorder {
	return &Recorder{
		mgr:      mgr,
		log:      log,
		sessions: make(map[string]*sessionInfo),
		resumed:  make(map[string]string),
		pending:  make(map[string]*pendingMessage),
	}
}

func (r *Recorder) Start(bus event.Bus) error {
	subs := []struct {
		topic string
		fn    any
	}{
		{agent.TopicAgentStarted, r.handleStarted},
		{agent.TopicAgentInput, r.handleInput},
		{agent.TopicAgentToken, r.handleToken},
		{agent.TopicAgentMessage, r.handleMessage},
		{agent.TopicAgentToolResult, r.handleToolResult},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
		{TopicSessionResume, r.handleResume},
		{compaction.TopicCompactionFinished, r.handleCompaction},
	}
	for _, s := range subs {
		if err := bus.Subscribe(s.topic, s.fn); err != nil {
			return fmt.Errorf("session recorder: subscribe %s: %w", s.topic, err)
		}
	}
	return nil
}

func (r *Recorder) Stop(bus event.Bus) error {
	subs := []struct {
		topic string
		fn    any
	}{
		{agent.TopicAgentStarted, r.handleStarted},
		{agent.TopicAgentInput, r.handleInput},
		{agent.TopicAgentToken, r.handleToken},
		{agent.TopicAgentMessage, r.handleMessage},
		{agent.TopicAgentToolResult, r.handleToolResult},
		{agent.TopicAgentFinished, r.handleFinished},
		{agent.TopicAgentError, r.handleError},
		{TopicSessionResume, r.handleResume},
		{compaction.TopicCompactionFinished, r.handleCompaction},
	}
	var firstErr error
	for _, s := range subs {
		if err := bus.Unsubscribe(s.topic, s.fn); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("session recorder: unsubscribe %s: %w", s.topic, err)
		}
	}
	return firstErr
}

func (r *Recorder) sessionID(agentID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.sessions[agentID]
	if !ok {
		return "", false
	}
	return info.sessionID, true
}

func (r *Recorder) getSessionInfo(agentID string) (*sessionInfo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.sessions[agentID]
	return info, ok
}

func (r *Recorder) handleResume(e SessionResume) {
	if e.AgentID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.SessionID == "" {
		delete(r.sessions, e.AgentID)
		delete(r.resumed, e.AgentID)
		return
	}
	r.resumed[e.AgentID] = e.SessionID
	if info, ok := r.sessions[e.AgentID]; ok && info.sessionID != e.SessionID {
		info.sessionID = e.SessionID
	}
}

func (r *Recorder) handleStarted(e agent.AgentStarted) {
	r.mu.Lock()
	if _, ok := r.sessions[e.AgentID]; ok {
		r.mu.Unlock()
		return
	}
	sessID := r.resumed[e.AgentID]
	r.mu.Unlock()

	subagent := e.ParentAgentID != ""
	displayName := e.DisplayName
	if displayName == "" {
		displayName = e.AgentName
	}
	var parentSessionID string
	if subagent {
		parentSessionID = r.sessionIDForAgent(e.ParentAgentID)
	}

	if sessID == "" {
		opts := CreateOptions{}
		if e.Provider != nil {
			opts.Provider = e.Provider.Name()
		}
		opts.Model = e.Model.ID
		opts.ReasoningEffort = e.ReasoningEffort
		if dir, err := os.Getwd(); err == nil {
			opts.ProjectDir = dir
		}
		if subagent {
			parentTitle := r.sessionTitle(parentSessionID)
			if isPlaceholderTitle(parentTitle) {
				parentTitle = ""
			}
			opts.Title = joinTitle(parentTitle, displayName)
		}

		s, err := r.mgr.Create(opts)
		if err != nil {
			r.log.Fdebug("session recorder: create session for agent %q: %v", e.AgentName, err)
			return
		}
		sessID = s.ID
	}

	info := &sessionInfo{
		sessionID:       sessID,
		provider:        e.Provider,
		model:           e.Model,
		subagent:        subagent,
		parentSessionID: parentSessionID,
		displayName:     displayName,
	}
	r.mu.Lock()
	r.sessions[e.AgentID] = info
	r.mu.Unlock()

	r.mgr.publish(TopicSessionAttached, SessionAttached{
		AgentID:   e.AgentID,
		SessionID: sessID,
	})
}

func (r *Recorder) sessionIDForAgent(agentID string) string {
	if agentID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if info, ok := r.sessions[agentID]; ok {
		return info.sessionID
	}
	return ""
}

func (r *Recorder) sessionTitle(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	s, err := r.mgr.Get(sessionID)
	if err != nil {
		return ""
	}
	return s.Title
}

func joinTitle(parentTitle, name string) string {
	parentTitle = strings.TrimSpace(parentTitle)
	name = strings.TrimSpace(name)
	switch {
	case parentTitle == "":
		return name
	case name == "":
		return parentTitle
	default:
		return parentTitle + " - " + name
	}
}

func (r *Recorder) handleInput(e agent.AgentInput) {
	id, ok := r.sessionID(e.AgentID)
	if !ok {
		return
	}
	first := r.isFirstMessage(id)
	if _, err := r.mgr.AppendMessage(id, llm.UserMessage(e.Input)); err != nil {
		r.log.Fdebug("session recorder: append input: %v", err)
		return
	}

	if first {
		if info, ok := r.getSessionInfo(e.AgentID); ok && info.provider != nil && !info.subagent {
			go r.generateTitle(id, info.provider, info.model, e.Input)
		}
	}
}

func (r *Recorder) isFirstMessage(sessionID string) bool {
	msgs, err := r.mgr.Messages(sessionID)
	if err != nil {
		return false
	}
	return len(msgs) == 0
}

func (r *Recorder) generateTitle(sessionID string, provider llm.Provider, model llm.Model, input string) {
	title, err := GenerateSessionTitle(provider, model, input)
	if err != nil {
		r.log.Fdebug("session recorder: generate title: %v", err)
		return
	}
	if title == "" {
		return
	}
	if err := r.mgr.SetTitle(sessionID, title); err != nil {
		r.log.Fdebug("session recorder: set title: %v", err)
		return
	}
	r.updateChildTitles(sessionID, title)
}

func (r *Recorder) updateChildTitles(parentSessionID, parentTitle string) {
	type child struct {
		sessionID string
		name      string
	}
	var children []child
	r.mu.Lock()
	for _, info := range r.sessions {
		if info.subagent && info.parentSessionID == parentSessionID {
			children = append(children, child{sessionID: info.sessionID, name: info.displayName})
		}
	}
	r.mu.Unlock()
	for _, c := range children {
		if err := r.mgr.SetTitle(c.sessionID, joinTitle(parentTitle, c.name)); err != nil {
			r.log.Fdebug("session recorder: set subagent title: %v", err)
		}
	}
}

func (r *Recorder) handleToken(e agent.AgentToken) {
	r.mu.Lock()
	info, ok := r.sessions[e.AgentID]
	p := r.pending[e.AgentID]
	if p == nil {
		p = &pendingMessage{}
		r.pending[e.AgentID] = p
	}
	p.content += e.Content
	p.reasoning += e.Reasoning
	p.chunks++
	commit := p.chunks >= chunksPerCommit
	r.mu.Unlock()

	if !ok {
		return
	}
	if commit {
		r.commitPending(e.AgentID, info.sessionID)
	}
}

func (r *Recorder) handleMessage(e agent.AgentMessage) {
	id, ok := r.sessionID(e.AgentID)
	if !ok {
		return
	}
	r.mu.Lock()
	streaming := r.pending[e.AgentID] != nil && r.pending[e.AgentID].chunks > 0
	committed := r.pending[e.AgentID] != nil && r.pending[e.AgentID].committed
	r.mu.Unlock()
	if streaming {
		r.commitPendingWithToolCalls(e.AgentID, id, e.Message.ToolCalls)
		return
	}
	if committed {
		r.mu.Lock()
		if p := r.pending[e.AgentID]; p != nil {
			p.committed = false
		}
		r.mu.Unlock()
		if len(e.Message.ToolCalls) > 0 {
			msg := llm.AssistantMessage("")
			msg.ToolCalls = e.Message.ToolCalls
			if _, err := r.mgr.AppendMessage(id, msg); err != nil {
				r.log.Fdebug("session recorder: append tool calls: %v", err)
			}
		}
		return
	}
	if _, err := r.mgr.AppendMessage(id, e.Message); err != nil {
		r.log.Fdebug("session recorder: append message: %v", err)
	}
}

func (r *Recorder) handleError(e agent.AgentError) {
	if id, ok := r.sessionID(e.AgentID); ok {
		r.commitPending(e.AgentID, id)
	}
}

func (r *Recorder) commitPending(agentID, sessID string) {
	r.commitPendingWithToolCalls(agentID, sessID, nil)
}

func (r *Recorder) commitPendingWithToolCalls(agentID, sessID string, toolCalls []llm.ToolCall) {
	r.mu.Lock()
	p, ok := r.pending[agentID]
	if !ok || p.chunks == 0 {
		r.mu.Unlock()
		return
	}
	msg := llm.AssistantMessage(p.content)
	if p.reasoning != "" {
		msg.Reasoning = p.reasoning
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	p.content = ""
	p.reasoning = ""
	p.chunks = 0
	r.mu.Unlock()

	if msg.Content == "" && msg.Reasoning == "" && len(msg.ToolCalls) == 0 {
		return
	}
	r.mu.Lock()
	if p := r.pending[agentID]; p != nil {
		p.committed = true
	}
	r.mu.Unlock()
	if _, err := r.mgr.AppendMessage(sessID, msg); err != nil {
		r.log.Fdebug("session recorder: append tokens: %v", err)
	}
}

func (r *Recorder) handleToolResult(e agent.AgentToolResult) {
	if id, ok := r.sessionID(e.AgentID); ok {
		if _, err := r.mgr.AppendMessage(id, llm.ToolMessage(e.Output, e.CallID)); err != nil {
			r.log.Fdebug("session recorder: append tool result: %v", err)
		}
	}
}

func (r *Recorder) handleFinished(e agent.AgentFinished) {
	if id, ok := r.sessionID(e.AgentID); ok {
		if err := r.mgr.AccumulateUsage(id, e.Usage); err != nil {
			r.log.Fdebug("session recorder: accumulate usage: %v", err)
		}
	}
}

func (r *Recorder) handleCompaction(e compaction.Event) {
	r.mu.Lock()
	info, ok := r.sessions[e.AgentID]
	r.mu.Unlock()
	if !ok {
		return
	}
	oldID := info.sessionID

	newSess, err := r.mgr.CompactSession(oldID, e.CompactedMessages, e.Strategy)
	if err != nil {
		r.log.Fdebug("session recorder: compact session: %v", err)
		return
	}

	r.mu.Lock()
	info.sessionID = newSess.ID
	r.mu.Unlock()

	r.log.Fdebug("session recorder: compacted session %s -> %s (%s)", oldID, newSess.ID, e.Strategy)
}
