package permission

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
)

func TestJudgeBuiltLazilyAndCached(t *testing.T) {
	mgr, bus := newJudgeTestManager(t)
	syncJudgeProvider(t, mgr, bus, "judge-lazy", []llm.Model{{ID: "m1"}})

	m := &Middleware{llm: mgr}
	if m.judgeAgent != nil {
		t.Fatal("judge must not be resolved at construction")
	}
	j1 := m.judge()
	j2 := m.judge()
	if j1 == nil || j1 != j2 {
		t.Fatal("judge must be built lazily and cached")
	}
	if j1.Model.ID != "m1" {
		t.Fatalf("judge model = %q, want m1", j1.Model.ID)
	}
}

func TestJudgeUnavailableWithoutManager(t *testing.T) {
	m := New(Deps{})
	if m.judge() != nil {
		t.Fatal("expected no judge agent without an LLM manager")
	}
}

func TestJudgeResolutionFailureCached(t *testing.T) {
	mgr, _ := newJudgeTestManager(t)
	m := &Middleware{llm: mgr}
	if m.judge() != nil {
		t.Fatal("expected nil judge agent on resolution failure")
	}
	if m.judge() != nil {
		t.Fatal("resolution failure must be cached")
	}
}

func TestJudgeDeniesOnResolutionFailure(t *testing.T) {
	mgr, _ := newJudgeTestManager(t)
	m := New(Deps{
		Config: &config.PermissionConfig{Default: "judge"},
		LLM:    mgr,
	})
	a, _ := testAgent(t)
	ctx := agent.WithAgent(context.Background(), a)

	var runs int
	_, err := m.InvokeTool(ctx, llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath": "a.txt"}`}}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		runs++
		return "content", nil
	})
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %v", err)
	}
	if denied.Reason != "judge provider unavailable" {
		t.Fatalf("reason = %q", denied.Reason)
	}
	if runs != 0 {
		t.Fatalf("runs = %d, want 0", runs)
	}
}

func TestGenerateJudgeSystemPrompt(t *testing.T) {
	sys, err := generateJudgeSystemPrompt()
	if err != nil {
		t.Fatalf("generate system prompt: %v", err)
	}
	for _, want := range []string{"permission judge", "allow", "reason"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q:\n%s", want, sys)
		}
	}
}

func TestBuildJudgePrompt(t *testing.T) {
	call := llm.ToolCall{Function: llm.Function{Name: "read", Arguments: `{"filePath":"/etc/passwd"}`}}
	history := []llm.Message{
		llm.UserMessage("refactor the auth service"),
		llm.AssistantMessage("Let me look at the auth code"),
		llm.ToolMessage("Error: permission denied", "t1"),
		llm.UserMessage("also check the config"),
		llm.AssistantMessage("I'll read the config file"),
	}
	input, err := buildJudgePrompt(call, errors.New("path escapes the workspace root"), history)
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	for _, want := range []string{"read", "filePath", "/etc/passwd", "path escapes the workspace root", "refactor the auth service", "I'll read the config file", "permission denied"} {
		if !strings.Contains(input, want) {
			t.Errorf("judge prompt missing %q:\n%s", want, input)
		}
	}
}

func TestFormatHistory(t *testing.T) {
	thinking := llm.AssistantMessage("")
	thinking.Reasoning = "thinking hard"
	msgs := []llm.Message{
		llm.SystemMessage("be good"),
		llm.UserMessage("first"),
		llm.AssistantMessage("second"),
		llm.UserMessage("third"),
		thinking,
		llm.AssistantMessage("fourth"),
		llm.UserMessage("fifth"),
		llm.AssistantMessage("sixth"),
		llm.UserMessage("seventh"),
	}
	out := formatHistory(msgs)
	if !strings.Contains(out, "assistant thinking: thinking hard") {
		t.Errorf("judge context missing thinking:\n%s", out)
	}
	for _, want := range []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"} {
		if !strings.Contains(out, want) {
			t.Errorf("judge context must include every message, missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "be good") {
		t.Errorf("judge context must not contain system messages:\n%s", out)
	}
}

func TestFormatHistoryContentAndThinking(t *testing.T) {
	m := llm.AssistantMessage("the answer")
	m.Reasoning = "first I'll check the file"
	out := formatHistory([]llm.Message{
		llm.UserMessage("how are u"),
		m,
	})
	if !strings.Contains(out, "assistant: the answer") || !strings.Contains(out, "assistant thinking: first I'll check the file") {
		t.Fatalf("expected content and thinking:\n%s", out)
	}
}

func TestFormatHistoryToolCalls(t *testing.T) {
	callMsg := llm.AssistantMessage("")
	callMsg.ToolCalls = []llm.ToolCall{{ID: "t1", Function: llm.Function{Name: "bash"}}, {ID: "t2", Function: llm.Function{Name: "read"}}}
	msgs := []llm.Message{
		llm.UserMessage("run it"),
		callMsg,
		llm.ToolMessage("Error: boom", "t1"),
	}
	out := formatHistory(msgs)
	for _, want := range []string{"tool call: bash, read", "tool error: boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("judge context missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "assistant: \n") {
		t.Error("empty assistant line rendered")
	}
	if strings.Contains(out, "Error: boom") {
		t.Errorf("error prefix must be stripped:\n%s", out)
	}
}

func TestFormatHistoryExcludesToolOutput(t *testing.T) {
	callMsg := llm.AssistantMessage("")
	callMsg.ToolCalls = []llm.ToolCall{{ID: "t1", Function: llm.Function{Name: "bash"}}}
	msgs := []llm.Message{
		llm.UserMessage("run it"),
		callMsg,
		llm.ToolMessage(`{"result": "huge output that must not leak"}`, "t1"),
		llm.UserMessage("what did it say?"),
	}
	out := formatHistory(msgs)
	if strings.Contains(out, "huge output") {
		t.Errorf("tool output must not be included:\n%s", out)
	}
	if !strings.Contains(out, "what did it say?") {
		t.Errorf("user message after tool call missing:\n%s", out)
	}
}

func TestFormatHistoryEmpty(t *testing.T) {
	if got := formatHistory(nil); got != "" {
		t.Fatalf("expected empty context, got %q", got)
	}
	if got := formatHistory([]llm.Message{llm.SystemMessage("x"), llm.ToolMessage("y", "t1")}); got != "" {
		t.Fatalf("expected empty context with only system/tool messages, got %q", got)
	}
}
