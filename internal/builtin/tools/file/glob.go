package file

import (
	"context"
	"fmt"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateGlobToolPrompt() (string, error) {
	sys, err := globToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func globTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateGlobToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate glob tool prompt: %v", err))
	}

	return tool.NewSpec(
		"glob",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The glob pattern to match files against.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The directory to search in. If not specified, the current working directory will be used. IMPORTANT: Omit this field to use the default directory. DO NOT enter 'undefined' or 'null' - simply omit it for the default behavior. Must be a valid directory path if provided.",
				},
			},
			"required": []string{"pattern"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("glob: invalid arguments: %w", err)
			}
			if params.Pattern == "" {
				return "", fmt.Errorf("glob: pattern is required")
			}

			results, err := fs.GlobCtx(ctx, params.Pattern, params.Path)
			if err != nil {
				return "", fmt.Errorf("glob: %w", err)
			}

			if len(results) == 0 {
				return "No files found.\n", nil
			}

			limit := 100
			truncated := false
			if len(results) > limit {
				results = results[:limit]
				truncated = true
			}

			var b strings.Builder
			for i, r := range results {
				if i > 0 {
					b.WriteString("\n")
				}
				b.WriteString(r)
			}

			if truncated {
				b.WriteString("\n\n(Results are truncated. Consider using a more specific path or pattern.)")
			}

			return b.String(), nil
		},
	).SetPermissionError(isScopeError)
}
