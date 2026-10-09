package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/agent/middlewares"
	"github.com/peggco/pegg/internal/agent/reminder"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

type mockResponse struct {
	content   string
	reasoning string
	calls     []llm.ToolCall
	usage     llm.Usage
}

type scriptedProvider struct {
	name      string
	mu        sync.Mutex
	responses []mockResponse
	streams   [][]llm.StreamChunk
	lastReq   *llm.Request
}

func (p *scriptedProvider) Name() string { return p.name }

func (p *scriptedProvider) ListModels(_ context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "mock-model"}}, nil
}

func (p *scriptedProvider) Chat(_ context.Context, req *llm.Request) (*llm.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastReq = req
	if len(p.responses) == 0 {
		return nil, errors.New("mock: no more responses")
	}
	m := p.responses[0]
	p.responses = p.responses[1:]
	return responseOf(m), nil
}

func (p *scriptedProvider) ChatStream(_ context.Context, req *llm.Request, handler llm.StreamHandler) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastReq = req
	if len(p.streams) == 0 {
		return errors.New("mock: no more streams")
	}
	chunks := p.streams[0]
	p.streams = p.streams[1:]
	for _, c := range chunks {
		if err := handler(c); err != nil {
			return err
		}
	}
	return nil
}

func responseOf(m mockResponse) *llm.Response {
	msg := llm.AssistantMessage(m.content)
	if m.reasoning != "" {
		msg.Reasoning = m.reasoning
	}
	if len(m.calls) > 0 {
		msg.ToolCalls = m.calls
	}
	fr := llm.FinishReasonStop
	if len(m.calls) > 0 {
		fr = llm.FinishReasonToolCalls
	}
	return &llm.Response{
		Model:   "mock-model",
		Choices: []llm.Choice{{Index: 0, Message: &msg, FinishReason: &fr}},
		Usage:   m.usage,
	}
}

func toolCall(id, name, args string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.Function{
			Name:      name,
			Arguments: args,
		},
	}
}

func newTestAgent(t *testing.T, opts ...Option) (*Agent, *scriptedProvider) {
	t.Helper()
	prov := &scriptedProvider{name: "mock"}
	all := append([]Option{
		WithModel(llm.Model{ID: "mock-model"}),
		WithProvider(prov),
		WithLogger(logger.New(logger.LevelDebug, discardHandler{})),
	}, opts...)
	return New("test-agent", all...), prov
}

func echoTool() tool.Tool {
	return tool.NewSpec("echo", "echoes the input", map[string]any{"type": "object"}, func(_ context.Context, args string) (string, error) {
		return "echo:" + args, nil
	})
}

type countingMiddleware struct {
	middleware.BaseMiddleware
	count *int
}

func (m *countingMiddleware) BeforeRun(_ context.Context, _, _ string) error {
	*m.count++
	return nil
}

func TestMiddlewareNamesResolvedOnce(t *testing.T) {
	count := 0
	name := fmt.Sprintf("counting-%d", time.Now().UnixNano())
	if err := middlewares.Register(name, &countingMiddleware{count: &count}); err != nil {
		t.Fatal(err)
	}
	defer middlewares.Unregister(name)

	a, prov := newTestAgent(t, WithMiddlewareNames(name))
	prov.responses = []mockResponse{{content: "one"}, {content: "two"}}

	if _, err := a.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("after first run, count = %d, want 1", count)
	}
	if _, err := a.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("after second run, count = %d, want 2 (middleware must not duplicate)", count)
	}
}

func TestRunToolNameResolution(t *testing.T) {
	tools.Register(tool.NewSpec("resolved-tool", "resolves by name", nil, func(context.Context, string) (string, error) {
		return "ok", nil
	}))
	defer tools.Unregister("resolved-tool")

	a, prov := newTestAgent(t, WithToolNames("resolved-tool"))
	prov.responses = []mockResponse{{content: "done"}}

	if _, ok := a.Tools.Get("resolved-tool"); ok {
		t.Fatal("tool must not be resolved until run")
	}
	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Tools.Get("resolved-tool"); !ok {
		t.Fatal("tool must be resolved from registry at run")
	}

	a2, _ := newTestAgent(t, WithToolNames("missing-tool"))
	if _, err := a2.Run(context.Background(), "hi"); !errors.Is(err, tool.ErrNotFound) {
		t.Fatalf("want ErrNotFound for missing tool name, got %v", err)
	}
}

func TestRunBasic(t *testing.T) {
	a, prov := newTestAgent(t, WithSystemPrompt("you are a test agent"))
	prov.responses = []mockResponse{{
		content: "hello world",
		usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}}

	res, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "hello world" {
		t.Fatalf("output = %q", res.Output)
	}
	if res.Iterations != 1 {
		t.Fatalf("iterations = %d, want 1", res.Iterations)
	}
	if res.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %+v", res.Usage)
	}
	if res.FinishReason != llm.FinishReasonStop {
		t.Fatalf("finish reason = %q", res.FinishReason)
	}
	if len(res.History) != 3 {
		t.Fatalf("history len = %d, want 3", len(res.History))
	}
	if res.History[0].Role != llm.RoleSystem {
		t.Fatalf("history[0] role = %q, want system", res.History[0].Role)
	}
	if res.History[1].Role != llm.RoleUser {
		t.Fatalf("history[1] role = %q, want user", res.History[1].Role)
	}
	if res.History[2].Role != llm.RoleAssistant {
		t.Fatalf("history[2] role = %q, want assistant", res.History[2].Role)
	}
}

