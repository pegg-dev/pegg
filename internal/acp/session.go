package acp

import (
	"context"
	"fmt"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/utils/query"
)

type SessionNewParams struct {
	Cwd                   string      `json:"cwd"`
	MCPServers            []MCPServer `json:"mcpServers,omitempty"`
	AdditionalDirectories []string    `json:"additionalDirectories,omitempty"`
}

type SessionNewResult struct {
	SessionId SessionId `json:"sessionId"`
	Mode      string    `json:"mode,omitempty"`
}

type MCPServer struct {
	Name    string       `json:"name"`
	Command string       `json:"command,omitempty"`
	Args    []string     `json:"args,omitempty"`
	Env     []EnvVar     `json:"env,omitempty"`
	Type    string       `json:"type,omitempty"`
	URL     string       `json:"url,omitempty"`
	Headers []HttpHeader `json:"headers,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HttpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SessionLoadParams struct {
	SessionId             SessionId   `json:"sessionId"`
	Cwd                   string      `json:"cwd"`
	MCPServers            []MCPServer `json:"mcpServers,omitempty"`
	AdditionalDirectories []string    `json:"additionalDirectories,omitempty"`
}

type SessionResumeParams struct {
	SessionId             SessionId   `json:"sessionId"`
	Cwd                   string      `json:"cwd"`
	MCPServers            []MCPServer `json:"mcpServers,omitempty"`
	AdditionalDirectories []string    `json:"additionalDirectories,omitempty"`
}

type SessionDeleteParams struct {
	SessionId SessionId `json:"sessionId"`
}

type SessionCloseParams struct {
	SessionId SessionId `json:"sessionId"`
}

type SessionListParams struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type SessionListResult struct {
	Sessions   []SessionListItem `json:"sessions"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

type SessionListItem struct {
	ID        SessionId `json:"id"`
	Title     string    `json:"title"`
	CreatedAt string    `json:"createdAt"`
	UpdatedAt string    `json:"updatedAt"`
}

func (s *Server) handleSessionNew(ctx context.Context, params json.RawMessage) (any, error) {
	var req SessionNewParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/new params"}
	}
	if req.Cwd == "" {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "cwd is required"}
	}

	orch, err := s.newAgent()
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInternal, Message: "failed to create agent: " + err.Error()}
	}

	acpID := uuid.NewString()
	acpSess := &ACPSession{
		ID:        acpID,
		Agent:     orch,
		CreatedAt: time.Now(),
		Cwd:       req.Cwd,
	}

	provName := ""
	if orch.Provider != nil {
		provName = orch.Provider.Name()
	}
	vs, err := s.sessions.Create(session.CreateOptions{
		Title:      "ACP Session",
		Provider:   provName,
		Model:      orch.Model.ID,
		ProjectDir: req.Cwd,
	})
	if err != nil {
		s.log.Fdebug("acp: create pegg session: %v", err)
	} else {
		acpSess.PeggSessionID = vs.ID
		s.bus.Publish(session.TopicSessionResume, session.SessionResume{
			AgentID:   orch.ID,
			SessionID: vs.ID,
		})
	}

	s.addSession(acpSess)
	return SessionNewResult{SessionId: SessionId(acpID)}, nil
}

func (s *Server) handleSessionLoad(ctx context.Context, params json.RawMessage) (any, error) {
	var req SessionLoadParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/load params"}
	}

	vs, err := s.sessions.Get(string(req.SessionId))
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "session not found"}
	}

	orch, err := s.newAgent()
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInternal, Message: "failed to create agent: " + err.Error()}
	}

	msgs, err := s.sessions.Messages(vs.ID)
	if err == nil {
		for _, msg := range msgs {
			updateType := "agent_message_chunk"
			if msg.Role == llm.RoleUser {
				updateType = "user_message_chunk"
			}
			content := extractContent(msg)
			if content != nil {
				s.notify(string(req.SessionId), SessionUpdate{
					SessionUpdate: updateType,
					MessageID:     msg.ID,
					Content:       content,
				})
			}
		}
	}

	acpSess := &ACPSession{
		ID:            string(req.SessionId),
		Agent:         orch,
		CreatedAt:     time.Now(),
		Cwd:           req.Cwd,
		PeggSessionID: vs.ID,
	}
	s.addSession(acpSess)

	s.bus.Publish(session.TopicSessionResume, session.SessionResume{
		AgentID:   orch.ID,
		SessionID: vs.ID,
	})

	return nil, nil
}

func (s *Server) handleSessionResume(ctx context.Context, params json.RawMessage) (any, error) {
	var req SessionResumeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/resume params"}
	}

	vs, err := s.sessions.Get(string(req.SessionId))
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "session not found"}
	}

	orch, err := s.newAgent()
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInternal, Message: "failed to create agent: " + err.Error()}
	}

	acpSess := &ACPSession{
		ID:            string(req.SessionId),
		Agent:         orch,
		CreatedAt:     time.Now(),
		Cwd:           req.Cwd,
		PeggSessionID: vs.ID,
	}
	s.addSession(acpSess)

	s.bus.Publish(session.TopicSessionResume, session.SessionResume{
		AgentID:   orch.ID,
		SessionID: vs.ID,
	})

	return map[string]any{}, nil
}

func (s *Server) handleSessionDelete(ctx context.Context, params json.RawMessage) (any, error) {
	var req SessionDeleteParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/delete params"}
	}
	if err := s.sessions.Delete(string(req.SessionId)); err != nil {
		return nil, &RPCError{Code: ErrCodeInternal, Message: "failed to delete session"}
	}
	return nil, nil
}

func (s *Server) handleSessionClose(ctx context.Context, params json.RawMessage) (any, error) {
	var req SessionCloseParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid session/close params"}
	}
	s.removeSession(string(req.SessionId))
	return map[string]any{}, nil
}

func (s *Server) handleSessionList(ctx context.Context, params json.RawMessage) (any, error) {
	q := query.Query{
		Page: query.Page{Number: 1, Size: 50},
		Sort: []query.Sort{{Column: "updated_at", Dir: query.Desc}},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	}

	sessions, total, err := s.sessions.List(q)
	if err != nil {
		return nil, &RPCError{Code: ErrCodeInternal, Message: "failed to list sessions"}
	}

	items := make([]SessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, SessionListItem{
			ID:        SessionId(sess.ID),
			Title:     sess.Title,
			CreatedAt: sess.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: sess.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return SessionListResult{
		Sessions: items,
		NextCursor: func() string {
			if len(items) >= total && total > 0 {
				return ""
			}
			return ""
		}(),
	}, nil
}

func (s *Server) newAgent() (*agent.Agent, error) {
	return s.newAgentFn()
}

func (s *Server) newOrchestratorAgent() (*agent.Agent, error) {
	orch, err := agents.New("orchestrator")
	if err != nil {
		return nil, fmt.Errorf("create orchestrator: %w", err)
	}
	orch.Bus = s.bus

	prov, mdl, err := s.resolveModel("", "")
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}
	orch.SetModelProvider(mdl, prov)
	return orch, nil
}

func extractContent(msg session.Message) *ContentBlock {
	switch c := msg.Content.(type) {
	case string:
		return &ContentBlock{Type: "text", Text: c}
	}
	return nil
}
