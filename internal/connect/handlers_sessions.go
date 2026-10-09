package connect

import (
	"context"

	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
)

type sessionDTO struct {
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

func toSessionDTO(s *session.Session) sessionDTO {
	return sessionDTO{
		ID:              s.ID,
		Title:           s.Title,
		Provider:        s.Provider,
		Model:           s.Model,
		ReasoningEffort: s.ReasoningEffort,
		ProjectDir:      s.ProjectDir,
		ParentID:        s.ParentID,
		CreatedAt:       s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       s.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Usage:           s.Usage,
	}
}

func toSessionMessage(m *session.Message) SessionMessage {
	return SessionMessage{
		ID:         m.ID,
		SessionID:  m.SessionID,
		Seq:        m.Seq,
		Role:       string(m.Role),
		Content:    m.Content,
		Reasoning:  m.Reasoning,
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
		ToolCalls:  m.ToolCalls,
		CreatedAt:  m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func (c *Connector) registerSessionMethods() {
	c.dispatch.register("session.list", ScopeSessionsRead, c.handleSessionList)
	c.dispatch.register("session.get", ScopeSessionsRead, c.handleSessionGet)
	c.dispatch.register("session.messages", ScopeSessionsRead, c.handleSessionMessages)
	c.dispatch.register("session.snapshots", ScopeSessionsRead, c.handleSessionSnapshots)
	c.dispatch.register("session.create", ScopeSessionsWrite, c.handleSessionCreate)
	c.dispatch.register("session.rename", ScopeSessionsWrite, c.handleSessionRename)
	c.dispatch.register("session.delete", ScopeSessionsWrite, c.handleSessionDelete)
	c.dispatch.register("session.revert", ScopeSessionsWrite, c.handleSessionRevert)
	c.dispatch.register("session.undo_revert", ScopeSessionsWrite, c.handleSessionUndoRevert)
	c.dispatch.register("session.fork", ScopeSessionsWrite, c.handleSessionFork)
}

type sessionListParams struct {
	Search string `json:"search"`
	Page   int    `json:"page"`
	Size   int    `json:"size"`
	All    bool   `json:"all"`
}

func (c *Connector) handleSessionList(_ context.Context, req *Request) (any, error) {
	if c.deps.Sessions == nil {
		return map[string]any{"sessions": []sessionDTO{}, "total": 0}, nil
	}
	var p sessionListParams
	_ = req.Decode(&p)
	q := listQuery(p.Page, p.Size)
	if p.Search != "" {
		q.Search = p.Search
		q.SearchColumns = []string{"title"}
	}
	if !p.All {
		q.Filters = append(q.Filters, queryFilterProject(c.workspace()))
	}
	sessions, total, err := c.deps.Sessions.List(q)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "list sessions: " + err.Error()}
	}
	out := make([]sessionDTO, len(sessions))
	for i := range sessions {
		out[i] = toSessionDTO(&sessions[i])
	}
	return map[string]any{"sessions": out, "total": total}, nil
}

type sessionRefParams struct {
	ID string `json:"id"`
}

func (c *Connector) sessionID(req *Request) string {
	var p sessionRefParams
	_ = req.Decode(&p)
	if p.ID != "" {
		return p.ID
	}
	return req.SessionID
}

func (c *Connector) requireSessions() error {
	if c.deps.Sessions == nil {
		return &FrameError{Code: ErrCodeUnavailable, Message: "session store unavailable"}
	}
	return nil
}

func (c *Connector) handleSessionGet(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	id := c.sessionID(req)
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "session id is required"}
	}
	s, err := c.deps.Sessions.Get(id)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "session not found"}
	}
	return toSessionDTO(s), nil
}

func (c *Connector) handleSessionMessages(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	id := c.sessionID(req)
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "session id is required"}
	}
	msgs, err := c.deps.Sessions.Messages(id)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "session not found"}
	}
	out := make([]SessionMessage, len(msgs))
	for i := range msgs {
		out[i] = toSessionMessage(&msgs[i])
	}
	return map[string]any{"messages": out}, nil
}

type sessionCreateParams struct {
	Title    string `json:"title"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (c *Connector) handleSessionCreate(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	var p sessionCreateParams
	_ = req.Decode(&p)
	s, err := c.deps.Sessions.Create(session.CreateOptions{
		Title:      p.Title,
		Provider:   p.Provider,
		Model:      p.Model,
		ProjectDir: c.workspace(),
	})
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "create session: " + err.Error()}
	}
	return toSessionDTO(s), nil
}
