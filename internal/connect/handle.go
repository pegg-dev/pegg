package connect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/event"
)

const topicAgentAsk = agent.TopicAgentAsk

func (c *Connector) subscribeBus(bus event.Bus) error {
	if bus == nil {
		return nil
	}
	if err := bus.Subscribe(topicAgentAsk, c.onAgentAsk); err != nil {
		return fmt.Errorf("connect: subscribe agent ask: %w", err)
	}
	_ = bus.Subscribe(agent.TopicAgentError, c.onAgentError)
	_ = bus.Subscribe(agent.TopicErrorMessage, c.onErrorMessage)
	_ = bus.Subscribe(agent.TopicErrorMessageFinished, c.onErrorMessageFinished)
	return nil
}

func (c *Connector) handleRequest(ctx context.Context, f *Frame) {
	resp := &Frame{Type: frameResp, ID: f.ID}

	if !c.methodAllowed(f.Method) {
		resp.Error = &FrameError{Code: ErrCodeForbidden, Message: "method not allowed: " + f.Method}
		Audit("method.denied", map[string]any{"method": f.Method, "reason": "allowlist"})
		c.send(resp)
		return
	}
	spec, ok := c.dispatch.lookup(f.Method)
	if !ok {
		resp.Error = &FrameError{Code: ErrCodeNotFound, Message: "unknown method: " + f.Method}
		c.send(resp)
		return
	}
	if !hasScope(c.grantedScopes(), spec.scope) {
		resp.Error = &FrameError{Code: ErrCodeForbidden, Message: "missing scope: " + spec.scope}
		Audit("method.denied", map[string]any{"method": f.Method, "reason": "scope", "need": spec.scope})
		c.send(resp)
		return
	}

	req := &Request{ID: f.ID, SessionID: f.SessionID, RunID: f.RunID, Params: f.Params}
	result, err := c.callHandler(spec.fn, ctx, req)
	if err != nil {
		resp.Error = asFrameError(err)
		Audit("method.error", map[string]any{"method": f.Method, "error": err.Error()})
	} else {
		payload, merr := marshal(result)
		if merr != nil {
			resp.Error = &FrameError{Code: ErrCodeInternal, Message: merr.Error()}
		} else {
			resp.Result = payload
		}
		Audit("method.ok", map[string]any{"method": f.Method})
	}
	c.send(resp)
}

func (c *Connector) callHandler(fn HandlerFunc, ctx context.Context, req *Request) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &FrameError{Code: ErrCodeInternal, Message: fmt.Sprintf("panic: %v", r)}
			c.log.Fwarn("connect: handler panic recovered: %v", r)
		}
	}()
	return fn(ctx, req)
}

func (c *Connector) onAgentAsk(e agent.AgentAsk) {
	run := c.runs.byAgent(e.AgentID)
	if run == nil {
		return
	}
	run.setQuestions(e.Questions)
	event := eventRunAsk
	if len(e.Questions) == 1 && e.Questions[0].ID == "decision" {
		event = eventRunPermission
	}
	c.emit(event, run.sessionID, run.id, runAskPayload{AgentName: e.AgentName, Questions: e.Questions})
}

func (c *Connector) onAgentError(e agent.AgentError) {
	msg := ""
	if e.Err != nil {
		msg = e.Err.Error()
	}
	sessionID, runID := c.runContextFor(e.AgentID)
	c.emit(eventRunAgentError, sessionID, runID, runAgentErrorPayload{
		AgentID:   e.AgentID,
		AgentName: e.AgentName,
		Message:   msg,
	})
}

func (c *Connector) onErrorMessage(e agent.ErrorMessage) {
	sessionID, runID := c.runContextFor(e.AgentID)
	c.emit(eventRunWarning, sessionID, runID, runWarningPayload{Message: e.Message})
}

func (c *Connector) onErrorMessageFinished(e agent.ErrorMessageFinished) {
	sessionID, runID := c.runContextFor(e.AgentID)
	c.emit(eventRunWarnClear, sessionID, runID, nil)
}

func (c *Connector) runContextFor(agentID string) (string, string) {
	if run := c.runs.byAgent(agentID); run != nil {
		return run.sessionID, run.id
	}
	return "", ""
}

func (c *Connector) workspace() string {
	if c.deps.VFS != nil {
		if root := c.deps.VFS.Root(); root != "" {
			return root
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

func (c *Connector) grantedScopes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.scopes...)
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{
			ScopeRead, ScopeSessionsRead, ScopeSessionsWrite, ScopeChat,
			ScopeSettingsRead, ScopeSettingsWrite, ScopeExec,
		}
	}
	return scopes
}

func (c *Connector) capabilities() []string {
	return []string{"chat", "sessions", "settings", "models", "streaming", "parallel-runs"}
}

func (c *Connector) methodAllowed(method string) bool {
	if len(c.cfg.AllowedMethods) == 0 {
		return true
	}
	for _, m := range c.cfg.AllowedMethods {
		if m == method {
			return true
		}
		if strings.HasSuffix(m, "*") && strings.HasPrefix(method, strings.TrimSuffix(m, "*")) {
			return true
		}
	}
	return false
}

func asFrameError(err error) *FrameError {
	var fe *FrameError
	if errors.As(err, &fe) {
		return fe
	}
	return &FrameError{Code: ErrCodeInternal, Message: err.Error()}
}
