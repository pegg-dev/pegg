package developer

import (
	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	_ "github.com/peggco/pegg/internal/builtin/middlewares"
	"github.com/peggco/pegg/internal/builtin/tools/file"
	"github.com/peggco/pegg/internal/vfs"
)

func Register(fs *vfs.VFS) {
	agents.Register(func() (*agent.Agent, error) {
		return newDeveloperAgent(fs)
	})
}

func newDeveloperAgent(fs *vfs.VFS) (*agent.Agent, error) {
	main := agent.New("developer",
		agent.WithSystemPromptFn(func(providerID, modelID string) string {
			sys, err := generateDeveloperPrompt(providerID, modelID)
			if err != nil {
				return ""
			}
			return sys
		}),
		agent.WithTools(file.Tools(fs)...),
		agent.WithToolNames("bash", "webfetch", "websearch", "todoread", "todowrite", "mem-search", "mem-read"),
		agent.WithMiddlewareNames("loop-detector", "redaction", "retry", "permission", "compaction"),
	)

	return main, nil
}
