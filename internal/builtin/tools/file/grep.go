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

func generateGrepToolPrompt() (string, error) {
	sys, err := grepToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

type stringOrArray []string

func (s *stringOrArray) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = stringOrArray{str}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*s = stringOrArray(arr)
	return nil
}

func grepTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateGrepToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate grep tool prompt: %v", err))
	}

	return tool.NewSpec(
		"grep",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regex pattern to search for in file contents.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The directory to search in. Defaults to the current working directory.",
				},
				"include": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "File pattern to include in the search (e.g. '*.js', '*.{ts,tsx}')",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"content", "files_with_matches", "count"},
					"description": "Output mode: 'content' (default) returns matching lines with line numbers, 'files_with_matches' returns only file paths, 'count' returns match count per file.",
				},
			},
			"required": []string{"pattern"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Pattern string        `json:"pattern"`
				Path    string        `json:"path"`
				Include stringOrArray `json:"include"`
				Mode    string        `json:"mode"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("grep: invalid arguments: %w", err)
			}
			if params.Pattern == "" {
				return "", fmt.Errorf("grep: pattern is required")
			}

			mode := vfs.GrepMode(params.Mode)
			if mode == "" {
				mode = vfs.GrepModeContent
			}

			results, err := fs.GrepCtx(ctx, params.Pattern, params.Path, []string(params.Include), mode, 100)
			if err != nil {
				return "", fmt.Errorf("grep: %w", err)
			}

			if len(results) == 0 {
				return "No matches found.\n", nil
			}

			return formatGrepResults(results, mode), nil
		},
	).SetPermissionError(isScopeError)
}

func formatGrepResults(results []vfs.GrepResult, mode vfs.GrepMode) string {
	var b strings.Builder
	const limit = 100
	truncated := len(results) > limit

	if mode == vfs.GrepModeFilesWithMatches {
		fmt.Fprintf(&b, "Matches: %d files\n", len(results))
		shown := results
		if truncated {
			shown = results[:limit]
		}
		for _, r := range shown {
			fmt.Fprintf(&b, "  %s\n", r.Path)
		}
	} else if mode == vfs.GrepModeCount {
		total := 0
		for _, r := range results {
			total += r.Count
		}
		fmt.Fprintf(&b, "Matches: %d files, %d total matches\n", len(results), total)
		shown := results
		if truncated {
			shown = results[:limit]
		}
		for _, r := range shown {
			fmt.Fprintf(&b, "  %5d  %s\n", r.Count, r.Path)
		}
	} else {
		fmt.Fprintf(&b, "Matches: %d\n", len(results))
		shown := results
		if truncated {
			shown = results[:limit]
		}
		for _, r := range shown {
			fmt.Fprintf(&b, "  %s:%d: %s\n", r.Path, r.Line, r.Text)
		}
	}
	if truncated {
		fmt.Fprintf(&b, "\n(Results are truncated. Consider using a more specific path or pattern)\n")
	}
	return b.String()
}
