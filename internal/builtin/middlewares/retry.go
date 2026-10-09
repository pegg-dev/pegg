package middlewares

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/peggco/pegg/internal/agent"
	agentmw "github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/llm"
)

var defaultBackoffSchedule = []time.Duration{
	time.Second,
	3 * time.Second,
	10 * time.Second,
	30 * time.Second,
	time.Minute,
	5 * time.Minute,
}

type RetryOption func(*Retry)

func WithBackoffSchedule(schedule []time.Duration) RetryOption {
	return func(r *Retry) {
		if len(schedule) > 0 {
			r.backoff = append([]time.Duration(nil), schedule...)
		}
	}
}

type Retry struct {
	agentmw.BaseMiddleware
	backoff []time.Duration
}

func NewRetry(opts ...RetryOption) *Retry {
	r := &Retry{backoff: append([]time.Duration(nil), defaultBackoffSchedule...)}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Retry) InvokeLLM(ctx context.Context, req *llm.Request, next agentmw.LLMInvoker) (*llm.Response, error) {
	retried := false
	for attempt := 1; ; attempt++ {
		resp, err := next(ctx, req)
		if err == nil {
			if retried {
				r.publishFinished(ctx)
			}
			return resp, nil
		}
		if !r.shouldRetry(ctx, err) {
			if retried {
				r.publishFinished(ctx)
			}
			return nil, err
		}
		retried = true
		wait := r.backoffFor(attempt)
		r.publishError(ctx, errorMessage(attempt, wait, err))
		if err := r.sleep(ctx, wait); err != nil {
			r.publishFinished(ctx)
			return nil, err
		}
	}
}

func (r *Retry) InvokeLLMStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler, next agentmw.LLMStreamInvoker) error {
	tracker := &streamTracker{}
	wrapped := tracker.wrap(handler)
	retried := false
	for attempt := 1; ; attempt++ {
		tracker.reset()
		err := next(ctx, req, wrapped)
		if err == nil {
			if retried {
				r.publishFinished(ctx)
			}
			return nil
		}
		if tracker.delivered || !r.shouldRetry(ctx, err) {
			if retried {
				r.publishFinished(ctx)
			}
			return err
		}
		retried = true
		wait := r.backoffFor(attempt)
		r.publishError(ctx, errorMessage(attempt, wait, err))
		if err := r.sleep(ctx, wait); err != nil {
			r.publishFinished(ctx)
			return err
		}
	}
}

func (r *Retry) shouldRetry(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	var pe *llm.ProviderError
	if errors.As(err, &pe) {
		return pe.Temporary()
	}
	return true
}

func (r *Retry) backoffFor(attempt int) time.Duration {
	if attempt <= 0 {
		return r.backoff[0]
	}
	if attempt > len(r.backoff) {
		return r.backoff[len(r.backoff)-1]
	}
	return r.backoff[attempt-1]
}

func (r *Retry) sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Retry) publishError(ctx context.Context, msg string) {
	a := agent.FromContext(ctx)
	if a == nil || a.Bus == nil {
		return
	}
	a.Bus.Publish(agent.TopicErrorMessage, agent.ErrorMessage{
		AgentID:   a.ID,
		AgentName: a.Name,
		Message:   msg,
	})
}

func (r *Retry) publishFinished(ctx context.Context) {
	a := agent.FromContext(ctx)
	if a == nil || a.Bus == nil {
		return
	}
	a.Bus.Publish(agent.TopicErrorMessageFinished, agent.ErrorMessageFinished{
		AgentID:   a.ID,
		AgentName: a.Name,
	})
}

func errorMessage(attempt int, wait time.Duration, err error) string {
	msg := "Request failed"
	if attempt > 0 {
		msg += fmt.Sprintf(" (attempt %d)", attempt)
	}
	if err != nil {
		msg += ": " + err.Error()
	}
	if wait > 0 {
		msg += " — retrying in " + shortDuration(wait)
	}
	return msg
}

func shortDuration(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d >= time.Second {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return d.String()
}

type streamTracker struct {
	delivered bool
}

func (t *streamTracker) reset() { t.delivered = false }

func (t *streamTracker) wrap(handler llm.StreamHandler) llm.StreamHandler {
	return func(chunk llm.StreamChunk) error {
		if chunk.Content != "" || chunk.Reasoning != "" || len(chunk.ToolCalls) > 0 {
			t.delivered = true
		}
		return handler(chunk)
	}
}
