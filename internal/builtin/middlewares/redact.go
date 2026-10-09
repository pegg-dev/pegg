package middlewares

import (
	"context"

	agentmw "github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/llm"
)

type Redaction struct {
	agentmw.BaseMiddleware
	red *redactor
}

func NewRedaction(opts ...RedactionOption) *Redaction {
	return &Redaction{red: newRedactor(opts...)}
}

func (r *Redaction) RedactString(s string) string {
	return r.red.Redact(s)
}

func (r *Redaction) BeforeLLM(_ context.Context, req *llm.Request) error {
	if req == nil {
		return nil
	}
	for i := range req.Messages {
		if req.Messages[i].Role == llm.RoleAssistant {
			continue
		}
		r.red.redactMessage(&req.Messages[i], false)
	}
	return nil
}
