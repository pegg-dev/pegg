package connect

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
)

func TestHandleStreamEmitsFrames(t *testing.T) {
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	cs := &connState{out: make(chan *Frame, 32), done: make(chan struct{})}
	c.cur.Store(cs)

	r := &run{id: "r1", sessionID: "s1", agentID: "a1", provider: "p", model: "m", state: runStateRunning}
	m := c.runs

	m.handleStream(r, agent.StreamEvent{Type: agent.StreamToken, Content: "hello "})
	m.handleStream(r, agent.StreamEvent{Type: agent.StreamToken, Reasoning: "think"})
	m.handleStream(r, agent.StreamEvent{Type: agent.StreamToolCall, ToolCall: &llm.ToolCall{
		ID:       "c1",
		Function: llm.Function{Name: "bash", Arguments: `{"cmd":"ls"}`},
	}})
	m.handleStream(r, agent.StreamEvent{Type: agent.StreamToolResult, ToolCall: &llm.ToolCall{
		ID:       "c1",
		Function: llm.Function{Name: "bash"},
	}, ToolOutput: "ok"})

	want := []string{eventRunToken, eventRunToken, eventRunToolCall, eventRunToolResult}
	for _, name := range want {
		select {
		case f := <-cs.out:
			if f.Type != frameEvent || f.Event != name {
				t.Fatalf("got type=%s event=%s, want event=%s", f.Type, f.Event, name)
			}
			if f.RunID != "r1" || f.SessionID != "s1" {
				t.Fatalf("frame missing run/session id: %+v", f)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s event", name)
		}
	}
}

func TestStartRejectsEmptyMessage(t *testing.T) {
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if _, err := c.runs.Start(StartParams{Message: "   "}); err == nil {
		t.Fatal("expected error for empty message")
	}
}

type fakeProvider struct{ name string }

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{}, nil
}
func (p *fakeProvider) ChatStream(_ context.Context, _ *llm.Request, handler llm.StreamHandler) error {
	if err := handler(llm.StreamChunk{Content: "Hello"}); err != nil {
		return err
	}
	if err := handler(llm.StreamChunk{Content: " world"}); err != nil {
		return err
	}
	return handler(llm.StreamChunk{FinishReason: llm.FinishReasonStop})
}
func (p *fakeProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "fake-1", Name: "Fake"}}, nil
}

func TestExecuteEmitsRunLifecycle(t *testing.T) {
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	cs := &connState{out: make(chan *Frame, 64), done: make(chan struct{})}
	c.cur.Store(cs)

	orch := agent.New("orchestrator",
		agent.WithProvider(&fakeProvider{name: "fake"}),
		agent.WithModel(llm.Model{ID: "fake-1"}),
	)
	r := &run{id: "r1", sessionID: "s1", agentID: orch.ID, provider: "fake", model: "fake-1", state: runStateRunning}

	done := make(chan struct{})
	go func() {
		c.runs.execute(c.runCtx, r, orch, "hi", nil)
		close(done)
	}()

	var events []string
	var tokenText strings.Builder
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case f := <-cs.out:
			if f.Type != frameEvent {
				continue
			}
			events = append(events, f.Event)
			if f.Event == eventRunToken {
				var p runTokenPayload
				_ = jsonUnmarshal(f.Data, &p)
				tokenText.WriteString(p.Content)
			}
			if f.Event == eventRunDone {
				break loop
			}
		case <-timeout:
			t.Fatalf("timed out; events so far: %v", events)
		}
	}
	<-done

	if len(events) == 0 || events[0] != eventRunStarted {
		t.Fatalf("first event = %v, want %s", events, eventRunStarted)
	}
	if got := tokenText.String(); got != "Hello world" {
		t.Fatalf("streamed text = %q, want %q", got, "Hello world")
	}
	if last := events[len(events)-1]; last != eventRunDone {
		t.Fatalf("last event = %s, want %s (events: %v)", last, eventRunDone, events)
	}
}

