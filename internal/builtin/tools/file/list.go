package file

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateListToolPrompt() (string, error) {
	sys, err := listToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func listTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateListToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate list tool prompt: %v", err))
	}

	return tool.NewSpec(
		"list",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the directory to list. Omit or set to empty string to list the workspace root. Examples: 'src', 'internal/config', 'docs'.",
				},
				"ignore": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of glob patterns to ignore.",
				},
			},
			"required": []string{},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Path   string   `json:"path"`
				Ignore []string `json:"ignore"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("list: invalid arguments: %w", err)
			}

			dir := params.Path
			if dir == "" {
				dir = "."
			}

			result, err := fs.ListRecursiveIgnoreCtx(ctx, dir, params.Ignore)
			if err != nil {
				return "", fmt.Errorf("list: %w", err)
			}

			return formatListResult(result), nil
		},
	).SetPermissionError(isScopeError)
}

func formatListResult(r vfs.ListResult) string {
	dir := r.Path
	if dir == "" {
		dir = "."
	}

	out := fmt.Sprintf("Directory: %s\n", dir)
	out += fmt.Sprintf("Files: %d | Directories: %d | Total size: %d bytes\n", r.FileCount, r.DirCount, r.TotalSize)

	if len(r.Entries) == 0 {
		out += "(empty)\n"
		return out
	}
	out += "---\n"

	dirs := make(map[string]bool)
	filesByDir := make(map[string][]string)

	for _, e := range r.Entries {
		p := filepath.ToSlash(e.Path)
		p = path.Clean(p)

		if e.IsDir {
			parts := strings.Split(p, "/")
			for i := 0; i <= len(parts); i++ {
				d := "."
				if i > 0 {
					d = strings.Join(parts[:i], "/")
				}
				dirs[d] = true
			}
		} else {
			dirName := path.Dir(p)
			if dirName == "" {
				dirName = "."
			}

			parts := strings.Split(dirName, "/")
			if dirName == "." {
				parts = nil
			}

			for i := 0; i <= len(parts); i++ {
				d := "."
				if i > 0 {
					d = strings.Join(parts[:i], "/")
				}
				dirs[d] = true
			}

			filesByDir[dirName] = append(filesByDir[dirName], path.Base(p))
		}
	}

	var renderDir func(dirPath string, depth int) string
	renderDir = func(dirPath string, depth int) string {
		indent := strings.Repeat("  ", depth)
		var output string

		if depth > 0 {
			output += fmt.Sprintf("%s%s/\n", indent, path.Base(dirPath))
		}

		childIndent := strings.Repeat("  ", depth+1)

		var children []string
		for d := range dirs {
			if path.Dir(d) == dirPath && d != dirPath {
				children = append(children, d)
			}
		}
		sort.Strings(children)

		for _, child := range children {
			output += renderDir(child, depth+1)
		}

		files := filesByDir[dirPath]
		sort.Strings(files)
		for _, f := range files {
			output += fmt.Sprintf("%s%s\n", childIndent, f)
		}

		return output
	}

	out += fmt.Sprintf("%s/\n", dir)
	out += renderDir(".", 0)

	return out
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%5d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%5.1fKB", float64(size)/1024)
	} else {
		return fmt.Sprintf("%5.1fMB", float64(size)/(1024*1024))
	}
}
