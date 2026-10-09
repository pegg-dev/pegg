package subagent

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
)

func generateTaskStatusToolPrompt() (string, error) {
	sys, err := taskstatusToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func subAgentsStatusTool() tool.Tool {
	prompt, err := generateTaskStatusToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate taskstatus tool prompt: %v", err))
	}

	return tool.NewSpec(
		"taskstatus",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_names": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional names of subagents to show. Omit to show all.",
				},
			},
			"required": []string{},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				AgentNames []string `json:"agent_names"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("taskstatus: invalid arguments: %w", err)
			}

			parent := agent.FromContext(ctx)
			if len(params.AgentNames) == 0 {
				return "Subagents:\n" + formatStatuses(store.all(parent)), nil
			}
			agents, err := store.filter(params.AgentNames, parent)
			if err != nil {
				return "", fmt.Errorf("taskstatus: %w", err)
			}
			return "Subagents:\n" + formatStatuses(agents), nil
		},
	)
}
