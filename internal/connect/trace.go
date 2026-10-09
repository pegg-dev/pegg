package connect

import (
	"context"
	"time"

	"github.com/peggco/pegg/internal/llm"
)

func (c *Connector) trace(format string, args ...any) {
	if c.deps.Trace != nil {
		c.deps.Trace(format, args...)
		return
	}
	if c.log != nil {
		c.log.Finfo(format, args...)
	}
}

type traceProvider struct {
	inner llm.Provider
	trace func(string, ...any)
	runID string
}

func (p *traceProvider) Name() string { return p.inner.Name() }

func (p *traceProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	p.trace("run %s -> %s/%s (non-stream)", shortID(p.runID), p.inner.Name(), req.Model)
	start := time.Now()
	resp, err := p.inner.Chat(ctx, req)
	if err != nil {
		p.trace("run %s <- error after %s: %v", shortID(p.runID), time.Since(start).Round(time.Millisecond), err)
		return nil, err
	}
	p.trace("run %s <- response after %s", shortID(p.runID), time.Since(start).Round(time.Millisecond))
	return resp, nil
}

func (p *traceProvider) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	p.trace("run %s -> %s/%s (stream)", shortID(p.runID), p.inner.Name(), req.Model)
	start := time.Now()
	first := false
	err := p.inner.ChatStream(ctx, req, func(chunk llm.StreamChunk) error {
		if !first {
			first = true
			p.trace("run %s <- first chunk after %s", shortID(p.runID), time.Since(start).Round(time.Millisecond))
		}
		return handler(chunk)
	})
	if err != nil {
		p.trace("run %s <- stream error after %s: %v", shortID(p.runID), time.Since(start).Round(time.Millisecond), err)
		return err
	}
	p.trace("run %s <- stream closed after %s", shortID(p.runID), time.Since(start).Round(time.Millisecond))
	return nil
}

func (p *traceProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return p.inner.ListModels(ctx)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
