package middleware

import (
	"context"

	"github.com/peggco/pegg/internal/llm"
)

type Middleware interface {
	BeforeRun(ctx context.Context, agentName, input string) error
	AfterRun(ctx context.Context, agentName string, result *Result, runErr error) error
	BeforeLLM(ctx context.Context, req *llm.Request) error
	AfterLLM(ctx context.Context, req *llm.Request, resp *llm.Response) error
	BeforeTool(ctx context.Context, call llm.ToolCall) error
	AfterTool(ctx context.Context, call llm.ToolCall, output string, toolErr error) error
	OnError(ctx context.Context, runErr error) error
}

type LLMInvoker func(ctx context.Context, req *llm.Request) (*llm.Response, error)

type LLMStreamInvoker func(ctx context.Context, req *llm.Request, handler llm.StreamHandler) error

type InvokeLLM interface {
	InvokeLLM(ctx context.Context, req *llm.Request, next LLMInvoker) (*llm.Response, error)
}

type InvokeLLMStream interface {
	InvokeLLMStream(ctx context.Context, req *llm.Request, handler llm.StreamHandler, next LLMStreamInvoker) error
}

type ToolInvoker func(ctx context.Context, call llm.ToolCall) (string, error)

type InvokeTool interface {
	InvokeTool(ctx context.Context, call llm.ToolCall, next ToolInvoker) (string, error)
}

type BaseMiddleware struct{}

func (BaseMiddleware) BeforeRun(context.Context, string, string) error              { return nil }
func (BaseMiddleware) AfterRun(context.Context, string, *Result, error) error       { return nil }
func (BaseMiddleware) BeforeLLM(context.Context, *llm.Request) error                { return nil }
func (BaseMiddleware) AfterLLM(context.Context, *llm.Request, *llm.Response) error  { return nil }
func (BaseMiddleware) BeforeTool(context.Context, llm.ToolCall) error               { return nil }
func (BaseMiddleware) AfterTool(context.Context, llm.ToolCall, string, error) error { return nil }
func (BaseMiddleware) OnError(context.Context, error) error                         { return nil }
