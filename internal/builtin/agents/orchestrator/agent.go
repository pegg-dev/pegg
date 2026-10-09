package orchestrator

import (
	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	_ "github.com/peggco/pegg/internal/builtin/middlewares"
	"github.com/peggco/pegg/internal/builtin/tools/file"
	"github.com/peggco/pegg/internal/vfs"
)

func Register(fs *vfs.VFS) {
	agents.Register(func() (*agent.Agent, error) {
		return newOrchestratorAgent(fs)
	})
}

func newOrchestratorAgent(fs *vfs.VFS) (*agent.Agent, error) {
	main := agent.New("orchestrator",
		agent.WithSystemPromptFn(func(providerID, modelID string) string {
			sys, err := generateOrchestratorPrompt(providerID, modelID)
			if err != nil {
				return ""
			}
			return sys
		}),
		agent.WithTools(file.Tools(fs)...),
		agent.WithToolNames("askuserquestion", "bash", "task", "taskstatus", "todoread", "todowrite", "webfetch", "websearch", "loadskill", "enterplanmode", "exitplanmode", "mem-search", "mem-read"),
		agent.WithMiddlewareNames("loop-detector", "redaction", "retry", "permission", "compaction"),
	)

	return main, nil
}
