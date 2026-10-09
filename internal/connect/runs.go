package connect

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

const (
	runStateRunning  = "running"
	runStateDone     = "done"
	runStateError    = "error"
	runStateCanceled = "canceled"
)

type run struct {
	id        string
	sessionID string
	agentID   string
	provider  string
	model     string
	state     string
	startedAt time.Time
	cancel    context.CancelFunc

	mu        sync.Mutex
	questions []agent.AskQuestion
	gotOutput bool
	timedOut  bool
}

func (r *run) markOutput() {
	r.mu.Lock()
	r.gotOutput = true
	r.mu.Unlock()
}

func (r *run) awaitingModel() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.gotOutput
}

func (r *run) markTimedOut() {
	r.mu.Lock()
	r.timedOut = true
	r.mu.Unlock()
}

func (r *run) didTimeout() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.timedOut
}

func (r *run) setQuestions(qs []agent.AskQuestion) {
	r.mu.Lock()
	r.questions = qs
	r.mu.Unlock()
}

func (r *run) setState(s string) {
	r.mu.Lock()
	r.state = s
	r.mu.Unlock()
}

func (r *run) snapshot() RunInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RunInfo{
		ID:        r.id,
		SessionID: r.sessionID,
		Provider:  r.provider,
		Model:     r.model,
		State:     r.state,
		StartedAt: r.startedAt.Unix(),
	}
}

type runManager struct {
	c          *Connector
	base       context.Context
	mu         sync.Mutex
	runs       map[string]*run
	byAgentMap map[string]*run
	sem        chan struct{}
}

func newRunManager(c *Connector) *runManager {
	n := c.cfg.MaxConcurrency
	if n <= 0 {
		n = config.DefaultConnectConcurrency
	}
	return &runManager{
		c:          c,
		base:       c.runCtx,
		runs:       make(map[string]*run),
		byAgentMap: make(map[string]*run),
		sem:        make(chan struct{}, n),
	}
}

func (m *runManager) byAgent(agentID string) *run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byAgentMap[agentID]
}

func (m *runManager) get(id string) *run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

func (m *runManager) register(r *run) {
	m.mu.Lock()
	m.runs[r.id] = r
	m.byAgentMap[r.agentID] = r
	m.mu.Unlock()
}

func (m *runManager) release(r *run) {
	m.mu.Lock()
	delete(m.runs, r.id)
	delete(m.byAgentMap, r.agentID)
	m.mu.Unlock()
	select {
	case <-m.sem:
	default:
	}
}

func (m *runManager) tryAcquire() bool {
	select {
	case m.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *runManager) Cancel(id string) error {
	r := m.get(id)
	if r == nil {
		return &FrameError{Code: ErrCodeNotFound, Message: "run not found: " + id}
	}
	r.cancel()
	return nil
}

func (m *runManager) Answer(id string, answers map[string]string) error {
	r := m.get(id)
	if r == nil {
		return &FrameError{Code: ErrCodeNotFound, Message: "run not found: " + id}
	}
	if m.c.deps.Bus == nil {
		return &FrameError{Code: ErrCodeUnavailable, Message: "event bus unavailable"}
	}
	m.c.deps.Bus.Publish(agent.TopicAgentAskAnswer, agent.AgentAskAnswer{
		AgentID: r.agentID,
		Answers: answers,
	})
	return nil
}

func (m *runManager) List() []RunInfo {
	m.mu.Lock()
	runs := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	out := make([]RunInfo, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

func (m *runManager) cancelAll() {
	m.mu.Lock()
	runs := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	for _, r := range runs {
		r.cancel()
	}
}

func (m *runManager) resolveModel(provider, model string) (llm.Provider, llm.Model, error) {
	if m.c.deps.LLM == nil {
		return nil, llm.Model{}, &FrameError{Code: ErrCodeUnavailable, Message: "llm manager unavailable"}
	}
	m.c.deps.LLM.WaitUntilReady()

	mode := llm.SelectModePreferred
	if model != "" {
		mode = llm.SelectModeExact
	}
	res := m.c.deps.LLM.Select(llm.SelectRequest{Mode: mode, Provider: provider, Model: model})
	if res.Err != nil {
		return nil, llm.Model{}, &FrameError{Code: ErrCodeBadRequest, Message: "model selection failed: " + res.Err.Error()}
	}
	prov, err := m.c.deps.LLM.Provider(res.Provider)
	if err != nil {
		return nil, llm.Model{}, &FrameError{Code: ErrCodeBadRequest, Message: "provider unavailable: " + err.Error()}
	}
	return prov, res.Model, nil
}

func newRunID() string { return uuid.NewString() }
