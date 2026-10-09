package middlewares

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestRedactionBeforeLLMRedactsUserAndToolOnly(t *testing.T) {
	r := NewRedaction()
	systemContent := `password = "sys-secret-1234567890"`
	assistantArgs := `{"content":"apiKey = k.apiKey()"}`
	req := llm.NewRequest("m", []llm.Message{
		llm.SystemMessage(systemContent),
		llm.UserMessage(`my key is sk-user-secret-1234567890abcdef`),
		llm.ToolMessage(`read result: {"token":"file-secret-1234567890"}`, "call-1"),
		{
			Role:    llm.RoleAssistant,
			Content: "ok",
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Arguments: assistantArgs}},
			},
		},
	})
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}

	if got := req.Messages[0].Content.(string); got != systemContent {
		t.Errorf("system message must survive redaction, got %q", got)
	}

	for i := 1; i <= 2; i++ {
		s, _ := req.Messages[i].Content.(string)
		if !strings.Contains(s, RedactionToken) {
			t.Errorf("message %d content not redacted: %q", i, s)
		}
	}

	if s, _ := req.Messages[3].Content.(string); s != "ok" {
		t.Errorf("assistant content must survive redaction, got %q", s)
	}
	if got := req.Messages[3].ToolCalls[0].Function.Arguments; got != assistantArgs {
		t.Errorf("assistant tool-call args must survive redaction, got %q", got)
	}
}

func TestRedactionBeforeLLMSkipsSystemMessages(t *testing.T) {
	r := NewRedaction()
	content := `api_key = "sk-system-secret-1234567890abcdef"`
	req := llm.NewRequest("m", []llm.Message{
		llm.SystemMessage(content),
	})
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if got := req.Messages[0].Content.(string); got != content {
		t.Errorf("system message must not be redacted, got %q", got)
	}
}

func TestRedactionRedactString(t *testing.T) {
	r := NewRedaction()
	if got := r.RedactString("sk-should-be-redacted-1234567890"); !strings.Contains(got, RedactionToken) {
		t.Errorf("RedactString = %q", got)
	}
}

func TestRedactionNilSafe(t *testing.T) {
	r := NewRedaction()
	if err := r.BeforeLLM(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestRedactionBeforeLLMSkipsMediaContentParts(t *testing.T) {
	r := NewRedaction()
	img := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	req := llm.NewRequest("m", []llm.Message{
		llm.UserMessage([]any{
			map[string]any{"type": "text", "text": `api_key="sk-should-be-redacted-1234567890"`},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + img}},
		}),
	})
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	parts := req.Messages[0].Content.([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, RedactionToken) {
		t.Errorf("text part not redacted: %q", text)
	}
	url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if strings.Contains(url, RedactionToken) {
		t.Errorf("image url payload must not be redacted: %q", url)
	}
}

func TestRedactionBeforeLLMEmptyMessages(t *testing.T) {
	r := NewRedaction()
	req := llm.NewRequest("m", nil)
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
}
