package middlewares

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestRedactionBeforeLLMRedactsAllMessages(t *testing.T) {
	r := NewRedaction()
	req := llm.NewRequest("m", []llm.Message{
		llm.SystemMessage(`password = "sys-secret-1234567890"`),
		llm.UserMessage(`my key is sk-abcdefghijklmnopqrstuvwxyz1234`),
		llm.ToolMessage(`read result: {"token": "file-secret-1234567890"}`, "call-1"),
		{
			Role:    llm.RoleAssistant,
			Content: "ok",
			ToolCalls: []llm.ToolCall{
				{Function: llm.Function{Arguments: `{"content":"apiKey := k.apiKey()"}`}},
			},
		},
	})
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	for i, m := range req.Messages[:3] {
		s, _ := m.Content.(string)
		if strings.Contains(s, "sk-abcdefghijklmnopqrstuvwxyz1234") {
			t.Errorf("message %d content not redacted: %q", i, s)
		}
		if strings.Contains(s, "file-secret-1234567890") {
			t.Errorf("message %d content not redacted: %q", i, s)
		}
		if strings.Contains(s, "sys-secret-1234567890") {
			t.Errorf("message %d content not redacted: %q", i, s)
		}
	}
	if s, _ := req.Messages[3].Content.(string); s != "ok" {
		t.Errorf("assistant content must survive redaction, got %q", s)
	}
	args := req.Messages[3].ToolCalls[0].Function.Arguments
	if args != `{"content":"apiKey := k.apiKey()"}` {
		t.Errorf("assistant tool-call args must survive redaction, got %q", args)
	}
}

func TestRedactionRedactString(t *testing.T) {
	r := NewRedaction()
	if got := r.RedactString("AKIAIOSFODNN7EXAMPLE"); !strings.Contains(got, "[**REDACTED**]") {
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
	img := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
	req := llm.NewRequest("m", []llm.Message{
		{Content: []any{
			map[string]any{"type": "text", "text": `api_key="sk-1234567890abcdefghijklmno"`},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + img}},
		}},
	})
	if err := r.BeforeLLM(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	parts := req.Messages[0].Content.([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "[**REDACTED**]") {
		t.Errorf("text part not redacted: %q", text)
	}
	url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
	if strings.Contains(url, "[**REDACTED**]") {
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
