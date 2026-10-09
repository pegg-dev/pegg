package plan

import (
	"context"
	"fmt"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

const (
	approvalQuestionID = "plan_approval"
	approvalApprove    = "Yes, approve and exit plan mode"
	approvalReject     = "No, keep planning"
)

func exitplanmodeToolPrompt() (string, error) {
	sys, err := exitplanmodeToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func exitplanmodeTool(fs *vfs.VFS) tool.Tool {
	prompt, err := exitplanmodeToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate exitplanmode tool prompt: %v", err))
	}

	return tool.NewSpec(
		"exitplanmode",
		prompt,
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		func(ctx context.Context, args string) (string, error) {
			parent := agent.FromContext(ctx)
			if parent == nil {
				return "", fmt.Errorf("exitplanmode: no parent agent in context")
			}
			parent.DetachReminder()
			if fs != nil {
				fs.ClearWriteOnly()
			}
			return "Plan mode disabled. You may now make changes and execute the approved plan.", nil
		},
	)
}
