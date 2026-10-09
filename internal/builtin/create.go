package builtin

import (
	"github.com/peggco/pegg/internal/builtin/agents"
	"github.com/peggco/pegg/internal/builtin/middlewares"
	"github.com/peggco/pegg/internal/builtin/reminders"
	"github.com/peggco/pegg/internal/builtin/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

type Options struct {
	LLM      *llm.Manager
	Decision *decision.Manager
	Config   *config.Config
	Bus      event.Bus
	Memory   *memory.Manager
}

func Create(fs *vfs.VFS, sess *session.Manager, opts Options) error {
	agents.Create(fs)
	middlewares.Create(fs, middlewares.Deps{
		Config:   opts.Config,
		LLM:      opts.LLM,
		Decision: opts.Decision,
		Sessions: sess,
	})
	tools.Create(fs, sess)
	if opts.Memory != nil {
		if err := opts.Memory.Start(); err != nil {
			return err
		}
		if err := memory.RegisterTools(opts.Memory); err != nil {
			return err
		}
	}
	if err := reminders.Create(opts.Bus); err != nil {
		return err
	}
	return nil
}
