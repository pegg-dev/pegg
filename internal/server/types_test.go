package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

func TestToSessionResponse(t *testing.T) {
	now := time.Now()
	s := &session.Session{
		ID:              "s1",
		Title:           "test session",
		Provider:        "openai",
		Model:           "gpt-4",
		ReasoningEffort: "high",
		ProjectDir:      "/tmp/project",
		ParentID:        "p1",
		CreatedAt:       now,
		UpdatedAt:       now,
		Usage:           llm.Usage{TotalTokens: 100},
	}
	resp := toSessionResponse(s)

	if resp.ID != "s1" {
		t.Errorf("expected ID s1, got %s", resp.ID)
	}
	if resp.Title != "test session" {
		t.Errorf("expected Title 'test session', got %s", resp.Title)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected Provider openai, got %s", resp.Provider)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("expected Model gpt-4, got %s", resp.Model)
	}
	if resp.ReasoningEffort != "high" {
		t.Errorf("expected ReasoningEffort high, got %s", resp.ReasoningEffort)
	}
	if resp.ProjectDir != "/tmp/project" {
		t.Errorf("expected ProjectDir /tmp/project, got %s", resp.ProjectDir)
	}
	if resp.ParentID != "p1" {
		t.Errorf("expected ParentID p1, got %s", resp.ParentID)
	}
	if resp.CreatedAt != now.Format("2006-01-02T15:04:05Z") {
		t.Errorf("expected CreatedAt %s, got %s", now.Format("2006-01-02T15:04:05Z"), resp.CreatedAt)
	}
	if resp.Usage.TotalTokens != 100 {
		t.Errorf("expected Usage.TotalTokens 100, got %d", resp.Usage.TotalTokens)
	}
}

func TestToSessionResponseEmpty(t *testing.T) {
	s := &session.Session{}
	resp := toSessionResponse(s)
	if resp.ID != "" {
		t.Errorf("expected empty ID, got %s", resp.ID)
	}
	if resp.Title != "" {
		t.Errorf("expected empty Title, got %s", resp.Title)
	}
}

func TestToSessionResponseZeroTime(t *testing.T) {
	s := &session.Session{
		ID:        "s1",
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}
	resp := toSessionResponse(s)
	if resp.CreatedAt != "0001-01-01T00:00:00Z" {
		t.Errorf("expected zero time, got %s", resp.CreatedAt)
	}
}

func TestToMessageResponse(t *testing.T) {
	now := time.Now()
	m := &session.Message{
		ID:         "m1",
		SessionID:  "s1",
		Seq:        3,
		Role:       llm.RoleUser,
		Content:    "hello",
		Reasoning:  "thinking",
		Name:       "test",
		ToolCallID: "tc1",
		ToolCalls:  []llm.ToolCall{{ID: "tc1", Type: "function", Function: llm.Function{Name: "bash"}}},
		CreatedAt:  now,
	}
	resp := toMessageResponse(m)

	if resp.ID != "m1" {
		t.Errorf("expected ID m1, got %s", resp.ID)
	}
	if resp.SessionID != "s1" {
		t.Errorf("expected SessionID s1, got %s", resp.SessionID)
	}
	if resp.Seq != 3 {
		t.Errorf("expected Seq 3, got %d", resp.Seq)
	}
	if resp.Role != "user" {
		t.Errorf("expected Role user, got %s", resp.Role)
	}
	if resp.Content != "hello" {
		t.Errorf("expected Content 'hello', got %v", resp.Content)
	}
	if resp.Reasoning != "thinking" {
		t.Errorf("expected Reasoning 'thinking', got %v", resp.Reasoning)
	}
	if resp.Name != "test" {
		t.Errorf("expected Name 'test', got %s", resp.Name)
	}
	if resp.ToolCallID != "tc1" {
		t.Errorf("expected ToolCallID tc1, got %s", resp.ToolCallID)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 ToolCall, got %d", len(resp.ToolCalls))
	}
}

func TestToMessageResponseEmpty(t *testing.T) {
	m := &session.Message{}
	resp := toMessageResponse(m)
	if resp.ID != "" {
		t.Errorf("expected empty ID, got %s", resp.ID)
	}
	if resp.Role != "" {
		t.Errorf("expected empty Role, got %s", resp.Role)
	}
}

