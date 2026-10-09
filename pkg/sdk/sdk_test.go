package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Chat(_ context.Context, req *Request) (*Response, error) {
	return &Response{
		Model: req.Model,
		Choices: []Choice{{
			Message:      &Message{Role: RoleAssistant, Content: "mock answer for: " + req.Messages[len(req.Messages)-1].Content.(string)},
			FinishReason: &[]FinishReason{FinishReasonStop}[0],
		}},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (m *mockProvider) ChatStream(_ context.Context, req *Request, handler StreamHandler) error {
	for _, part := range strings.Split("streamed mock output", " ") {
		if err := handler(StreamChunk{Content: part + " "}); err != nil {
			return err
		}
	}
	return handler(StreamChunk{FinishReason: FinishReasonStop, Usage: &Usage{TotalTokens: 3}})
}

func (m *mockProvider) ListModels(_ context.Context) ([]Model, error) {
	return []Model{
		{ID: "mock-model", Name: "mock-model"},
		{ID: "mock-model-2", Name: "mock-model-2"},
	}, nil
}

func registerMock(name string) {
	llm.RegisterProvider(name, func(cfg config.LLMConfig) (Provider, error) {
		return &mockProvider{name: name}, nil
	})
}

func testOptions(t *testing.T, provider string) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Config:     config.DefaultConfig(),
		Workspace:  dir,
		SessionDir: dir + "/sessions",
		Providers: []LLMConfig{
			{Provider: provider, APIKey: "test-key"},
		},
	}
}

func TestEngineOpenClose(t *testing.T) {
	registerMock("sdk-open-close")
	opts := testOptions(t, "sdk-open-close")

	eng, err := Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if eng.Config() == nil {
		t.Fatal("Config() is nil")
	}

	if _, err := Open(context.Background(), opts); !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("second Open err = %v, want ErrAlreadyOpen", err)
	}

	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close idempotent: %v", err)
	}

	eng2, err := Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if err := eng2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestChat(t *testing.T) {
	registerMock("sdk-chat")
	eng, err := Open(context.Background(), testOptions(t, "sdk-chat"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer eng.Close()

	resp, err := eng.Chat(context.Background(), ChatRequest{
		Input:    "hello",
		Provider: "sdk-chat",
		Model:    "mock-model",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !strings.Contains(resp.Output, "streamed mock") {
		t.Fatalf("Output = %q, want streamed mock output", resp.Output)
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("usage not populated")
	}
	if resp.FinishReason != FinishReasonStop {
		t.Fatalf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if resp.SessionID == "" {
		t.Fatal("expected a persisted session id")
	}
	if len(resp.History) == 0 {
		t.Fatal("expected history")
	}
}

func TestChatStream(t *testing.T) {
	registerMock("sdk-chat-stream")
	eng, err := Open(context.Background(), testOptions(t, "sdk-chat-stream"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer eng.Close()

	var got []ChatEventType
	var content strings.Builder
	resp, err := eng.ChatStream(context.Background(), ChatRequest{
		Input:    "hello",
		Provider: "sdk-chat-stream",
		Model:    "mock-model",
	}, func(ev ChatEvent) error {
		got = append(got, ev.Type)
		content.WriteString(ev.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected stream events")
	}
	if resp.Output == "" {
		t.Fatal("expected output")
	}
}

func TestChatRequiresProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers = nil
	eng, err := Open(context.Background(), Options{
		Config:     cfg,
		Workspace:  t.TempDir(),
		SessionDir: t.TempDir() + "/s",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer eng.Close()

	_, err = eng.Chat(context.Background(), ChatRequest{Input: "hello"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Chat err = %v, want ErrNotConfigured", err)
	}
}

func TestChatAfterClose(t *testing.T) {
	registerMock("sdk-chat-closed")
	eng, err := Open(context.Background(), testOptions(t, "sdk-chat-closed"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := eng.Chat(context.Background(), ChatRequest{Input: "hello"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Chat after close err = %v, want ErrClosed", err)
	}
}

func TestMapStreamEventCompaction(t *testing.T) {
	ev := mapStreamEvent(agent.StreamEvent{
		Type:      agent.StreamCompaction,
		AgentID:   "a1",
		AgentName: "orchestrator",
		Strategy:  "sliding-window",
		Messages:  30,
		Tokens:    8000,
	})
	if ev.Type != EventCompaction {
		t.Fatalf("type = %q, want %q", ev.Type, EventCompaction)
	}
	if ev.Strategy != "sliding-window" || ev.Messages != 30 || ev.Tokens != 8000 {
		t.Fatalf("event = %+v", ev)
	}
	if ev.AgentID != "a1" || ev.AgentName != "orchestrator" {
		t.Fatalf("event = %+v", ev)
	}
}
