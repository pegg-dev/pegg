package file

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateEditToolPrompt() (string, error) {
	sys, err := editToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func editTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateEditToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate edit tool prompt: %v", err))
	}

	return tool.NewSpec(
		"edit",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to modify.",
				},
				"oldString": map[string]any{
					"type":        "string",
					"description": "The text to replace.",
				},
				"newString": map[string]any{
					"type":        "string",
					"description": "The text to replace it with (must be different from oldString).",
				},
				"replaceAll": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences of oldString (default false).",
				},
			},
			"required": []string{"filePath", "oldString", "newString"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				FilePath   string `json:"filePath"`
				OldString  string `json:"oldString"`
				NewString  string `json:"newString"`
				ReplaceAll bool   `json:"replaceAll"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("edit: invalid arguments: %w", err)
			}
			if params.FilePath == "" {
				return "", fmt.Errorf("edit: filePath is required")
			}
			if params.OldString == "" {
				return "", fmt.Errorf("edit: oldString is required")
			}

			result, err := fs.EditCtx(ctx, params.FilePath, params.OldString, params.NewString, params.ReplaceAll)
			if err != nil {
				return "", fmt.Errorf("edit: %w", err)
			}

			return result, nil
		},
	).SetPermissionError(isScopeError)
}