func TestRunEmptyInput(t *testing.T) {
	a, _ := newTestAgent(t)
	if _, err := a.Run(context.Background(), "   "); !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("want ErrEmptyInput, got %v", err)
	}
}

func TestRunToolLoop(t *testing.T) {
	a, prov := newTestAgent(t, WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", `{"msg":"x"}`)}},
		{content: "done"},
	}

	res, err := a.Run(context.Background(), "echo x")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "done" {
		t.Fatalf("output = %q", res.Output)
	}
	if res.Iterations != 2 {
		t.Fatalf("iterations = %d, want 2", res.Iterations)
	}
	found := false
	for _, m := range res.History {
		if m.Role == llm.RoleTool && m.ToolCallID == "c1" && m.Content == "echo:{\"msg\":\"x\"}" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool message not found in history: %+v", res.History)
	}
}

func TestRunMissingToolFeedback(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "nope", "{}")}},
		{content: "final"},
	}

	res, err := a.Run(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "final" {
		t.Fatalf("output = %q", res.Output)
	}
	found := false
	for _, m := range res.History {
		if m.Role == llm.RoleTool && strings.Contains(fmt.Sprint(m.Content), "not registered") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing-tool feedback not found: %+v", res.History)
	}
}

func TestRunToolErrorFeedback(t *testing.T) {
	failing := tool.NewSpec("failing", "", nil, func(context.Context, string) (string, error) {
		return "", errors.New("boom")
	})
	a, prov := newTestAgent(t, WithTool(failing))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "failing", "{}")}},
		{content: "recovered"},
	}

	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "recovered" {
		t.Fatalf("output = %q", res.Output)
	}
	found := false
	for _, m := range res.History {
		if m.Role == llm.RoleTool && strings.Contains(fmt.Sprint(m.Content), "boom") {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool error feedback not found: %+v", res.History)
	}
}

func TestRunMaxIterations(t *testing.T) {
	a, prov := newTestAgent(t, WithMaxIterations(3))
	for i := 0; i < 3; i++ {
		prov.responses = append(prov.responses, mockResponse{
			calls: []llm.ToolCall{toolCall(fmt.Sprintf("c%d", i), "echo", "{}")},
		})
	}

	res, err := a.Run(context.Background(), "loop")
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("want ErrMaxIterations, got %v", err)
	}
	if res == nil || res.Iterations != 3 {
		t.Fatalf("iterations = %+v, want 3", res)
	}
}

func TestRunToolCallInsideMaxIteration(t *testing.T) {
	a, prov := newTestAgent(t, WithMaxIterations(3), WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "{}")}},
		{calls: []llm.ToolCall{toolCall("c2", "echo", "{}")}},
		{content: "ok"},
	}

	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Iterations != 3 {
		t.Fatalf("iterations = %d, want 3", res.Iterations)
	}
}

type mutatingMiddleware struct {
	middleware.BaseMiddleware
}

func (mutatingMiddleware) BeforeLLM(_ context.Context, req *llm.Request) error {
	req.Temperature = 0.7
	return nil
}

func TestRunMiddlewareRequestMutation(t *testing.T) {
	a, prov := newTestAgent(t,
		WithMiddleware(mutatingMiddleware{}),
		WithTemperature(0.2),
	)
	prov.responses = []mockResponse{{content: "ok"}}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if prov.lastReq == nil || prov.lastReq.Temperature != 0.7 {
		t.Fatalf("temperature not mutated, req = %+v", prov.lastReq)
	}
}

func TestRunStructuredOutput(t *testing.T) {
	schema := map[string]any{"type": "object"}
	a, prov := newTestAgent(t, WithStructuredOutput("my_verdict", schema))
	prov.responses = []mockResponse{{content: `{"ok": true}`}}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if prov.lastReq == nil || prov.lastReq.ResponseFormat == nil {
		t.Fatalf("expected structured output on the request, req = %+v", prov.lastReq)
	}
	if prov.lastReq.ResponseFormat.Type != llm.ResponseFormatJSONSchema {
		t.Fatalf("response format type = %q", prov.lastReq.ResponseFormat.Type)
	}
	if prov.lastReq.ResponseFormat.JSONSchema == nil || prov.lastReq.ResponseFormat.JSONSchema.Name != "my_verdict" {
		t.Fatalf("response format = %+v", prov.lastReq.ResponseFormat)
	}
}

type abortingMiddleware struct {
	middleware.BaseMiddleware
}

func (abortingMiddleware) BeforeTool(context.Context, llm.ToolCall) error {
	return errors.New("forbidden")
}

func TestRunMiddlewareToolAbort(t *testing.T) {
	executed := false
	blocked := tool.NewSpec("blocked", "", nil, func(context.Context, string) (string, error) {
		executed = true
		return "ran", nil
	})
	a, prov := newTestAgent(t,
		WithTool(blocked),
		WithMiddleware(abortingMiddleware{}),
	)
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "blocked", "{}")}},
		{content: "final"},
	}

	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("tool should not have executed")
	}
	found := false
	for _, m := range res.History {
		if m.Role == llm.RoleTool && strings.Contains(fmt.Sprint(m.Content), "aborted by middleware") {
			found = true
		}
	}
	if !found {
		t.Fatalf("abort feedback not found: %+v", res.History)
	}
}

