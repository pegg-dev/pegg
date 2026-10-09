package tools

import (
	"github.com/peggco/pegg/internal/builtin/tools/ask"
	"github.com/peggco/pegg/internal/builtin/tools/file"
	"github.com/peggco/pegg/internal/builtin/tools/loadskill"
	"github.com/peggco/pegg/internal/builtin/tools/plan"
	"github.com/peggco/pegg/internal/builtin/tools/shell"
	"github.com/peggco/pegg/internal/builtin/tools/subagent"
	"github.com/peggco/pegg/internal/builtin/tools/todo"
	"github.com/peggco/pegg/internal/builtin/tools/web"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/vfs"
)

func Create(fs *vfs.VFS, sess *session.Manager) {
	file.FileTools(fs)
	todo.TodoTools(fs, sess.Bus())
	shell.ShellTools(fs)
	web.WebTools(fs)
	subagent.SubAgentTools(sess)
	ask.AskTool()
	loadskill.LoadSkillTool(sess)
	plan.PlanTools(fs)
}
