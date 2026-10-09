package todo

import (
	"context"
	"fmt"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateTodoreadToolPrompt() (string, error) {
	sys, err := todoreadToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func todoreadTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateTodoreadToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate list todo tool prompt: %v", err))
	}

	return tool.NewSpec(
		"todoread",
		prompt,
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		func(ctx context.Context, args string) (string, error) {
			all, err := store.all(ctx)
			if err != nil {
				return "", fmt.Errorf("todoread: %w", err)
			}
			result := formatTodoList(all)
			return result, nil
		},
	)
}
