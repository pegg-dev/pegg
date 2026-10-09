package middlewares

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

func TestRetryBackoffSchedule(t *testing.T) {
	r := NewRetry()
	want := []time.Duration{time.Second, 3 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute, 5 * time.Minute}
	for i, d := range want {
		if got := r.backoffFor(i + 1); got != d {
			t.Errorf("backoffFor(%d) = %v, want %v", i+1, got, d)
		}
	}
	if got := r.backoffFor(len(want) + 5); got != 5*time.Minute {
		t.Errorf("backoff beyond schedule = %v, want 5m", got)
	}
}

func TestShouldRetry(t *testing.T) {
	r := NewRetry()
	ctx := context.Background()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limited", &llm.ProviderError{StatusCode: 429}, true},
		{"server error", &llm.ProviderError{StatusCode: 500}, true},
		{"service unavailable", &llm.ProviderError{StatusCode: 503}, true},
		{"invalid api key", &llm.ProviderError{StatusCode: 401}, false},
		{"forbidden", &llm.ProviderError{StatusCode: 403}, false},
		{"not found", &llm.ProviderError{StatusCode: 404}, false},
		{"bad request", &llm.ProviderError{StatusCode: 422}, false},
		{"network error", &url.Error{Op: "Post", URL: "https://x", Err: errors.New("connection refused")}, true},
		{"unexpected error", errors.New("boom"), true},
	}
	for _, tc := range cases {
		if got := r.shouldRetry(ctx, tc.err); got != tc.want {
			t.Errorf("%s: shouldRetry = %v, want %v", tc.name, got, tc.want)
		}
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if r.shouldRetry(cancelCtx, &llm.ProviderError{StatusCode: 500}) {
		t.Error("shouldRetry must be false when context is cancelled")
	}
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))
	req := llm.NewRequest("m", nil)

	calls := 0
	resp, err := r.InvokeLLM(context.Background(), req, func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
		calls++
		if calls < 3 {
			return nil, &llm.ProviderError{StatusCode: 503, Message: "unavailable"}
		}
		return &llm.Response{Model: req.Model}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if resp.Model != "m" {
		t.Fatalf("resp.Model = %q, want m", resp.Model)
	}
}

func TestRetryNonRetryablePassesThrough(t *testing.T) {
	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))
	req := llm.NewRequest("m", nil)

	boom := &llm.ProviderError{StatusCode: 401, Message: "invalid api key"}
	calls := 0
	_, err := r.InvokeLLM(context.Background(), req, func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
		calls++
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected original error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryStopsOnCancelDuringBackoff(t *testing.T) {
	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Hour}))
	req := llm.NewRequest("m", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
		close(done)
	}()

	_, err := r.InvokeLLM(ctx, req, func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
		calls++
		return nil, errors.New("transient failure")
	})
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryPublishesErrorMessageEvents(t *testing.T) {
	bus := event.New()
	a := agent.New("test-agent", agent.WithBus(bus))
	ctx := agent.WithAgent(context.Background(), a)

	var messages []string
	var finished bool
	bus.Subscribe(agent.TopicErrorMessage, func(ev agent.ErrorMessage) {
		if ev.AgentID != a.ID {
			t.Errorf("event AgentID = %q, want %q", ev.AgentID, a.ID)
		}
		messages = append(messages, ev.Message)
	})
	bus.Subscribe(agent.TopicErrorMessageFinished, func(ev agent.ErrorMessageFinished) {
		if ev.AgentID != a.ID {
			t.Errorf("finished event AgentID = %q, want %q", ev.AgentID, a.ID)
		}
		finished = true
	})

	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))
	req := llm.NewRequest("m", nil)

	calls := 0
	_, err := r.InvokeLLM(ctx, req, func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("transient")
		}
		return &llm.Response{Model: "m"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("error messages = %d, want 2: %v", len(messages), messages)
	}
	for _, m := range messages {
		if !strings.Contains(m, "Request failed") || !strings.Contains(m, "retrying in") {
			t.Errorf("message = %q, want failure + backoff info", m)
		}
	}
	if !finished {
		t.Fatal("expected final finished event")
	}
}