func TestWatchdogFailsStalledRun(t *testing.T) {
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, func() *config.ConnectConfig {
		cc := config.DefaultConfig().Connect
		cc.ResponseTimeoutSecs = 1
		return cc
	}())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	cs := &connState{out: make(chan *Frame, 64), done: make(chan struct{})}
	c.cur.Store(cs)

	orch := agent.New("orchestrator",
		agent.WithProvider(&blockingProvider{}),
		agent.WithModel(llm.Model{ID: "block-1"}),
	)
	runCtx, cancel := context.WithCancel(c.runCtx)
	r := &run{id: "r1", sessionID: "s1", agentID: orch.ID, provider: "block", model: "block-1", state: runStateRunning, cancel: cancel}

	go c.runs.execute(runCtx, r, orch, "hi", nil)

	timeout := time.After(6 * time.Second)
	for {
		select {
		case f := <-cs.out:
			if f.Type == frameEvent && f.Event == eventRunError {
				var p runErrorPayload
				_ = jsonUnmarshal(f.Data, &p)
				if !strings.Contains(p.Message, "did not respond") {
					t.Fatalf("unexpected error: %q", p.Message)
				}
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for the watchdog to fail the stalled run")
		}
	}
}

type blockingProvider struct{}

func (p *blockingProvider) Name() string { return "block" }
func (p *blockingProvider) Chat(ctx context.Context, _ *llm.Request) (*llm.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (p *blockingProvider) ChatStream(ctx context.Context, _ *llm.Request, _ llm.StreamHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (p *blockingProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "block-1"}}, nil
}

type errorProvider struct{}

func (p *errorProvider) Name() string { return "err" }
func (p *errorProvider) Chat(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, errors.New("boom: invalid api key")
}
func (p *errorProvider) ChatStream(context.Context, *llm.Request, llm.StreamHandler) error {
	return errors.New("boom: invalid api key")
}
func (p *errorProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "err-1"}}, nil
}

func TestExecuteEmitsRunError(t *testing.T) {
	c, err := New(Deps{Config: config.DefaultConfig(), Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	cs := &connState{out: make(chan *Frame, 64), done: make(chan struct{})}
	c.cur.Store(cs)

	orch := agent.New("orchestrator",
		agent.WithProvider(&errorProvider{}),
		agent.WithModel(llm.Model{ID: "err-1"}),
	)
	r := &run{id: "r1", sessionID: "s1", agentID: orch.ID, provider: "err", model: "err-1", state: runStateRunning}

	done := make(chan struct{})
	go func() {
		c.runs.execute(c.runCtx, r, orch, "hi", nil)
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case f := <-cs.out:
			if f.Type == frameEvent && f.Event == eventRunError {
				var p runErrorPayload
				_ = jsonUnmarshal(f.Data, &p)
				if !strings.Contains(p.Message, "boom") {
					t.Fatalf("error message missing provider detail: %q", p.Message)
				}
				<-done
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for run.error event")
		}
	}
}

func TestForwardsAgentErrorAndRetry(t *testing.T) {
	bus := event.New()
	c, err := New(Deps{Config: config.DefaultConfig(), Bus: bus, Log: testLogger(), Version: "test"}, config.DefaultConfig().Connect)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	cs := &connState{out: make(chan *Frame, 32), done: make(chan struct{})}
	c.cur.Store(cs)

	bus.Publish(agent.TopicAgentError, agent.AgentError{
		AgentID: "a1", AgentName: "orchestrator", Err: errors.New("API error (HTTP 426)"),
	})
	bus.Publish(agent.TopicErrorMessage, agent.ErrorMessage{
		AgentID: "a1", Message: "Request failed (attempt 1): API error (HTTP 426) — retrying in 1s",
	})

	got := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case f := <-cs.out:
			if f.Type == frameEvent {
				got[f.Event] = true
			}
		case <-timeout:
			t.Fatalf("missing forwarded events, got %v", got)
		}
	}
	if !got[eventRunAgentError] {
		t.Fatalf("expected %s, got %v", eventRunAgentError, got)
	}
	if !got[eventRunWarning] {
		t.Fatalf("expected %s, got %v", eventRunWarning, got)
	}
}
