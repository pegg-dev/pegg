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

func generateReadToolPrompt() (string, error) {
	sys, err := readToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func readTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateReadToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate read tool prompt: %v", err))
	}

	return tool.NewSpec(
		"read",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath": map[string]any{
					"type":        "string",
					"description": "The path to the file to read.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "The line number to start reading from (1-based)",
					"minimum":     1,
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "The number of lines to read (defaults to 2000)",
					"minimum":     0,
				},
			},
			"required": []string{"filePath"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				FilePath string `json:"filePath"`
				Offset   int    `json:"offset"`
				Limit    int    `json:"limit"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("read: invalid arguments: %w", err)
			}
			if params.FilePath == "" {
				return "", fmt.Errorf("read: filePath is required")
			}

			fi, err := fs.Stat(params.FilePath)
			if err != nil {
				return "", fmt.Errorf("read: %w", err)
			}

			if fi.IsDir {
				result, err := fs.ListRecursiveIgnoreCtx(ctx, params.FilePath, nil)
				if err != nil {
					return "", fmt.Errorf("read: %w", err)
				}
				return formatListResult(result), nil
			}

			offset := params.Offset
			if offset < 1 {
				offset = 1
			}
			limit := params.Limit
			if limit <= 0 {
				limit = 2000
			}

			result, err := fs.ReadRangeCtx(ctx, params.FilePath, offset, limit)
			if err != nil {
				return "", fmt.Errorf("read: %w", err)
			}

			return formatReadOutput(result, offset, limit), nil
		},
	).SetPermissionError(isScopeError)
}

func formatReadOutput(vfsOutput string, offset, limit int) string {
	lines := strings.Split(vfsOutput, "\n")

	totalLines := 0
	contentStart := 0

	for i, line := range lines {
		if idx := strings.Index(line, "Lines: "); idx >= 0 {
			fmt.Sscanf(line[idx:], "Lines: %d", &totalLines)
		}
		if line == "---" {
			contentStart = i + 1
			break
		}
	}

	contentLines := lines[contentStart:]
	for len(contentLines) > 0 && contentLines[len(contentLines)-1] == "" {
		contentLines = contentLines[:len(contentLines)-1]
	}

	var b strings.Builder
	b.WriteString("<file>\n")

	for _, line := range contentLines {
		b.WriteString(line)
		b.WriteString("\n")
	}

	shown := len(contentLines)
	endLine := offset + shown - 1
	if totalLines > 0 && endLine < totalLines {
		b.WriteString(fmt.Sprintf("\n(File has more lines. Use 'offset' parameter to read beyond line %d)\n", endLine))
	}

	b.WriteString("</file>")
	return b.String()
}
