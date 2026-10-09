package middlewares

import (
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func contentMsg(s string) *llm.Message {
	return &llm.Message{
		Role:    llm.RoleAssistant,
		Content: s,
	}
}

func reasoningMsg(content, reasoning string) *llm.Message {
	return &llm.Message{
		Role:      llm.RoleAssistant,
		Content:   content,
		Reasoning: reasoning,
	}
}

func toolCallMsg(content string, calls []llm.ToolCall) *llm.Message {
	return &llm.Message{
		Role:      llm.RoleAssistant,
		Content:   content,
		ToolCalls: calls,
	}
}

func TestDetectTextLoop(t *testing.T) {
	window := []string{"hello world", "hello world", "hello world"}
	reason := detectLoop(window, "text")
	if reason == "" {
		t.Fatal("expected loop detection")
	}
}

func TestDetectTextLoopBelowThreshold(t *testing.T) {
	window := []string{"hello world", "hello world"}
	reason := detectLoop(window, "text")
	if reason != "" {
		t.Fatalf("expected no detection, got: %s", reason)
	}
}

func TestDetectTextLoopEmpty(t *testing.T) {
	reason := detectLoop(nil, "text")
	if reason != "" {
		t.Fatalf("expected no detection, got: %s", reason)
	}
}

func TestDetectTextLoopNonConsecutive(t *testing.T) {
	window := []string{"a", "b", "a", "b", "c"}
	reason := detectLoop(window, "text")
	if reason != "" {
		t.Fatalf("expected no detection for non-consecutive, got: %s", reason)
	}
}

func TestToolSig(t *testing.T) {
	tc := llm.ToolCall{
		Function: llm.Function{
			Name:      "read",
			Arguments: `{"filePath": "main.go"}`,
		},
	}
	sig := toolSig(tc)
	if sig != `read({"filePath": "main.go"})` {
		t.Errorf("expected 'read({...})', got %q", sig)
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"  hello   world  ", "hello world"},
		{"\nhello\nworld\n", "hello world"},
		{"", ""},
		{"  ", ""},
	}
	for _, tt := range tests {
		got := normalizeText(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeText(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoopDetectorTextLoop(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	for i := 0; i < 3; i++ {
		resp := &llm.Response{
			Choices: []llm.Choice{{Message: contentMsg("same text")}},
		}
		if err := ld.AfterLLM(t.Context(), nil, resp); err != nil {
			t.Fatal(err)
		}
	}

	ld.mu.Lock()
	detected := ld.loopDetected
	reason := ld.loopReason
	ld.mu.Unlock()

	if !detected {
		t.Fatal("expected loop detection after 3 identical texts")
	}
	if !contains(reason, "text") {
		t.Errorf("expected 'text' in reason, got: %s", reason)
	}

	req := llm.NewRequest("test", nil)
	if err := ld.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 injected message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != llm.RoleSystem {
		t.Errorf("expected system role, got %v", req.Messages[0].Role)
	}

	ld.mu.Lock()
	if ld.loopDetected {
		t.Fatal("loop flag should be cleared after injection")
	}
	ld.mu.Unlock()
}

func TestLoopDetectorToolLoop(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	for i := 0; i < 3; i++ {
		resp := &llm.Response{
			Choices: []llm.Choice{{
				Message: toolCallMsg("", []llm.ToolCall{
					{Function: llm.Function{Name: "read", Arguments: `{"x":"y"}`}},
				}),
			}},
		}
		ld.AfterLLM(t.Context(), nil, resp)
	}

	ld.mu.Lock()
	detected := ld.loopDetected
	reason := ld.loopReason
	ld.mu.Unlock()

	if !detected {
		t.Fatal("expected loop detection for repeated tool calls")
	}
	if !contains(reason, "text-tool") {
		t.Errorf("expected 'text-tool' in reason, got: %s", reason)
	}
}

func TestLoopDetectorNoDetection(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	texts := []string{"first", "second", "third", "fourth", "fifth"}
	for _, txt := range texts {
		resp := &llm.Response{
			Choices: []llm.Choice{{Message: contentMsg(txt)}},
		}
		ld.AfterLLM(t.Context(), nil, resp)
	}

	ld.mu.Lock()
	if ld.loopDetected {
		ld.mu.Unlock()
		t.Fatal("expected no loop detection for different texts")
	}
	ld.mu.Unlock()
}

func TestOnErrorResets(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	resp := &llm.Response{
		Choices: []llm.Choice{{Message: contentMsg("same")}},
	}
	for i := 0; i < 3; i++ {
		ld.AfterLLM(t.Context(), nil, resp)
	}

	ld.mu.Lock()
	if !ld.loopDetected {
		ld.mu.Unlock()
		t.Fatal("expected loop detection")
	}
	ld.mu.Unlock()

	ld.OnError(t.Context(), nil)

	ld.mu.Lock()
	if ld.loopDetected {
		t.Fatal("loop flag should be reset after OnError")
	}
	if len(ld.recentTexts) != 0 {
		t.Fatal("texts should be reset after OnError")
	}
	ld.mu.Unlock()
}

func TestDetectReasoningLoop(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	for i := 0; i < 3; i++ {
		resp := &llm.Response{
			Choices: []llm.Choice{{
				Message: reasoningMsg("diff text "+itoa(i), "same reasoning"),
			}},
		}
		ld.AfterLLM(t.Context(), nil, resp)
	}

	ld.mu.Lock()
	detected := ld.loopDetected
	reason := ld.loopReason
	ld.mu.Unlock()

	if !detected {
		t.Fatal("expected reasoning loop detection")
	}
	if !contains(reason, "thinking") {
		t.Errorf("expected 'thinking' in reason, got: %s", reason)
	}
}

func TestDetectCombinedTextToolLoop(t *testing.T) {
	ld := NewLoopDetector()
	ld.BeforeRun(t.Context(), "test", "input")

	for i := 0; i < 3; i++ {
		resp := &llm.Response{
			Choices: []llm.Choice{{
				Message: toolCallMsg("thinking step", []llm.ToolCall{
					{Function: llm.Function{Name: "read", Arguments: `{"x":"y"}`}},
				}),
			}},
		}
		ld.AfterLLM(t.Context(), nil, resp)
	}

	ld.mu.Lock()
	detected := ld.loopDetected
	reason := ld.loopReason
	ld.mu.Unlock()

	if !detected {
		t.Fatal("expected combined text-tool loop detection")
	}
	if !contains(reason, "text-tool") {
		t.Errorf("expected 'text-tool' in reason, got: %s", reason)
	}
}

func TestDetectResponseLoopTextOnly(t *testing.T) {
	recs := []responseRecord{
		{content: "hello", toolSigs: nil},
		{content: "hello", toolSigs: nil},
		{content: "hello", toolSigs: nil},
	}
	reason := detectResponseLoop(recs)
	if reason == "" {
		t.Fatal("expected response loop detection")
	}
}

func TestDetectResponseLoopToolOnly(t *testing.T) {
	recs := []responseRecord{
		{content: "", toolSigs: []string{"read(x)"}},
		{content: "", toolSigs: []string{"read(x)"}},
		{content: "", toolSigs: []string{"read(x)"}},
	}
	reason := detectResponseLoop(recs)
	if reason == "" {
		t.Fatal("expected tool-only loop detection")
	}
}

func TestDetectResponseLoopBelowThreshold(t *testing.T) {
	recs := []responseRecord{
		{content: "a"},
		{content: "a"},
	}
	reason := detectResponseLoop(recs)
	if reason != "" {
		t.Fatalf("expected no detection, got: %s", reason)
	}
}

func TestDetectResponseLoopEmpty(t *testing.T) {
	reason := detectResponseLoop(nil)
	if reason != "" {
		t.Fatalf("expected no detection, got: %s", reason)
	}
}

func TestDetectResponseLoopNonConsecutive(t *testing.T) {
	recs := []responseRecord{
		{content: "a"},
		{content: "b"},
		{content: "a"},
	}
	reason := detectResponseLoop(recs)
	if reason != "" {
		t.Fatalf("expected no detection for non-consecutive, got: %s", reason)
	}
}

func TestResponsesEqual(t *testing.T) {
	a := responseRecord{content: "hello", toolSigs: []string{"read(x)"}}
	b := responseRecord{content: "hello", toolSigs: []string{"read(x)"}}
	c := responseRecord{content: "hello", toolSigs: []string{"write(x)"}}
	d := responseRecord{content: "world", toolSigs: []string{"read(x)"}}

	if !responsesEqual(a, b) {
		t.Error("a and b should be equal")
	}
	if responsesEqual(a, c) {
		t.Error("a and c should differ (different tool sigs)")
	}
	if responsesEqual(a, d) {
		t.Error("a and d should differ (different content)")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	n := i
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
