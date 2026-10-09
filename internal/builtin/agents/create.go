package agents

import (
	"github.com/peggco/pegg/internal/builtin/agents/developer"
	"github.com/peggco/pegg/internal/builtin/agents/explorer"
	"github.com/peggco/pegg/internal/builtin/agents/orchestrator"
	"github.com/peggco/pegg/internal/builtin/agents/planner"
	"github.com/peggco/pegg/internal/vfs"
)

func Create(fs *vfs.VFS) {
	explorer.Register()
	planner.Register(fs)
	developer.Register(fs)
	orchestrator.Register(fs)
}