func TestRetryStreamRetriesBeforeOutput(t *testing.T) {
	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))
	req := llm.NewRequest("m", nil)

	calls := 0
	var chunks []string
	err := r.InvokeLLMStream(context.Background(), req, func(chunk llm.StreamChunk) error {
		chunks = append(chunks, chunk.Content)
		return nil
	}, func(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
		calls++
		if calls == 1 {
			return errors.New("connection reset")
		}
		return handler(llm.StreamChunk{Content: "ok"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(chunks) != 1 || chunks[0] != "ok" {
		t.Fatalf("chunks = %v, want [ok]", chunks)
	}
}

func TestRetryStreamNotRetriedAfterOutput(t *testing.T) {
	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))
	req := llm.NewRequest("m", nil)

	boom := errors.New("mid-stream failure")
	calls := 0
	err := r.InvokeLLMStream(context.Background(), req, func(llm.StreamChunk) error { return nil }, func(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
		calls++
		if err := handler(llm.StreamChunk{Content: "partial"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected original error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryPublishesFinishedOnCancel(t *testing.T) {
	bus := event.New()
	a := agent.New("test-agent", agent.WithBus(bus))
	ctx, cancel := context.WithCancel(agent.WithAgent(context.Background(), a))
	defer cancel()

	finished := make(chan struct{})
	bus.Subscribe(agent.TopicErrorMessageFinished, func(agent.ErrorMessageFinished) {
		close(finished)
	})

	r := NewRetry(WithBackoffSchedule([]time.Duration{time.Hour}))
	req := llm.NewRequest("m", nil)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := r.InvokeLLM(ctx, req, func(ctx context.Context, req *llm.Request) (*llm.Response, error) {
		return nil, errors.New("transient")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("expected finished event after cancellation")
	}
}

func TestErrorMessageFormat(t *testing.T) {
	msg := errorMessage(3, 10*time.Second, errors.New("connection reset"))
	if !strings.Contains(msg, "attempt 3") || !strings.Contains(msg, "connection reset") || !strings.Contains(msg, "10s") {
		t.Fatalf("message = %q, want attempt + error + backoff", msg)
	}
}

type flakyProvider struct {
	mu       int
	failures int
}

func (p *flakyProvider) Name() string { return "flaky" }

func (p *flakyProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "mock-model"}}, nil
}

func (p *flakyProvider) Chat(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if p.mu < p.failures {
		p.mu++
		return nil, &llm.ProviderError{StatusCode: 503, Message: "unavailable"}
	}
	msg := llm.AssistantMessage("done")
	fr := llm.FinishReasonStop
	return &llm.Response{
		Model:   "mock-model",
		Choices: []llm.Choice{{Index: 0, Message: &msg, FinishReason: &fr}},
	}, nil
}

func (p *flakyProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("not used")
}

func TestRetryAgentRunSucceedsAfterTransientFailures(t *testing.T) {
	prov := &flakyProvider{failures: 2}
	a := agent.New("test-agent",
		agent.WithModel(llm.Model{ID: "mock-model"}),
		agent.WithProvider(prov),
		agent.WithMiddleware(NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))),
	)

	res, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "done" {
		t.Fatalf("output = %q, want done", res.Output)
	}
	if prov.mu != 2 {
		t.Fatalf("provider failures consumed = %d, want 2", prov.mu)
	}
}

func TestRetryAgentRunStreamSucceedsAfterTransientFailures(t *testing.T) {
	prov := &flakyStreamProvider{mu: 2}
	a := agent.New("test-agent",
		agent.WithModel(llm.Model{ID: "mock-model"}),
		agent.WithProvider(prov),
		agent.WithMiddleware(NewRetry(WithBackoffSchedule([]time.Duration{time.Millisecond}))),
	)

	var got []string
	res, err := a.RunStream(context.Background(), "hi", func(ev agent.StreamEvent) error {
		if ev.Type == agent.StreamToken {
			got = append(got, ev.Content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "streamed done" {
		t.Fatalf("output = %q, want streamed done", res.Output)
	}
	if len(got) != 1 || got[0] != "streamed done" {
		t.Fatalf("tokens = %v, want [streamed done]", got)
	}
	if prov.mu != 0 {
		t.Fatalf("remaining failures = %d, want 0", prov.mu)
	}
}

type flakyStreamProvider struct {
	mu int
}

func (p *flakyStreamProvider) Name() string { return "flaky" }

func (p *flakyStreamProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "mock-model"}}, nil
}

func (p *flakyStreamProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, errors.New("not used")
}

func (p *flakyStreamProvider) ChatStream(_ context.Context, _ *llm.Request, handler llm.StreamHandler) error {
	if p.mu > 0 {
		p.mu--
		return &llm.ProviderError{StatusCode: 502, Message: "bad gateway"}
	}
	return handler(llm.StreamChunk{Content: "streamed done", FinishReason: llm.FinishReasonStop})
}
