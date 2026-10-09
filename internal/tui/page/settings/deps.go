package settings

import (
	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/plugin"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Deps struct {
	Config   *config.Config
	LLM      *llm.Manager
	Decision *decision.Manager
	Memory   *memory.Manager
	MCP      *mcp.Manager
	Sessions *session.Manager
	Agent    *agent.Agent
	Bus      event.Bus
	VFS      *vfs.VFS
	Cache    cache.Cache
	Plugin   *plugin.Manager
}
