package agent

import (
	"context"

	"github.com/peggco/pegg/internal/llm"
)

type agentCtxKey struct{}

func WithAgent(ctx context.Context, a *Agent) context.Context {
	if a == nil {
		return ctx
	}
	return context.WithValue(ctx, agentCtxKey{}, a)
}

func FromContext(ctx context.Context) *Agent {
	a, _ := ctx.Value(agentCtxKey{}).(*Agent)
	return a
}

type historyCtxKey struct{}

func WithHistory(ctx context.Context, history *[]llm.Message) context.Context {
	if history == nil {
		return ctx
	}
	return context.WithValue(ctx, historyCtxKey{}, history)
}

func HistoryFrom(ctx context.Context) []llm.Message {
	if h, ok := ctx.Value(historyCtxKey{}).(*[]llm.Message); ok {
		return append([]llm.Message(nil), (*h)...)
	}
	return nil
}

func HistoryRefFrom(ctx context.Context) *[]llm.Message {
	if h, ok := ctx.Value(historyCtxKey{}).(*[]llm.Message); ok {
		return h
	}
	return nil
}

type streamCtxKey struct{}

func WithStream(ctx context.Context, handler StreamHandler) context.Context {
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, streamCtxKey{}, handler)
}

func StreamFrom(ctx context.Context) StreamHandler {
	h, _ := ctx.Value(streamCtxKey{}).(StreamHandler)
	return h
}
