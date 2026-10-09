package file

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateWriteToolPrompt() (string, error) {
	sys, err := writeToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func writeTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateWriteToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate write tool prompt: %v", err))
	}

	return tool.NewSpec(
		"write",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write to the file.",
				},
			},
			"required": []string{"filePath", "content"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				FilePath string `json:"filePath"`
				Content  string `json:"content"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("write: invalid arguments: %w", err)
			}
			if params.FilePath == "" {
				return "", fmt.Errorf("write: filePath is required")
			}

			result, err := fs.WriteCtx(ctx, params.FilePath, []byte(params.Content))
			if err != nil {
				return "", fmt.Errorf("write: %w", err)
			}

			return result, nil
		},
	).SetPermissionError(isScopeError)
}
