package connect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/llm"
)

func (m *runManager) execute(ctx context.Context, r *run, orch *agent.Agent, message string, history []llm.Message) {
	defer m.release(r)

	m.c.emit(eventRunStarted, r.sessionID, r.id, runStartedPayload{
		RunID:     r.id,
		SessionID: r.sessionID,
		AgentID:   r.agentID,
		Provider:  r.provider,
		Model:     r.model,
	})

	progressDone := make(chan struct{})
	go m.progressLoop(ctx, r, progressDone)
	defer close(progressDone)

	timeout := responseTimeout(m.c.cfg)
	if timeout > 0 {
		go m.watchdog(ctx, r, timeout, progressDone)
	}

	handler := func(ev agent.StreamEvent) error {
		m.handleStream(r, ev)
		return nil
	}

	var (
		result *agent.RunResult
		err    error
	)
	if len(history) > 0 {
		result, err = orch.ResumeStream(ctx, message, history, handler)
	} else {
		result, err = orch.RunStream(ctx, message, handler)
	}

	if err != nil {
		if ctx.Err() != nil {
			r.setState(runStateCanceled)
			if r.didTimeout() {
				msg := fmt.Sprintf("model %s/%s did not respond within %s", r.provider, r.model, timeout)
				m.c.trace("run %s timed out: %s", shortID(r.id), msg)
				m.c.emit(eventRunError, r.sessionID, r.id, runErrorPayload{Message: msg})
				Audit("run.timeout", map[string]any{"run_id": r.id, "provider": r.provider, "model": r.model})
				return
			}
			m.c.trace("run %s canceled", shortID(r.id))
			m.c.emit(eventRunError, r.sessionID, r.id, runErrorPayload{Message: "run canceled"})
			Audit("run.canceled", map[string]any{"run_id": r.id, "session_id": r.sessionID})
			return
		}
		r.setState(runStateError)
		m.c.trace("run %s failed: %v", shortID(r.id), err)
		m.c.emit(eventRunError, r.sessionID, r.id, runErrorPayload{Message: err.Error()})
		Audit("run.error", map[string]any{"run_id": r.id, "error": err.Error()})
		return
	}

	r.setState(runStateDone)
	m.c.trace("run %s done in %s", shortID(r.id), time.Since(r.startedAt).Round(time.Millisecond))
	payload := runDonePayload{}
	if result != nil {
		payload.Output = result.Output
		payload.Usage = result.Usage
		payload.Iterations = result.Iterations
		payload.FinishReason = result.FinishReason
	}
	m.c.emit(eventRunDone, r.sessionID, r.id, payload)
	Audit("run.done", map[string]any{"run_id": r.id, "session_id": r.sessionID, "provider": r.provider, "model": r.model})
}

func (m *runManager) progressLoop(ctx context.Context, r *run, done <-chan struct{}) {
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.c.emit(eventRunProgress, r.sessionID, r.id, runProgressPayload{
				ElapsedSecs:   int(time.Since(start).Seconds()),
				AwaitingModel: r.awaitingModel(),
			})
		}
	}
}

func (m *runManager) watchdog(ctx context.Context, r *run, timeout time.Duration, done <-chan struct{}) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-ctx.Done():
		return
	case <-timer.C:
		if r.awaitingModel() {
			r.markTimedOut()
			m.c.trace("run %s produced no output after %s; cancelling", shortID(r.id), timeout)
			r.cancel()
		}
	}
}

func (m *runManager) handleStream(r *run, ev agent.StreamEvent) {
	switch ev.Type {
	case agent.StreamToken:
		r.markOutput()
		m.c.emit(eventRunToken, r.sessionID, r.id, runTokenPayload{
			Content:   ev.Content,
			Reasoning: ev.Reasoning,
		})
	case agent.StreamReasoning:
		r.markOutput()
		m.c.emit(eventRunReasoning, r.sessionID, r.id, runTokenPayload{Reasoning: ev.Reasoning})
	case agent.StreamToolCall:
		if ev.ToolCall != nil {
			m.c.emit(eventRunToolCall, r.sessionID, r.id, runToolCallPayload{
				CallID:   ev.ToolCall.ID,
				ToolName: ev.ToolCall.Function.Name,
				Args:     ev.ToolCall.Function.Arguments,
			})
		}
	case agent.StreamToolResult:
		out := runToolResultPayload{Output: ev.ToolOutput}
		if ev.ToolCall != nil {
			out.CallID = ev.ToolCall.ID
			out.ToolName = ev.ToolCall.Function.Name
		}
		if ev.ToolErr != nil {
			out.Error = ev.ToolErr.Error()
		}
		m.c.emit(eventRunToolResult, r.sessionID, r.id, out)
	case agent.StreamCompaction:
		m.c.emit(eventRunCompaction, r.sessionID, r.id, runCompactionPayload{
			Strategy: ev.Strategy,
			Messages: ev.Messages,
			Tokens:   ev.Tokens,
		})
	case agent.StreamDone:
		if ev.Usage != nil {
			m.c.emit(eventRunUsage, r.sessionID, r.id, runUsagePayload{Usage: *ev.Usage})
		}
	}
}

func loadAttachments(paths []string) ([]llm.Attachment, error) {
	var out []llm.Attachment
	for _, p := range paths {
		clean := filepath.Clean(p)
		if clean == "." || strings.HasPrefix(clean, "..") {
			return nil, fmt.Errorf("invalid attachment path: %q", p)
		}
		data, err := os.ReadFile(clean)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", p, err)
		}
		mediaType := "application/octet-stream"
		attType := llm.AttachmentTypeFile
		if strings.HasPrefix(mediaType, "image/") {
			attType = llm.AttachmentTypeImage
		}
		att := llm.NewAttachmentFromBase64(attType, mediaType, llm.EncodeFileToBase64(data))
		att.FileName = filepath.Base(clean)
		out = append(out, att)
	}
	return out, nil
}
