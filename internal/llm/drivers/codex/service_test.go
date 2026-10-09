package codex

import (
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestBuildRequestMapping(t *testing.T) {
	s := NewService("codex", ServiceConfig{})
	req := &llm.Request{
		Model:           "gpt-5.3-codex",
		ReasoningEffort: "medium",
		Messages: []llm.Message{
			llm.SystemMessage("you are helpful"),
			llm.UserMessage("hello"),
			{Role: llm.RoleAssistant, Content: "working", ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.Function{Name: "bash", Arguments: `{"cmd":"ls"}`}}}},
			llm.ToolMessage("file1", "call-1"),
		},
		Tools: []llm.Tool{{Type: "function", Function: llm.ToolFunction{Name: "bash", Description: "run", Parameters: map[string]any{"type": "object"}}}},
	}

	cr := s.buildRequest(req)
	if cr.Instructions != "you are helpful" {
		t.Fatalf("instructions = %q", cr.Instructions)
	}
	if !cr.Stream || cr.Store {
		t.Fatalf("stream=%v store=%v", cr.Stream, cr.Store)
	}
	if cr.Reasoning == nil || cr.Reasoning.Effort != "medium" {
		t.Fatalf("reasoning = %+v", cr.Reasoning)
	}
	if len(cr.Input) != 4 {
		t.Fatalf("input len = %d, want 4 (%+v)", len(cr.Input), cr.Input)
	}
	if cr.Input[0]["type"] != "message" || cr.Input[0]["role"] != "user" {
		t.Fatalf("user item = %+v", cr.Input[0])
	}
	if cr.Input[1]["type"] != "message" || cr.Input[1]["role"] != "assistant" {
		t.Fatalf("assistant item = %+v", cr.Input[1])
	}
	if cr.Input[2]["type"] != "function_call" || cr.Input[2]["name"] != "bash" {
		t.Fatalf("function_call item = %+v", cr.Input[2])
	}
	if cr.Input[3]["type"] != "function_call_output" || cr.Input[3]["call_id"] != "call-1" {
		t.Fatalf("function_call_output item = %+v", cr.Input[3])
	}
	if len(cr.Tools) != 1 || cr.Tools[0]["name"] != "bash" {
		t.Fatalf("tools = %+v", cr.Tools)
	}
}

func TestParseEventTextDelta(t *testing.T) {
	s := NewService("codex", ServiceConfig{})
	acc := newAccumulator()

	chunk, emit, err := s.parseEvent([]byte(`{"type":"response.output_text.delta","delta":"Hel"}`), acc)
	if err != nil || !emit || chunk.Content != "Hel" {
		t.Fatalf("chunk=%+v emit=%v err=%v", chunk, emit, err)
	}
	_, emit, _ = s.parseEvent([]byte(`{"type":"response.content_part.delta","delta":{"type":"output_text","text":"lo"}}`), acc)
	if !emit {
		t.Fatal("content_part.delta should emit")
	}
}

func TestParseEventFunctionCall(t *testing.T) {
	s := NewService("codex", ServiceConfig{})
	acc := newAccumulator()

	chunk, emit, err := s.parseEvent([]byte(`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call-1","name":"bash","arguments":"{\"cmd\":\"ls\"}"}}`), acc)
	if err != nil || !emit {
		t.Fatalf("emit=%v err=%v", emit, err)
	}
	if len(chunk.ToolCalls) != 1 || chunk.ToolCalls[0].ID != "call-1" || chunk.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("tool calls = %+v", chunk.ToolCalls)
	}
}

func TestParseEventCompleted(t *testing.T) {
	s := NewService("codex", ServiceConfig{})
	acc := newAccumulator()

	chunk, emit, err := s.parseEvent([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`), acc)
	if err != nil || !emit || !chunk.IsDone {
		t.Fatalf("chunk=%+v emit=%v err=%v", chunk, emit, err)
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %+v", chunk.Usage)
	}
}

func TestParseEventFailed(t *testing.T) {
	s := NewService("codex", ServiceConfig{})
	acc := newAccumulator()
	_, _, err := s.parseEvent([]byte(`{"type":"response.failed","error":{"code":"rate_limit","message":"slow down"}}`), acc)
	if err == nil {
		t.Fatal("expected an error")
	}
}
