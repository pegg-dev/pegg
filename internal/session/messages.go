package session

import "github.com/peggco/pegg/internal/llm"

func MessagesToLLM(msgs []Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			Reasoning:  m.Reasoning,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
			ToolCalls:  m.ToolCalls,
		})
	}
	return out
}
