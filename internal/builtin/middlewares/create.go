package middlewares

import (
	"github.com/peggco/pegg/internal/agent/middlewares"
	"github.com/peggco/pegg/internal/builtin/middlewares/compaction"
	"github.com/peggco/pegg/internal/builtin/middlewares/permission"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Deps struct {
	Config   *config.Config
	LLM      *llm.Manager
	Decision *decision.Manager
	Sessions *session.Manager
}

func Create(fs *vfs.VFS, deps Deps) {
	middlewares.Register("loop-detector", NewLoopDetector())
	middlewares.Register("redaction", NewRedaction())
	middlewares.Register("retry", NewRetry())
	var permCfg *config.PermissionConfig
	if deps.Config != nil {
		permCfg = deps.Config.Permission
	}
	perm := permission.New(permission.Deps{
		Config:   permCfg,
		LLM:      deps.LLM,
		Decision: deps.Decision,
	})
	middlewares.Register("permission", perm)
	if fs != nil {
		fs.OnAccessCheck(perm.AccessChecker)
	}
	var compCfg *config.CompactionConfig
	if deps.Config != nil {
		compCfg = deps.Config.Compaction
	}
	middlewares.Register("compaction", compaction.New(compaction.Deps{
		Config: compCfg,
		LLM:    deps.LLM,
	}))
}
