package codex

import (
	"strings"

	"github.com/peggco/pegg/internal/llm"
)

type codexRequest struct {
	Model             string           `json:"model"`
	Instructions      string           `json:"instructions,omitempty"`
	Input             []map[string]any `json:"input"`
	Tools             []map[string]any `json:"tools,omitempty"`
	ToolChoice        any              `json:"tool_choice,omitempty"`
	ParallelToolCalls bool             `json:"parallel_tool_calls"`
	Reasoning         *codexReasoning  `json:"reasoning,omitempty"`
	Stream            bool             `json:"stream"`
	Store             bool             `json:"store"`
}

type codexReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

func (s *Service) buildRequest(req *llm.Request) *codexRequest {
	cr := &codexRequest{
		Model:             req.Model,
		Stream:            true,
		Store:             false,
		ParallelToolCalls: req.ParallelToolCalls,
	}
	if req.ReasoningEffort != "" {
		cr.Reasoning = &codexReasoning{Effort: req.ReasoningEffort, Summary: "auto"}
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			cr.Instructions = joinText(cr.Instructions, llm.MessageText(msg))
		case llm.RoleUser:
			cr.Input = append(cr.Input, userItem(msg))
		case llm.RoleAssistant:
			if txt := llm.MessageText(msg); txt != "" {
				cr.Input = append(cr.Input, textItem("assistant", "output_text", txt))
			}
			for _, tc := range msg.ToolCalls {
				cr.Input = append(cr.Input, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
		case llm.RoleTool:
			cr.Input = append(cr.Input, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  llm.MessageText(msg),
			})
		}
	}

	for _, t := range req.Tools {
		cr.Tools = append(cr.Tools, map[string]any{
			"type":        "function",
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"parameters":  t.Function.Parameters,
		})
	}

	cr.ToolChoice = mapToolChoice(req.ToolChoice)
	return cr
}

func userItem(msg llm.Message) map[string]any {
	var parts []map[string]any
	switch c := msg.Content.(type) {
	case string:
		parts = append(parts, map[string]any{"type": "input_text", "text": c})
	case llm.Content:
		if c.Text != "" {
			parts = append(parts, map[string]any{"type": "input_text", "text": c.Text})
		}
		for _, att := range c.Attachments {
			if att.Type != llm.AttachmentTypeImage {
				continue
			}
			url := att.URL
			if url == "" && att.Data != "" {
				url = "data:" + att.MediaType + ";base64," + att.Data
			}
			if url != "" {
				parts = append(parts, map[string]any{"type": "input_image", "image_url": url})
			}
		}
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"type": "input_text", "text": ""})
	}
	return map[string]any{"type": "message", "role": "user", "content": parts}
}

func textItem(role, contentType, text string) map[string]any {
	return map[string]any{
		"type":    "message",
		"role":    role,
		"content": []map[string]any{{"type": contentType, "text": text}},
	}
}

func mapToolChoice(choice any) any {
	switch c := choice.(type) {
	case string:
		if c == "" {
			return nil
		}
		return c
	case llm.ToolChoice:
		if c.Function.Name != "" {
			return map[string]any{"type": "function", "name": c.Function.Name}
		}
		if c.Type != "" {
			return c.Type
		}
	}
	return nil
}

func joinText(a, b string) string {
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n\n" + b
}