func TestRunEvents(t *testing.T) {
	bus := event.New()
	var started *AgentStarted
	var finished *AgentFinished
	var toolEvt *AgentToolCall
	_ = bus.Subscribe(TopicAgentStarted, func(s AgentStarted) { started = &s })
	_ = bus.Subscribe(TopicAgentFinished, func(f AgentFinished) { finished = &f })
	_ = bus.Subscribe(TopicAgentToolCall, func(c AgentToolCall) { toolEvt = &c })

	a, prov := newTestAgent(t, WithBus(bus), WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "{}")}},
		{content: "done"},
	}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if started == nil || started.AgentName != "test-agent" {
		t.Fatalf("started = %+v", started)
	}
	if toolEvt == nil || toolEvt.Call.Function.Name != "echo" {
		t.Fatalf("toolCall event = %+v", toolEvt)
	}
	if finished == nil || finished.Output != "done" || finished.Iterations != 2 {
		t.Fatalf("finished = %+v", finished)
	}
}

func TestRunContextWindowTracking(t *testing.T) {
	a, prov := newTestAgent(t,
		WithModel(llm.Model{ID: "mock-model", Config: &llm.ModelConfig{MaxInputTokens: 40000, MaxTokens: 1000}}),
	)
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "{}")}, usage: llm.Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500}},
		{content: "second", usage: llm.Usage{PromptTokens: 2000, CompletionTokens: 1500, TotalTokens: 3500, Cost: 0.42}},
	}

	res, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.ID != "mock-model" {
		t.Fatalf("model = %+v", res.Model)
	}
	if res.Model.MaxInputTokens() != 40000 {
		t.Fatalf("context window = %d", res.Model.MaxInputTokens())
	}
	if res.Usage.TotalTokens != 3500 {
		t.Fatalf("total tokens = %d", res.Usage.TotalTokens)
	}
	if res.Usage.Cost != 0.42 {
		t.Fatalf("cost = %v", res.Usage.Cost)
	}
	if want := 8.75; res.Usage.ContextPercent(res.Model.MaxInputTokens()) != want {
		t.Fatalf("context percent = %v, want %v", res.Usage.ContextPercent(res.Model.MaxInputTokens()), want)
	}
	if got := llm.FormatContextUsage(res.Usage, res.Model.MaxInputTokens()); got != "3.5K (9%)" {
		t.Fatalf("formatted = %q", got)
	}
}

func TestRunContextWindowUnknownConfig(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "x", usage: llm.Usage{TotalTokens: 2000}}}
	res, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.MaxInputTokens() != 0 || res.Usage.ContextPercent(0) != 0 {
		t.Fatalf("window=%d pct=%v", res.Model.MaxInputTokens(), res.Usage.ContextPercent(0))
	}
}

func TestRunContextWindowInEvents(t *testing.T) {
	bus := event.New()
	var finished *AgentFinished
	_ = bus.Subscribe(TopicAgentFinished, func(f AgentFinished) { finished = &f })

	a, prov := newTestAgent(t,
		WithBus(bus),
		WithModel(llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 40000}}),
	)
	prov.responses = []mockResponse{{content: "done", usage: llm.Usage{TotalTokens: 10000}}}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if finished == nil {
		t.Fatal("no finished event")
	}
	if finished.Model.MaxInputTokens() != 40000 || finished.Usage.ContextPercent(finished.Model.MaxInputTokens()) != 25 {
		t.Fatalf("finished = %+v", finished)
	}
}

func TestRunStreamingContextWindow(t *testing.T) {
	a, prov := newTestAgent(t,
		WithModel(llm.Model{ID: "m", Config: &llm.ModelConfig{MaxInputTokens: 40000}}),
	)
	prov.streams = [][]llm.StreamChunk{{
		{Content: "Hel"},
		{Content: "lo"},
		{Usage: &llm.Usage{PromptTokens: 3000, CompletionTokens: 1000, TotalTokens: 4000}},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}

	var evts []StreamEvent
	if _, err := a.RunStream(context.Background(), "hi", func(e StreamEvent) error {
		evts = append(evts, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var done *StreamEvent
	for i := range evts {
		if evts[i].Type == StreamDone {
			done = &evts[i]
		}
	}
	if done == nil {
		t.Fatal("no done event")
	}
	if done.Model.MaxInputTokens() != 40000 || done.Usage.ContextPercent(done.Model.MaxInputTokens()) != 10 {
		t.Fatalf("done = %+v", done)
	}
}

func TestRunEventsCarryModel(t *testing.T) {
	bus := event.New()
	model := llm.Model{ID: "mock-model", Config: &llm.ModelConfig{MaxInputTokens: 40000}}

	var gotInput *AgentInput
	var gotMsg *AgentMessage
	var gotCall *AgentToolCall
	var gotResult *AgentToolResult
	var gotFinished *AgentFinished
	_ = bus.Subscribe(TopicAgentInput, func(e AgentInput) { gotInput = &e })
	_ = bus.Subscribe(TopicAgentMessage, func(e AgentMessage) { gotMsg = &e })
	_ = bus.Subscribe(TopicAgentToolCall, func(e AgentToolCall) { gotCall = &e })
	_ = bus.Subscribe(TopicAgentToolResult, func(e AgentToolResult) { gotResult = &e })
	_ = bus.Subscribe(TopicAgentFinished, func(e AgentFinished) { gotFinished = &e })

	a, prov := newTestAgent(t, WithBus(bus), WithModel(model), WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "{}")}},
		{content: "done"},
	}

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}

	for _, e := range []struct {
		name string
		got  llm.Model
	}{
		{"input", gotInput.Model},
		{"message", gotMsg.Model},
		{"tool call", gotCall.Model},
		{"tool result", gotResult.Model},
		{"finished", gotFinished.Model},
	} {
		if e.got.ID != model.ID || e.got.Config == nil {
			t.Fatalf("%s event model = %+v, want %+v", e.name, e.got, model)
		}
	}
}

