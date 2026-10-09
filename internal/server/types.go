package server

import (
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type RunRequest struct {
	Message   string   `json:"message"`
	SessionID string   `json:"session_id,omitempty"`
	Model     string   `json:"model,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Files     []string `json:"files,omitempty"`
	Stream    bool     `json:"stream"`
}

type RunResponse struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SessionResponse struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	ProjectDir      string    `json:"project_dir,omitempty"`
	ParentID        string    `json:"parent_id,omitempty"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
	Usage           llm.Usage `json:"usage"`
}

type MessageResponse struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"session_id"`
	Seq        int            `json:"seq"`
	Role       string         `json:"role"`
	Content    any            `json:"content"`
	Reasoning  any            `json:"reasoning,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []llm.ToolCall `json:"tool_calls,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Total    int               `json:"total"`
}

type ModelsResponse struct {
	Models []ModelEntry `json:"models"`
}

type ModelEntry struct {
	Provider string    `json:"provider"`
	Model    llm.Model `json:"model"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func toSessionResponse(s *session.Session) SessionResponse {
	return SessionResponse{
		ID:              s.ID,
		Title:           s.Title,
		Provider:        s.Provider,
		Model:           s.Model,
		ReasoningEffort: s.ReasoningEffort,
		ProjectDir:      s.ProjectDir,
		ParentID:        s.ParentID,
		CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Usage:           s.Usage,
	}
}

func toMessageResponse(m *session.Message) MessageResponse {
	return MessageResponse{
		ID:         m.ID,
		SessionID:  m.SessionID,
		Seq:        m.Seq,
		Role:       string(m.Role),
		Content:    m.Content,
		Reasoning:  m.Reasoning,
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
		ToolCalls:  m.ToolCalls,
		CreatedAt:  m.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
