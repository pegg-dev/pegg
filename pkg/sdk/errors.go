package sdk

import (
	"errors"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/session"
)

var (
	ErrAlreadyOpen   = errors.New("sdk: an engine is already open in this process")
	ErrNotOpen       = errors.New("sdk: engine is not open")
	ErrClosed        = errors.New("sdk: engine is closed")
	ErrNotConfigured = errors.New("sdk: no LLM providers configured")
	ErrNoModels      = errors.New("sdk: no models available for the requested provider")
	ErrNoSession     = errors.New("sdk: session not found")
	ErrEmptyInput    = errors.New("sdk: input must not be empty")
	ErrModelTimeout  = errors.New("sdk: timed out selecting a model")
)

var (
	ErrAgentEmptyInput     = agent.ErrEmptyInput
	ErrAgentMaxIterations  = agent.ErrMaxIterations
	ErrAgentNoProvider     = agent.ErrNoProvider
	ErrAgentToolExecFailed = agent.ErrToolExecutionFailed

	ErrToolNil       = tool.ErrNilTool
	ErrToolEmpty     = tool.ErrEmptyName
	ErrToolDuplicate = tool.ErrDuplicate
	ErrToolNotFound  = tool.ErrNotFound

	ErrMiddlewareNil       = middleware.ErrNilMiddleware
	ErrMiddlewareDuplicate = middleware.ErrDuplicate
	ErrMiddlewareNotFound  = middleware.ErrNotFound

	ErrSessionNotFound        = session.ErrNotFound
	ErrSessionDuplicate       = session.ErrDuplicate
	ErrSessionMessageNotFound = session.ErrMessageNotFound
	ErrSessionEmptyTitle      = session.ErrEmptyTitle

	ErrCacheNotFound = cache.ErrNotFound
)

func Is(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}