func TestRunTokenAndErrorEventsCarryModel(t *testing.T) {
	model := llm.Model{ID: "mock-model", Config: &llm.ModelConfig{MaxInputTokens: 40000}}

	bus := event.New()
	var gotToken *AgentToken
	_ = bus.Subscribe(TopicAgentToken, func(e AgentToken) { gotToken = &e })

	a, prov := newTestAgent(t, WithBus(bus), WithModel(model))
	prov.streams = [][]llm.StreamChunk{{
		{Content: "hi"},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}
	if _, err := a.RunStream(context.Background(), "hi", func(StreamEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if gotToken == nil || gotToken.Model.ID != model.ID || gotToken.Model.Config == nil {
		t.Fatalf("token event model = %+v", gotToken)
	}

	bus2 := event.New()
	var errEvt *AgentError
	_ = bus2.Subscribe(TopicAgentError, func(e AgentError) { errEvt = &e })
	a2, prov2 := newTestAgent(t, WithBus(bus2), WithModel(model))
	prov2.responses = nil
	if _, err := a2.Run(context.Background(), "hi"); err == nil {
		t.Fatal("want provider error")
	}
	if errEvt == nil || errEvt.Model.ID != model.ID || errEvt.Model.Config == nil {
		t.Fatalf("error event model = %+v", errEvt)
	}
}

func TestRunErrorEvent(t *testing.T) {
	bus := event.New()
	var errEvt *AgentError
	_ = bus.Subscribe(TopicAgentError, func(e AgentError) { errEvt = &e })

	a, _ := newTestAgent(t, WithBus(bus), WithProvider(nil))
	_, err := a.Run(context.Background(), "hi")
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("want ErrNoProvider, got %v", err)
	}
	if errEvt == nil || !errors.Is(errEvt.Err, ErrNoProvider) {
		t.Fatalf("error event = %+v", errEvt)
	}
}

func TestRunStreamingTokens(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.streams = [][]llm.StreamChunk{{
		{Content: "Hel"},
		{Content: "lo"},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}

	var evts []StreamEvent
	res, err := a.RunStream(context.Background(), "hi", func(e StreamEvent) error {
		evts = append(evts, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "Hello" {
		t.Fatalf("output = %q", res.Output)
	}
	if res.FinishReason != llm.FinishReasonStop {
		t.Fatalf("finish reason = %q", res.FinishReason)
	}
	var tokens, dones int
	for _, e := range evts {
		switch e.Type {
		case StreamToken:
			tokens++
		case StreamDone:
			dones++
		}
	}
	if tokens != 2 || dones != 1 {
		t.Fatalf("tokens=%d dones=%d, events=%+v", tokens, dones, evts)
	}
}

func TestRunStreamingToolCalls(t *testing.T) {
	a, prov := newTestAgent(t, WithTool(echoTool()))
	prov.streams = [][]llm.StreamChunk{
		{
			{ToolCalls: []llm.ToolCall{{
				Index: 0, ID: "c1", Type: "function",
				Function: llm.Function{Name: "echo", Arguments: `{"msg":"`},
			}}},
			{ToolCalls: []llm.ToolCall{{Index: 0, Function: llm.Function{Arguments: `hi"}`}}}},
			{FinishReason: llm.FinishReasonToolCalls, IsDone: true},
		},
		{
			{Content: "ok"},
			{FinishReason: llm.FinishReasonStop, IsDone: true},
		},
	}

	var evts []StreamEvent
	res, err := a.RunStream(context.Background(), "hi", func(e StreamEvent) error {
		evts = append(evts, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "ok" {
		t.Fatalf("output = %q", res.Output)
	}
	found := false
	for _, m := range res.History {
		if m.Role == llm.RoleTool && m.ToolCallID == "c1" && m.Content == `echo:{"msg":"hi"}` {
			found = true
		}
	}
	if !found {
		t.Fatalf("merged tool call not found: %+v", res.History)
	}
	var toolCalls, toolResults int
	for _, e := range evts {
		switch e.Type {
		case StreamToolCall:
			toolCalls++
		case StreamToolResult:
			toolResults++
		}
	}
	if toolCalls != 1 || toolResults != 1 {
		t.Fatalf("toolCalls=%d toolResults=%d", toolCalls, toolResults)
	}
}

func TestAgentOptions(t *testing.T) {
	log := logger.New(logger.LevelDebug, discardHandler{})
	bus := event.New()
	prov := &scriptedProvider{name: "mock"}
	prov.responses = []mockResponse{{content: "ok"}}

	fileTool := tool.NewSpec("file-read", "read files", nil, func(context.Context, string) (string, error) {
		return "file", nil
	})

	a := New("",
		WithDescription("desc"),
		WithModel(llm.Model{ID: "m1"}),
		WithProvider(prov),
		WithSystemPrompt("sys"),
		WithTemperature(0.5),
		WithTopP(0.9),
		WithMaxTokens(2048),
		WithMaxIterations(0),
		WithTool(echoTool()),
		WithTools(fileTool),
		WithBus(bus),
		WithLogger(log),
	)

	if a.Name != "agent" {
		t.Fatalf("name = %q, want default", a.Name)
	}
	if a.Description != "desc" || a.Model.ID != "m1" || a.Provider != prov {
		t.Fatalf("basic fields not set: %+v", a)
	}
	if a.SystemPrompt != "sys" || a.Temperature != 0.5 || a.TopP != 0.9 || a.MaxTokens != 2048 {
		t.Fatalf("llm fields not set: %+v", a)
	}
	if a.MaxIterations != DefaultMaxIterations {
		t.Fatalf("max iterations = %d, want default %d", a.MaxIterations, DefaultMaxIterations)
	}
	if _, ok := a.Tools.Get("echo"); !ok {
		t.Fatal("echo tool missing")
	}
	if _, ok := a.Tools.Get("file-read"); !ok {
		t.Fatal("file-read tool missing")
	}
	if a.log == nil || a.Bus == nil {
		t.Fatal("bus/logger options not applied")
	}

	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if prov.lastReq == nil {
		t.Fatal("no request captured")
	}
	if prov.lastReq.TopP != 0.9 || prov.lastReq.MaxTokens != 2048 {
		t.Fatalf("request params not applied: %+v", prov.lastReq)
	}
}

func TestRunReasoning(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{
		content:   "answer",
		reasoning: "thinking...",
	}}

	res, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.History[1].Role != llm.RoleAssistant || res.History[1].Reasoning != "thinking..." {
		t.Fatalf("reasoning not carried: %+v", res.History)
	}
}

func TestEmptyAssistantMessage(t *testing.T) {
	m := llm.AssistantMessage("")
	m.Reasoning = "thinking hard"
	if !emptyAssistantMessage(m) {
		t.Fatal("reasoning-only message must be considered empty")
	}

	if emptyAssistantMessage(llm.AssistantMessage("answer")) {
		t.Fatal("message with content must not be considered empty")
	}

	m2 := llm.AssistantMessage("")
	m2.ToolCalls = []llm.ToolCall{{ID: "t1", Function: llm.Function{Name: "read"}}}
	if emptyAssistantMessage(m2) {
		t.Fatal("tool-call message must not be considered empty")
	}

	if emptyAssistantMessage(llm.UserMessage("hi")) {
		t.Fatal("user message must not be considered empty")
	}
	if emptyAssistantMessage(llm.ToolMessage("out", "t1")) {
		t.Fatal("tool message must not be considered empty")
	}
}

func TestRunProviderError(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = nil

	_, err := a.Run(context.Background(), "hi")
	if err == nil || err.Error() != "mock: no more responses" {
		t.Fatalf("want provider error, got %v", err)
	}
}

type beforeRunFail struct {
	middleware.BaseMiddleware
}

func (beforeRunFail) BeforeRun(context.Context, string, string) error {
	return errors.New("no run")
}

func TestRunBeforeRunMiddlewareError(t *testing.T) {
	a, _ := newTestAgent(t, WithMiddleware(beforeRunFail{}))
	_, err := a.Run(context.Background(), "hi")
	if err == nil || err.Error() != "no run" {
		t.Fatalf("want middleware error, got %v", err)
	}
}

type afterRunFail struct {
	middleware.BaseMiddleware
}

func (afterRunFail) AfterRun(context.Context, string, *middleware.Result, error) error {
	return errors.New("after fail")
}

func TestRunAfterRunMiddlewareError(t *testing.T) {
	bus := event.New()
	var finished *AgentFinished
	_ = bus.Subscribe(TopicAgentFinished, func(f AgentFinished) { finished = &f })

	a, prov := newTestAgent(t, WithBus(bus), WithMiddleware(afterRunFail{}))
	prov.responses = []mockResponse{{content: "ok"}}

	_, err := a.Run(context.Background(), "hi")
	if err == nil || err.Error() != "after fail" {
		t.Fatalf("want AfterRun error, got %v", err)
	}
	if finished != nil {
		t.Fatal("agent.finished must not fire when AfterRun fails")
	}
}

type beforeLLMFail struct {
	middleware.BaseMiddleware
}

func (beforeLLMFail) BeforeLLM(context.Context, *llm.Request) error {
	return errors.New("llm blocked")
}

func TestRunBeforeLLMMiddlewareError(t *testing.T) {
	a, _ := newTestAgent(t, WithMiddleware(beforeLLMFail{}))
	_, err := a.Run(context.Background(), "hi")
	if err == nil || err.Error() != "llm blocked" {
		t.Fatalf("want BeforeLLM error, got %v", err)
	}
}

func TestRunStreamingReasoningAndUsage(t *testing.T) {
	bus := event.New()
	var tokenEvts []AgentToken
	_ = bus.Subscribe(TopicAgentToken, func(e AgentToken) { tokenEvts = append(tokenEvts, e) })

	a, prov := newTestAgent(t, WithBus(bus))
	prov.streams = [][]llm.StreamChunk{{
		{Reasoning: "think"},
		{Content: "a"},
		{Usage: &llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}

	var evts []StreamEvent
	res, err := a.RunStream(context.Background(), "hi", func(e StreamEvent) error {
		evts = append(evts, e)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "a" || res.Usage.TotalTokens != 15 {
		t.Fatalf("output=%q usage=%+v", res.Output, res.Usage)
	}
	if res.History[1].Reasoning != "think" {
		t.Fatalf("reasoning not accumulated: %+v", res.History[1])
	}
	gotReasoning := false
	gotToken := false
	for _, e := range evts {
		if e.Type == StreamReasoning && e.Reasoning == "think" {
			gotReasoning = true
		}
		if e.Type == StreamToken && e.Content == "a" {
			gotToken = true
		}
	}
	if !gotReasoning || !gotToken {
		t.Fatalf("missing stream events: %+v", evts)
	}
	gotReasoningEvt := false
	for _, e := range tokenEvts {
		if e.Reasoning == "think" {
			gotReasoningEvt = true
		}
	}
	if !gotReasoningEvt {
		t.Fatalf("token events = %+v", tokenEvts)
	}
}

func TestRunStreamingHandlerError(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.streams = [][]llm.StreamChunk{{
		{Content: "x"},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}

	_, err := a.RunStream(context.Background(), "hi", func(StreamEvent) error {
		return errors.New("handler stopped")
	})
	if err == nil || err.Error() != "handler stopped" {
		t.Fatalf("want handler error, got %v", err)
	}
}

type afterLLMFail struct {
	middleware.BaseMiddleware
}

func (afterLLMFail) AfterLLM(context.Context, *llm.Request, *llm.Response) error {
	return errors.New("after llm fail")
}

func TestRunStreamingAfterLLMMiddlewareError(t *testing.T) {
	a, prov := newTestAgent(t, WithMiddleware(afterLLMFail{}))
	prov.streams = [][]llm.StreamChunk{{
		{Content: "x"},
		{FinishReason: llm.FinishReasonStop, IsDone: true},
	}}

	_, err := a.RunStream(context.Background(), "hi", func(StreamEvent) error { return nil })
	if err == nil || err.Error() != "after llm fail" {
		t.Fatalf("want AfterLLM error, got %v", err)
	}
}

func TestAgent_ResumeSeedsHistory(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "follow-up answer"}}

	history := []llm.Message{
		llm.SystemMessage("system prompt"),
		llm.UserMessage("first task"),
		llm.AssistantMessage("first answer"),
	}
	res, err := a.Resume(context.Background(), "follow-up", history)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Output != "follow-up answer" {
		t.Errorf("output = %q, want follow-up answer", res.Output)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	msgs := prov.lastReq.Messages
	if len(msgs) != 4 {
		t.Fatalf("request messages = %d, want 4 (history + new input)", len(msgs))
	}
	if msgs[0].Role != llm.RoleSystem || msgs[0].Content != "system prompt" {
		t.Errorf("msg[0] = %+v, want system prompt", msgs[0])
	}
	if msgs[1].Role != llm.RoleUser || msgs[1].Content != "first task" {
		t.Errorf("msg[1] = %+v, want first task", msgs[1])
	}
	if msgs[2].Role != llm.RoleAssistant || msgs[2].Content != "first answer" {
		t.Errorf("msg[2] = %+v, want first answer", msgs[2])
	}
	if msgs[3].Role != llm.RoleUser || msgs[3].Content != "follow-up" {
		t.Errorf("msg[3] = %+v, want follow-up input", msgs[3])
	}
}

func TestAgent_ResumeDoesNotDuplicateSystemPrompt(t *testing.T) {
	a, prov := newTestAgent(t, WithSystemPrompt("built-in prompt"))
	prov.responses = []mockResponse{{content: "ok"}}

	history := []llm.Message{llm.SystemMessage("built-in prompt"), llm.UserMessage("task")}
	if _, err := a.Resume(context.Background(), "more", history); err != nil {
		t.Fatalf("resume: %v", err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	msgs := prov.lastReq.Messages
	count := 0
	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			count++
		}
	}
	if count != 1 {
		t.Errorf("system messages = %d, want 1 (history passed verbatim)", count)
	}
}

func TestContinueWithoutNotifications(t *testing.T) {
	a, _ := newTestAgent(t)
	res, err := a.Continue(context.Background(), nil)
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if res != nil {
		t.Fatalf("result = %+v, want nil when nothing to process", res)
	}
}

func TestContinueProcessesPendingNotification(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "handled"}}
	a.QueueNotification(reminder.New("subagent", "background work done", "agent", "bg-1"))

	res, err := a.Continue(context.Background(), []llm.Message{llm.UserMessage("prev")})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if res == nil || res.Output != "handled" {
		t.Fatalf("result = %+v, want handled", res)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	msgs := prov.lastReq.Messages
	if len(msgs) != 2 {
		t.Fatalf("request messages = %d, want 2 (history + reminder)", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser {
		t.Fatalf("msgs[0] = %+v, want history verbatim", msgs[0])
	}
	if msgs[1].Role != llm.RoleSystem || !strings.Contains(fmt.Sprint(msgs[1].Content), "background work done") {
		t.Fatalf("reminder not injected: %+v", msgs)
	}
}

type recordingProvider struct {
	llm.Provider
	reqs []*llm.Request
}

func (p *recordingProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	reqCopy := *req
	reqCopy.Messages = append([]llm.Message(nil), req.Messages...)
	p.reqs = append(p.reqs, &reqCopy)
	return p.Provider.Chat(ctx, req)
}

func TestRunInjectsAttachedReminderEveryIteration(t *testing.T) {
	base, prov := newTestAgent(t, WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "hi")}},
		{content: "second"},
	}
	rec := &recordingProvider{Provider: base.Provider}
	base.Provider = rec
	base.AttachReminder(reminder.New("plan_mode", "PLAN MODE ACTIVE"))

	res, err := base.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "second" {
		t.Fatalf("output = %q, want second", res.Output)
	}
	if len(rec.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(rec.reqs))
	}
	wantRoles := []llm.Role{llm.RoleUser, llm.RoleTool}
	for i, req := range rec.reqs {
		last := req.Messages[len(req.Messages)-1]
		if last.Role != wantRoles[i] {
			t.Errorf("req[%d] last message role = %q, want %q", i, last.Role, wantRoles[i])
		}
		if !strings.Contains(fmt.Sprint(last.Content), "PLAN MODE ACTIVE") {
			t.Errorf("req[%d] last message = %+v, want reminder appended to its content", i, last)
		}
		for _, m := range req.Messages {
			if m.Role == llm.RoleSystem && strings.Contains(fmt.Sprint(m.Content), "PLAN MODE ACTIVE") {
				t.Errorf("req[%d] reminder must not be a separate system message: %+v", i, m)
			}
		}
	}
	if strings.Contains(fmt.Sprint(res.History), "PLAN MODE ACTIVE") {
		t.Fatalf("history must not contain the ephemeral reminder")
	}
}

func TestDetachReminderStopsInjection(t *testing.T) {
	base, prov := newTestAgent(t)
	base.AttachReminder(reminder.New("plan_mode", "PLAN MODE ACTIVE"))

	prov.responses = []mockResponse{{content: "one"}}
	if _, err := base.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	base.DetachReminder()

	prov.responses = []mockResponse{{content: "two"}}
	if _, err := base.Run(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()
	for _, m := range prov.lastReq.Messages {
		if strings.Contains(fmt.Sprint(m.Content), "PLAN MODE ACTIVE") {
			t.Fatalf("reminder still injected after detach: %+v", prov.lastReq.Messages)
		}
	}
}

type cancelStreamProvider struct {
	*scriptedProvider
	cancel context.CancelFunc
}

func (p *cancelStreamProvider) ChatStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error {
	p.mu.Lock()
	chunks := p.streams[0]
	p.streams = p.streams[1:]
	p.mu.Unlock()
	for _, c := range chunks {
		if err := handler(c); err != nil {
			return err
		}
	}
	p.cancel()
	return context.Canceled
}

func TestRunStreamReturnsPartialHistoryOnCancel(t *testing.T) {
	base, prov := newTestAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	rec := &cancelStreamProvider{scriptedProvider: prov, cancel: cancel}
	rec.streams = [][]llm.StreamChunk{
		{{Content: "partial "}, {Content: "tokens"}},
	}
	base.Provider = rec

	res, err := base.RunStream(ctx, "hi", func(StreamEvent) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if res == nil {
		t.Fatal("result = nil, want partial result with history")
	}
	if len(res.History) != 2 {
		t.Fatalf("history = %d messages, want 2 (user + partial assistant)", len(res.History))
	}
	if res.History[0].Role != llm.RoleUser || llm.MessageText(res.History[0]) != "hi" {
		t.Errorf("history[0] = %+v, want user input", res.History[0])
	}
	if res.History[1].Role != llm.RoleAssistant || llm.MessageText(res.History[1]) != "partial tokens" {
		t.Errorf("history[1] = %+v, want partial assistant content", res.History[1])
	}
}

type erroringProvider struct {
	llm.Provider
	calls int
	err   error
}

func (p *erroringProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	p.calls++
	if p.calls >= 2 {
		return nil, p.err
	}
	return p.Provider.Chat(ctx, req)
}

func TestRunReturnsPartialHistoryOnError(t *testing.T) {
	base, prov := newTestAgent(t, WithTool(echoTool()))
	prov.responses = []mockResponse{
		{calls: []llm.ToolCall{toolCall("c1", "echo", "hi")}},
	}
	ep := &erroringProvider{Provider: base.Provider, err: errors.New("boom")}
	base.Provider = ep

	res, err := base.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("want error from second iteration")
	}
	if res == nil {
		t.Fatal("result = nil, want partial result with history")
	}
	if len(res.History) != 3 {
		t.Fatalf("history = %d messages, want 3 (user + assistant call + tool result)", len(res.History))
	}
	last := res.History[2]
	if last.Role != llm.RoleTool || llm.MessageText(last) != "echo:hi" {
		t.Errorf("history[2] = %+v, want tool result", last)
	}
}

func TestRunResolvesInputToolCalls(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}
	a.Tools.Register(tool.NewSpec("loadskill", "loads a skill", nil, func(_ context.Context, args string) (string, error) {
		return "skill-content:" + args, nil
	}))

	OnMessageInput(func(in MessageInput) MessageInput {
		if !strings.Contains(in.Text, "resolve-input-calls-test") {
			return in
		}
		in.Text = strings.ReplaceAll(in.Text, "/review", "")
		in.Calls = append(in.Calls, llm.ToolCall{ID: "c-skill", Type: "function", Function: llm.Function{Name: "loadskill", Arguments: `{"name":"review"}`}})
		return in
	})

	res, err := a.Run(context.Background(), "resolve-input-calls-test /review")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "done" {
		t.Fatalf("output = %q, want done", res.Output)
	}
	if len(res.History) != 4 {
		t.Fatalf("history = %d messages, want 4 (user, tool call, tool result, answer)", len(res.History))
	}
	if res.History[0].Role != llm.RoleUser || strings.Contains(llm.MessageText(res.History[0]), "/review") {
		t.Fatalf("history[0] = %+v, want stripped user message", res.History[0])
	}
	msg := res.History[1]
	if msg.Role != llm.RoleAssistant || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "loadskill" {
		t.Fatalf("history[1] = %+v, want synthetic loadskill call", msg)
	}
	toolMsg := res.History[2]
	if toolMsg.Role != llm.RoleTool || !strings.Contains(llm.MessageText(toolMsg), "skill-content") {
		t.Fatalf("history[2] = %+v, want tool result", toolMsg)
	}
}

func TestRunSkipsEmptyUserMessageAfterExpansion(t *testing.T) {
	a, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "done"}}
	a.Tools.Register(tool.NewSpec("loadskill", "loads a skill", nil, func(_ context.Context, args string) (string, error) {
		return "skill-content:" + args, nil
	}))

	OnMessageInput(func(in MessageInput) MessageInput {
		if !strings.Contains(in.Text, "only-skill-input-test") {
			return in
		}
		in.Text = ""
		in.Calls = append(in.Calls, llm.ToolCall{ID: "c-only", Type: "function", Function: llm.Function{Name: "loadskill", Arguments: `{"name":"x"}`}})
		return in
	})

	res, err := a.Run(context.Background(), "only-skill-input-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.History) != 3 {
		t.Fatalf("history = %d messages, want 3 (tool call, tool result, answer)", len(res.History))
	}
	if res.History[0].Role != llm.RoleAssistant || len(res.History[0].ToolCalls) != 1 {
		t.Fatalf("history[0] = %+v, want synthetic tool call without user message", res.History[0])
	}
	if res.History[1].Role != llm.RoleTool {
		t.Fatalf("history[1] = %+v, want tool result", res.History[1])
	}
}

type notifyProvider struct {
	llm.Provider
	agent *Agent
	once  sync.Once
}

func (p *notifyProvider) Chat(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	resp, err := p.Provider.Chat(ctx, req)
	p.once.Do(func() { p.agent.QueueNotification(reminder.New("subagent", "done")) })
	return resp, err
}

func TestRunContinuesWhileNotificationsPending(t *testing.T) {
	base, prov := newTestAgent(t)
	prov.responses = []mockResponse{{content: "first"}, {content: "second"}}
	a := &notifyProvider{Provider: base.Provider, agent: base}
	base.Provider = a

	res, err := base.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "second" {
		t.Fatalf("output = %q, want second (loop must continue to drain reminder)", res.Output)
	}
	if res.Iterations != 2 {
		t.Fatalf("iterations = %d, want 2", res.Iterations)
	}
}

func TestAgentClone(t *testing.T) {
	bus := event.New()
	prov := &scriptedProvider{name: "mock"}
	prov.responses = []mockResponse{{content: "ok"}}

	a := New("original",
		WithDescription("desc"),
		WithModel(llm.Model{ID: "m1"}),
		WithProvider(prov),
		WithSystemPrompt("sys"),
		WithTemperature(0.5),
		WithMaxTokens(2048),
		WithTool(echoTool()),
		WithBus(bus),
	)
	clone := a.Clone("copy")
	if clone.ID == a.ID {
		t.Fatal("clone must have a new identity")
	}
	if clone.Name != "copy" {
		t.Fatalf("clone name = %q, want copy", clone.Name)
	}
	if clone.Provider != a.Provider || clone.Model.ID != a.Model.ID || clone.SystemPrompt != "sys" {
		t.Fatalf("clone fields not copied: %+v", clone)
	}
	if clone.Temperature != 0.5 || clone.MaxTokens != 2048 || clone.Bus != bus {
		t.Fatalf("clone params not copied: %+v", clone)
	}
	if _, ok := clone.Tools.Get("echo"); !ok {
		t.Fatal("clone tools missing")
	}
}
