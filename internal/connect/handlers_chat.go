package connect

import "context"

func (c *Connector) registerChatMethods() {
	c.dispatch.register("chat.send", ScopeChat, c.handleChatSend)
	c.dispatch.register("run.cancel", ScopeChat, c.handleRunCancel)
	c.dispatch.register("run.answer", ScopeChat, c.handleRunAnswer)
	c.dispatch.register("run.list", ScopeRead, c.handleRunList)
}

type chatSendParams struct {
	Message         string   `json:"message"`
	SessionID       string   `json:"session_id"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoning_effort"`
	Files           []string `json:"files"`
}

func (c *Connector) handleChatSend(_ context.Context, req *Request) (any, error) {
	var p chatSendParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.SessionID == "" {
		p.SessionID = req.SessionID
	}
	info, err := c.runs.Start(StartParams{
		Message:         p.Message,
		SessionID:       p.SessionID,
		Provider:        p.Provider,
		Model:           p.Model,
		ReasoningEffort: p.ReasoningEffort,
		Files:           p.Files,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"run_id":     info.ID,
		"session_id": info.SessionID,
		"state":      info.State,
	}, nil
}

type runRefParams struct {
	RunID string `json:"run_id"`
}

func (c *Connector) handleRunCancel(_ context.Context, req *Request) (any, error) {
	var p runRefParams
	_ = req.Decode(&p)
	id := p.RunID
	if id == "" {
		id = req.RunID
	}
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "run_id is required"}
	}
	if err := c.runs.Cancel(id); err != nil {
		return nil, err
	}
	return map[string]any{"run_id": id, "state": runStateCanceled}, nil
}

type runAnswerParams struct {
	RunID   string            `json:"run_id"`
	Answers map[string]string `json:"answers"`
}

func (c *Connector) handleRunAnswer(_ context.Context, req *Request) (any, error) {
	var p runAnswerParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	id := p.RunID
	if id == "" {
		id = req.RunID
	}
	if id == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "run_id is required"}
	}
	if len(p.Answers) == 0 {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "answers are required"}
	}
	if err := c.runs.Answer(id, p.Answers); err != nil {
		return nil, err
	}
	Audit("run.answer", map[string]any{"run_id": id})
	return map[string]any{"run_id": id, "answered": true}, nil
}

func (c *Connector) handleRunList(_ context.Context, _ *Request) (any, error) {
	return map[string]any{"runs": c.runs.List()}, nil
}
