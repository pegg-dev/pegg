package permission

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/peggco/pegg/internal/agent"
	agentmw "github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/core/config"
	decisionapi "github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/vfs"
)

type Mode string

const (
	ModeAllow     Mode = "allow"
	ModeSemiAsk   Mode = "semi-ask"
	ModeAsk       Mode = "ask"
	ModeSemiJudge Mode = "semi-judge"
	ModeJudge     Mode = "judge"
)

var validModes = map[Mode]bool{
	ModeAllow:     true,
	ModeSemiAsk:   true,
	ModeAsk:       true,
	ModeSemiJudge: true,
	ModeJudge:     true,
}

func AllModes() []Mode {
	return []Mode{ModeAllow, ModeSemiAsk, ModeAsk, ModeSemiJudge, ModeJudge}
}

const defaultMode = ModeSemiAsk

type Deps struct {
	Config   *config.PermissionConfig
	LLM      *llm.Manager
	Decision *decisionapi.Manager
	Store    *Store
}

type Middleware struct {
	agentmw.BaseMiddleware

	cfg        *config.PermissionConfig
	store      *Store
	llm        *llm.Manager
	decision   *decisionapi.Manager
	judgeOnce  sync.Once
	judgeAgent *agent.Agent
}

func New(deps Deps) *Middleware {
	m := &Middleware{
		cfg:      deps.Config,
		store:    deps.Store,
		llm:      deps.LLM,
		decision: deps.Decision,
	}
	if m.store == nil {
		m.store = NewStore()
	}
	return m
}

func (m *Middleware) UpdateConfig(cfg *config.PermissionConfig) {
	m.cfg = cfg
}

func (m *Middleware) judge() *agent.Agent {
	if m.judgeAgent != nil {
		return m.judgeAgent
	}
	if m.llm == nil {
		return nil
	}
	m.judgeOnce.Do(func() {
		if prov, model, ok := m.resolveJudge(); ok {
			m.judgeAgent = newJudgeAgent(prov, model)
		}
	})
	return m.judgeAgent
}

func (m *Middleware) modeFor(name string) Mode {
	if m.cfg != nil {
		if r, ok := m.cfg.Rules[name]; ok {
			return parseMode(r)
		}
		return parseMode(m.cfg.Default)
	}
	return defaultMode
}

func parseMode(s string) Mode {
	mode := Mode(s)
	if validModes[mode] {
		return mode
	}
	return ModeAsk
}

type DeniedError struct {
	ToolName string
	Reason   string
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("permission denied for tool %q: %s", e.ToolName, e.Reason)
}

func (m *Middleware) InvokeTool(ctx context.Context, call llm.ToolCall, next agentmw.ToolInvoker) (string, error) {
	name := call.Function.Name
	mode := m.modeFor(name)

	switch mode {
	case ModeAllow:
		return next(WithUnrestricted(ctx), call)

	case ModeAsk, ModeJudge:
		return m.gate(ctx, call, next, mode, nil)

	case ModeSemiAsk, ModeSemiJudge:
		if name == "bash" {
			if bashAllowed(call.Function.Arguments) {
				return next(ctx, call)
			}
			return m.gate(ctx, call, next, mode, nil)
		}
		output, err := next(ctx, call)
		if err == nil {
			return output, nil
		}
		if !m.isPermissionError(ctx, call, err) {
			return output, err
		}
		return m.gate(ctx, call, next, mode, err)
	}
	return next(ctx, call)
}

func (m *Middleware) isPermissionError(ctx context.Context, call llm.ToolCall, err error) bool {
	if err == nil {
		return false
	}
	a := agent.FromContext(ctx)
	if a == nil {
		return false
	}
	t, ok := a.Tools.Get(call.Function.Name)
	if !ok {
		return false
	}
	pa, ok := t.(tool.PermissionAware)
	if !ok {
		return false
	}
	return pa.IsPermissionError(err)
}

func (m *Middleware) gate(ctx context.Context, call llm.ToolCall, next agentmw.ToolInvoker, mode Mode, permErr error) (string, error) {
	if m.store != nil {
		key := m.store.Key(call.Function.Name, call.Function.Arguments)
		if reason, ok := m.store.RejectionReason(key); ok {
			return "", &DeniedError{ToolName: call.Function.Name, Reason: "previously rejected: " + reason}
		}
		if m.store.IsAllowed(key) {
			return m.runApproved(ctx, call, next, permErr)
		}
	}

	dec, err := m.ask(ctx, call, mode, permErr)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &DeniedError{ToolName: call.Function.Name, Reason: err.Error()}
	}
	if dec.Allow {
		if dec.Persist && m.store != nil {
			m.store.Allow(call.Function.Name, call.Function.Arguments)
		}
		return m.runApproved(ctx, call, next, permErr)
	}
	if m.store != nil {
		m.store.Reject(call.Function.Name, call.Function.Arguments, dec.Reason)
	}
	return "", &DeniedError{ToolName: call.Function.Name, Reason: dec.Reason}
}

func (m *Middleware) runApproved(ctx context.Context, call llm.ToolCall, next agentmw.ToolInvoker, permErr error) (string, error) {
	runCtx := WithUnrestricted(ctx)
	if permErr != nil {
		var oob *vfs.OutOfBoundsError
		if errors.As(permErr, &oob) && oob.Path != "" {
			runCtx = WithPermittedPath(ctx, oob.Path)
		}
	}
	return next(runCtx, call)
}
