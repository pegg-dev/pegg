package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/llm"
)

func TestNewSSEWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", w.Header().Get("Cache-Control"))
	}
	if w.Header().Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %s", w.Header().Get("Connection"))
	}
	if w.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %s", w.Header().Get("X-Accel-Buffering"))
	}
	if sse == nil {
		t.Fatal("expected non-nil SSEWriter")
	}
}

func TestNewSSEWriterNonFlushable(t *testing.T) {
	w := &nonFlushableResponseWriter{httptest.NewRecorder()}
	_, err := NewSSEWriter(w)
	if err == nil {
		t.Error("expected error for non-flushable writer")
	}
}

func TestSSEWriterWriteEvent(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	event := SSEEvent{Type: "token", Content: "hello"}
	err = sse.WriteEvent("agent", event)
	if err != nil {
		t.Fatalf("failed to write event: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: agent") {
		t.Errorf("expected 'event: agent' in body, got %s", body)
	}
	if !strings.Contains(body, "\"type\":\"token\"") {
		t.Errorf("expected token type in body, got %s", body)
	}
	if !strings.Contains(body, "\"content\":\"hello\"") {
		t.Errorf("expected content hello in body, got %s", body)
	}
}

func TestSSEWriterWriteMultipleEvents(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	sse.WriteEvent("agent", SSEEvent{Type: "token", Content: "a"})
	sse.WriteEvent("agent", SSEEvent{Type: "token", Content: "b"})
	sse.WriteEvent("done", SSEEvent{Done: true})

	body := w.Body.String()
	if strings.Count(body, "event: agent") != 2 {
		t.Errorf("expected 2 agent events, got %d", strings.Count(body, "event: agent"))
	}
	if strings.Count(body, "event: done") != 1 {
		t.Errorf("expected 1 done event, got %d", strings.Count(body, "event: done"))
	}
}

func TestSSEWriterClose(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	sse.Close()

	err = sse.WriteEvent("agent", SSEEvent{Type: "token"})
	if err == nil {
		t.Error("expected error writing after close")
	}
}

func TestSSEWriterWriteAfterClose(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	sse.Close()

	err = sse.WriteEvent("agent", SSEEvent{Type: "token"})
	if err == nil {
		t.Error("expected error writing after close")
	}
}

func TestSSEWriterDoubleClose(t *testing.T) {
	w := httptest.NewRecorder()
	sse, err := NewSSEWriter(w)
	if err != nil {
		t.Fatalf("failed to create SSE writer: %v", err)
	}

	sse.Close()
	sse.Close()

	err = sse.WriteEvent("agent", SSEEvent{Type: "token"})
	if err == nil {
		t.Error("expected error writing after close")
	}
}

func TestSSEEventSerialization(t *testing.T) {
	event := SSEEvent{
		Type:       "token",
		Content:    "hello",
		Reasoning:  "thinking",
		ToolName:   "bash",
		ToolArgs:   "{}",
		ToolOutput: "output",
		Error:      "err",
		Done:       true,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SSEEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != "token" {
		t.Errorf("expected type token, got %s", decoded.Type)
	}
	if decoded.Content != "hello" {
		t.Errorf("expected content hello, got %s", decoded.Content)
	}
	if decoded.Done != true {
		t.Error("expected done true")
	}
}

func TestSSEEventWithUsage(t *testing.T) {
	event := SSEEvent{
		Type:  "done",
		Done:  true,
		Usage: llm.Usage{TotalTokens: 100, Cost: 0.05},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), "\"total_tokens\":100") {
		t.Errorf("expected total_tokens in output, got %s", string(data))
	}
}

func TestSSEEventWithCompaction(t *testing.T) {
	event := SSEEvent{
		Type:     "compaction",
		Strategy: "sliding-window",
		Messages: 30,
		Tokens:   8000,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SSEEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Type != "compaction" || decoded.Strategy != "sliding-window" || decoded.Messages != 30 || decoded.Tokens != 8000 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestSSEEventEmptyFields(t *testing.T) {
	event := SSEEvent{}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SSEEvent
	json.Unmarshal(data, &decoded)

	if decoded.Done != false {
		t.Error("expected done false")
	}
}
