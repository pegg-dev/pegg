package connect

import (
	"context"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type StartParams struct {
	Message         string
	SessionID       string
	Provider        string
	Model           string
	ReasoningEffort string
	Files           []string
}

func (m *runManager) Start(p StartParams) (RunInfo, error) {
	if strings.TrimSpace(p.Message) == "" {
		return RunInfo{}, &FrameError{Code: ErrCodeBadRequest, Message: "message is required"}
	}
	if m.c.deps.Sessions == nil {
		return RunInfo{}, &FrameError{Code: ErrCodeUnavailable, Message: "session store unavailable"}
	}
	if !m.tryAcquire() {
		return RunInfo{}, &FrameError{Code: ErrCodeConflict, Message: "maximum concurrent runs reached"}
	}
	acquired := true
	defer func() {
		if acquired {
			select {
			case <-m.sem:
			default:
			}
		}
	}()

	orch, err := agents.New("orchestrator")
	if err != nil {
		return RunInfo{}, &FrameError{Code: ErrCodeInternal, Message: "create agent: " + err.Error()}
	}
	orch.Bus = m.c.deps.Bus

	runID := newRunID()

	prov, mdl, err := m.resolveModel(p.Provider, p.Model)
	if err != nil {
		return RunInfo{}, err
	}
	orch.SetModelProvider(mdl, &traceProvider{inner: prov, trace: m.c.trace, runID: runID})
	m.c.trace("run %s starting on %s/%s", shortID(runID), prov.Name(), mdl.ID)
	if p.ReasoningEffort != "" {
		orch.ReasoningEffort = p.ReasoningEffort
	}

	if len(p.Files) > 0 {
		atts, aerr := loadAttachments(p.Files)
		if aerr != nil {
			return RunInfo{}, &FrameError{Code: ErrCodeBadRequest, Message: aerr.Error()}
		}
		orch.Attachments = atts
	}

	var (
		sessionID string
		history   []llm.Message
	)
	if p.SessionID != "" {
		msgs, merr := m.c.deps.Sessions.Messages(p.SessionID)
		if merr != nil {
			return RunInfo{}, &FrameError{Code: ErrCodeNotFound, Message: "session not found: " + p.SessionID}
		}
		sessionID = p.SessionID
		if orch.SystemPrompt != "" {
			history = append(history, llm.SystemMessage(orch.SystemPrompt))
		}
		history = append(history, session.MessagesToLLM(msgs)...)
	} else {
		s, serr := m.c.deps.Sessions.Create(session.CreateOptions{
			Provider:        prov.Name(),
			Model:           mdl.ID,
			ReasoningEffort: orch.ReasoningEffort,
			ProjectDir:      m.c.workspace(),
		})
		if serr != nil {
			return RunInfo{}, &FrameError{Code: ErrCodeInternal, Message: "create session: " + serr.Error()}
		}
		sessionID = s.ID
	}

	runCtx, cancel := context.WithCancel(m.base)
	r := &run{
		id:        runID,
		sessionID: sessionID,
		agentID:   orch.ID,
		provider:  prov.Name(),
		model:     mdl.ID,
		state:     runStateRunning,
		startedAt: time.Now(),
		cancel:    cancel,
	}
	m.register(r)
	acquired = false

	if m.c.deps.Bus != nil {
		m.c.deps.Bus.Publish(session.TopicSessionResume, session.SessionResume{
			AgentID:   orch.ID,
			SessionID: sessionID,
		})
	}

	go m.execute(runCtx, r, orch, p.Message, history)
	return r.snapshot(), nil
}
