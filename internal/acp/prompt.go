package acp

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/google/uuid"
	"github.com/peggco/pegg/internal/agent"
)

type SessionPromptParams struct {
	SessionId SessionId      `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type SessionPromptResult struct {
	StopReason StopReason `json:"stopReason"`
	Usage      *UsageInfo `json:"usage,omitempty"`
}

type UsageInfo struct {
	Used int   `json:"used"`
	Size int   `json:"size"`
	Cost *Cost `json:"cost,omitempty"`
}

func (s *Server) handleSessionPrompt(ctx context.Context, params json.RawMessage, id any) (any, error) {
	var req SessionPromptParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/prompt params"}
	}

	acpSess, ok := s.getSession(string(req.SessionId))
	if !ok {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "session not found"}
	}

	input := extractPromptText(req.Prompt)

	promptCtx, cancel := context.WithCancel(ctx)
	acpSess.SetCancel(cancel, id)

	eventCh := make(chan agent.StreamEvent, 64)
	go func() {
		defer close(eventCh)
		for ev := range eventCh {
			switch ev.Type {
			case agent.StreamToken:
				s.notify(string(req.SessionId), SessionUpdate{
					SessionUpdate: "agent_message_chunk",
					MessageID:     uuid.NewString(),
					Content:       &ContentBlock{Type: "text", Text: ev.Content},
				})
			case agent.StreamToolCall:
				if ev.ToolCall != nil {
					s.notify(string(req.SessionId), SessionUpdate{
						SessionUpdate: "tool_call",
						ToolCallID:    ev.ToolCall.ID,
						Title:         ev.ToolCall.Function.Name,
						Kind:          string(ToolKindOther),
						Status:        string(ToolCallPending),
					})
				}
			case agent.StreamToolResult:
				if ev.ToolCall != nil {
					s.notify(string(req.SessionId), SessionUpdate{
						SessionUpdate: "tool_call_update",
						ToolCallID:    ev.ToolCall.ID,
						Status:        string(ToolCallCompleted),
					})
				}
			case agent.StreamCompaction:
				s.notify(string(req.SessionId), SessionUpdate{
					SessionUpdate: "agent_message_chunk",
					MessageID:     uuid.NewString(),
					Content: &ContentBlock{Type: "text",
						Text: fmt.Sprintf("[Context compacted (%s) — %d messages, %d tokens]",
							ev.Strategy, ev.Messages, ev.Tokens)},
				})
			}
		}
	}()

	result, err := acpSess.Agent.RunStream(promptCtx, input, func(ev agent.StreamEvent) error {
		select {
		case eventCh <- ev:
		case <-promptCtx.Done():
			return promptCtx.Err()
		}
		return nil
	})

	close(eventCh)

	if result != nil && result.Usage.TotalTokens > 0 {
		s.notify(string(req.SessionId), SessionUpdate{
			SessionUpdate: "usage_update",
			Used:          result.Usage.TotalTokens,
			Size:          200000,
			Cost: &Cost{
				Amount:   result.Usage.Cost,
				Currency: "USD",
			},
		})
	}

	stopReason := StopReasonEndTurn
	if err != nil {
		if promptCtx.Err() != nil {
			stopReason = StopReasonCancelled
		} else {
			stopReason = StopReasonError
		}
	}

	return SessionPromptResult{StopReason: stopReason}, nil
}

func extractPromptText(blocks []ContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}
