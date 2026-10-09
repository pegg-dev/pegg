package connect

import "context"

type sessionRenameParams struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (c *Connector) handleSessionRename(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	var p sessionRenameParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	id := p.ID
	if id == "" {
		id = req.SessionID
	}
	if id == "" || p.Title == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "id and title are required"}
	}
	if err := c.deps.Sessions.SetTitle(id, p.Title); err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "rename session: " + err.Error()}
	}
	c.emit(eventSessionUpdate, id, "", map[string]any{"id": id, "title": p.Title})
	return map[string]any{"id": id, "title": p.Title}, nil
}

func (c *Connector) handleSessionDelete(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	id := c.sessionID(req)
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "session id is required"}
	}
	if err := c.deps.Sessions.Delete(id); err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "delete session: " + err.Error()}
	}
	c.emit(eventSessionUpdate, id, "", map[string]any{"id": id, "deleted": true})
	Audit("session.delete", map[string]any{"session_id": id})
	return map[string]any{"id": id, "deleted": true}, nil
}

func (c *Connector) handleSessionSnapshots(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	id := c.sessionID(req)
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "session id is required"}
	}
	snaps, err := c.deps.Sessions.Snapshots(id)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: "session not found"}
	}
	out := make([]map[string]any, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, map[string]any{
			"id":              s.ID,
			"session_id":      s.SessionID,
			"head_message_id": s.HeadMessageID,
			"created_at":      s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			"messages":        len(s.Messages),
		})
	}
	return map[string]any{"snapshots": out}, nil
}

type sessionRevertParams struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
}

func (c *Connector) handleSessionRevert(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	var p sessionRevertParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	id := p.ID
	if id == "" {
		id = req.SessionID
	}
	if id == "" || p.MessageID == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "id and message_id are required"}
	}
	snapshotID, err := c.deps.Sessions.Revert(id, p.MessageID)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "revert: " + err.Error()}
	}
	return map[string]any{"id": id, "snapshot_id": snapshotID}, nil
}

type sessionUndoRevertParams struct {
	ID         string `json:"id"`
	SnapshotID string `json:"snapshot_id"`
}

func (c *Connector) handleSessionUndoRevert(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	var p sessionUndoRevertParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	id := p.ID
	if id == "" {
		id = req.SessionID
	}
	if id == "" || p.SnapshotID == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "id and snapshot_id are required"}
	}
	if err := c.deps.Sessions.UndoRevert(id, p.SnapshotID); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "undo revert: " + err.Error()}
	}
	return map[string]any{"id": id, "restored": true}, nil
}

type sessionForkParams struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
}

func (c *Connector) handleSessionFork(_ context.Context, req *Request) (any, error) {
	if err := c.requireSessions(); err != nil {
		return nil, err
	}
	var p sessionForkParams
	_ = req.Decode(&p)
	id := p.ID
	if id == "" {
		id = req.SessionID
	}
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "session id is required"}
	}
	s, err := c.deps.Sessions.Fork(id, p.MessageID)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "fork: " + err.Error()}
	}
	return toSessionDTO(s), nil
}