func TestToMessageResponseNilFields(t *testing.T) {
	m := &session.Message{
		ID:      "m1",
		Role:    llm.RoleUser,
		Seq:     1,
		Content: "hello",
	}
	resp := toMessageResponse(m)
	if resp.Reasoning != nil {
		t.Errorf("expected nil reasoning, got %v", resp.Reasoning)
	}
	if resp.ToolCalls != nil {
		t.Errorf("expected nil tool calls, got %v", resp.ToolCalls)
	}
}

func TestRunRequestSerialization(t *testing.T) {
	req := RunRequest{
		Message:   "hello",
		SessionID: "s1",
		Model:     "gpt-4",
		Provider:  "openai",
		Files:     []string{"file1.txt"},
		Stream:    true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RunRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Message != "hello" {
		t.Errorf("expected message hello, got %s", decoded.Message)
	}
	if decoded.SessionID != "s1" {
		t.Errorf("expected session_id s1, got %s", decoded.SessionID)
	}
	if decoded.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", decoded.Model)
	}
	if decoded.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", decoded.Provider)
	}
	if !decoded.Stream {
		t.Error("expected stream true")
	}
}

func TestRunRequestOptionalFields(t *testing.T) {
	req := RunRequest{Message: "test"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RunRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SessionID != "" {
		t.Errorf("expected empty session_id, got %s", decoded.SessionID)
	}
	if decoded.Model != "" {
		t.Errorf("expected empty model, got %s", decoded.Model)
	}
	if decoded.Stream {
		t.Error("expected stream false")
	}
}

func TestRunResponseSerialization(t *testing.T) {
	resp := RunResponse{SessionID: "s1", AgentID: "a1"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RunResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SessionID != "s1" {
		t.Errorf("expected session_id s1, got %s", decoded.SessionID)
	}
	if decoded.AgentID != "a1" {
		t.Errorf("expected agent_id a1, got %s", decoded.AgentID)
	}
}

func TestErrorResponseSerialization(t *testing.T) {
	resp := ErrorResponse{Error: "something went wrong"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Error != "something went wrong" {
		t.Errorf("expected error message, got %s", decoded.Error)
	}
}

func TestSessionResponseSerialization(t *testing.T) {
	resp := SessionResponse{
		ID:              "s1",
		Title:           "test",
		Provider:        "openai",
		Model:           "gpt-4",
		ReasoningEffort: "high",
		ProjectDir:      "/tmp",
		ParentID:        "p1",
		CreatedAt:       "2024-01-01T00:00:00Z",
		UpdatedAt:       "2024-01-01T00:00:00Z",
		Usage:           llm.Usage{TotalTokens: 100},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SessionResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "s1" {
		t.Errorf("expected ID s1, got %s", decoded.ID)
	}
	if decoded.ReasoningEffort != "high" {
		t.Errorf("expected ReasoningEffort high, got %s", decoded.ReasoningEffort)
	}
}

func TestMessageResponseSerialization(t *testing.T) {
	resp := MessageResponse{
		ID:         "m1",
		SessionID:  "s1",
		Seq:        1,
		Role:       "user",
		Content:    "hello",
		Reasoning:  "thinking",
		Name:       "test",
		ToolCallID: "tc1",
		ToolCalls:  []llm.ToolCall{{ID: "tc1", Type: "function"}},
		CreatedAt:  "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MessageResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ID != "m1" {
		t.Errorf("expected ID m1, got %s", decoded.ID)
	}
	if decoded.ToolCallID != "tc1" {
		t.Errorf("expected ToolCallID tc1, got %s", decoded.ToolCallID)
	}
}

func TestListSessionsResponseSerialization(t *testing.T) {
	resp := ListSessionsResponse{
		Sessions: []SessionResponse{
			{ID: "s1", Title: "test1"},
			{ID: "s2", Title: "test2"},
		},
		Total: 2,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ListSessionsResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Total != 2 {
		t.Errorf("expected total 2, got %d", decoded.Total)
	}
	if len(decoded.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(decoded.Sessions))
	}
}

func TestHealthResponseSerialization(t *testing.T) {
	resp := HealthResponse{Status: "ok", Version: "1.0.0"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded HealthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Status != "ok" {
		t.Errorf("expected status ok, got %s", decoded.Status)
	}
	if decoded.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", decoded.Version)
	}
}

func TestModelEntrySerialization(t *testing.T) {
	entry := ModelEntry{
		Provider: "openai",
		Model:    llm.Model{ID: "gpt-4", Name: "GPT-4"},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ModelEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", decoded.Provider)
	}
	if decoded.Model.ID != "gpt-4" {
		t.Errorf("expected model ID gpt-4, got %s", decoded.Model.ID)
	}
}
