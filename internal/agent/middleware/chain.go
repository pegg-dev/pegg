package middleware

import (
	"context"
	"sync"

	"github.com/peggco/pegg/internal/llm"
)

type Chain struct {
	mu  sync.RWMutex
	mws []Middleware
}

func NewChain(mws ...Middleware) *Chain {
	return &Chain{mws: mws}
}

func (c *Chain) Append(mws ...Middleware) {
	if len(mws) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range mws {
		if m == nil {
			continue
		}
		c.mws = append(c.mws, m)
	}
}

func (c *Chain) snapshot() []Middleware {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Middleware(nil), c.mws...)
}

func (c *Chain) Clone() *Chain {
	return &Chain{mws: c.snapshot()}
}

func (c *Chain) BeforeRun(ctx context.Context, agentName, input string) error {
	for _, m := range c.snapshot() {
		if err := m.BeforeRun(ctx, agentName, input); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) AfterRun(ctx context.Context, agentName string, result *Result, runErr error) error {
	for _, m := range c.snapshot() {
		if err := m.AfterRun(ctx, agentName, result, runErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) BeforeLLM(ctx context.Context, req *llm.Request) error {
	for _, m := range c.snapshot() {
		if err := m.BeforeLLM(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) AfterLLM(ctx context.Context, req *llm.Request, resp *llm.Response) error {
	for _, m := range c.snapshot() {
		if err := m.AfterLLM(ctx, req, resp); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) BeforeTool(ctx context.Context, call llm.ToolCall) error {
	for _, m := range c.snapshot() {
		if err := m.BeforeTool(ctx, call); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) AfterTool(ctx context.Context, call llm.ToolCall, output string, toolErr error) error {
	for _, m := range c.snapshot() {
		if err := m.AfterTool(ctx, call, output, toolErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Chain) InvokeLLM(ctx context.Context, req *llm.Request, next LLMInvoker) (*llm.Response, error) {
	mws := c.snapshot()
	invoke := next
	for i := len(mws) - 1; i >= 0; i-- {
		w, ok := mws[i].(InvokeLLM)
		if !ok {
			continue
		}
		w, prev := w, invoke
		invoke = func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
			return w.InvokeLLM(ctx, req, prev)
		}
	}
	return invoke(ctx, req)
}

func (c *Chain) InvokeLLMStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler, next LLMStreamInvoker) error {
	mws := c.snapshot()
	invoke := next
	for i := len(mws) - 1; i >= 0; i-- {
		w, ok := mws[i].(InvokeLLMStream)
		if !ok {
			continue
		}
		w, prev := w, invoke
		invoke = func(ctx context.Context, req *llm.Request, h llm.StreamHandler) error {
			return w.InvokeLLMStream(ctx, req, h, prev)
		}
	}
	return invoke(ctx, req, handler)
}

func (c *Chain) InvokeTool(ctx context.Context, call llm.ToolCall, next ToolInvoker) (string, error) {
	mws := c.snapshot()
	invoke := next
	for i := len(mws) - 1; i >= 0; i-- {
		w, ok := mws[i].(InvokeTool)
		if !ok {
			continue
		}
		w, prev := w, invoke
		invoke = func(ctx context.Context, call llm.ToolCall) (string, error) {
			return w.InvokeTool(ctx, call, prev)
		}
	}
	return invoke(ctx, call)
}

func (c *Chain) OnError(ctx context.Context, runErr error) error {
	var last error
	for _, m := range c.snapshot() {
		if err := m.OnError(ctx, runErr); err != nil {
			last = err
		}
	}
	return last
}
