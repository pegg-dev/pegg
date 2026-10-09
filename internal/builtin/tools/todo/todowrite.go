package todo

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateTodowriteToolPrompt() (string, error) {
	sys, err := todowriteToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func todowriteTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateTodowriteToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate update todo tool prompt: %v", err))
	}

	return tool.NewSpec(
		"todowrite",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "The complete todo list to set",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "Unique identifier for the todo item",
							},
							"title": map[string]any{
								"type":        "string",
								"description": "Title of the task",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "Detailed description of the task",
							},
							"status": map[string]any{
								"type":        "string",
								"description": "Current status: pending, in_progress, completed, cancelled",
							},
							"priority": map[string]any{
								"type":        "string",
								"description": "Priority level: high, medium, low",
							},
							"dependsOn": map[string]any{
								"type":        "array",
								"description": "IDs of tasks this depends on",
								"items": map[string]any{
									"type": "string",
								},
							},
						},
						"required": []string{"id", "title", "status", "priority"},
					},
				},
			},
			"required": []string{"todos"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Todos []*Todo `json:"todos"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("todowrite: invalid arguments: %w", err)
			}

			if err := store.setAll(ctx, params.Todos); err != nil {
				return "", fmt.Errorf("todowrite: %w", err)
			}
			all, err := store.all(ctx)
			if err != nil {
				return "", fmt.Errorf("todowrite: %w", err)
			}
			return formatTodoList(all), nil
		},
	)
}
