package sdk

import (
	"context"

	"github.com/peggco/pegg/internal/builtin/middlewares/permission"
	"github.com/peggco/pegg/internal/schedule"
)

type engineExecutor struct {
	engine *Engine
}

func (x engineExecutor) Execute(ctx context.Context, req schedule.ExecuteRequest) schedule.ExecuteResult {
	if x.engine == nil {
		return schedule.ExecuteResult{ExitCode: -1, Output: "sdk: engine unavailable"}
	}

	chatReq := ChatRequest{
		Input:     req.Schedule.Prompt,
		SessionID: req.SessionID,
	}
	if s, err := x.engine.sessions.Get(req.SessionID); err == nil {
		chatReq.Provider = s.Provider
		chatReq.Model = s.Model
	}
	if timeout := req.Schedule.TimeoutValue(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	resp, err := x.engine.Chat(permission.WithAutoApprove(ctx), chatReq)
	res := schedule.ExecuteResult{Usage: resp.Usage}
	if err != nil {
		res.ExitCode = 1
		res.Output = err.Error()
	}
	return res
}
