package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateBashToolPrompt() (string, error) {
	sys, err := bashToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func bashTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateBashToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate bash tool prompt: %v", err))
	}

	return tool.NewSpec(
		"bash",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command to execute.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds. Defaults to 40. Use a larger value for long-running commands (e.g. builds, test suites), a smaller one for quick checks.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Clear, concise description of what this command does in 5-10 words. Examples:\nInput: ls\nOutput: Lists files in current directory\n\nInput: git status\nOutput: Shows working tree status\n\nInput: npm install\nOutput: Installs package dependencies\n\nInput: mkdir foo\nOutput: Creates directory 'foo'",
				},
			},
			"required": []string{"command", "description"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Command     string `json:"command"`
				Timeout     int    `json:"timeout"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("bash: invalid arguments: %w", err)
			}
			if params.Command == "" {
				return "", fmt.Errorf("bash: command is required")
			}
			if params.Description == "" {
				return "", fmt.Errorf("bash: description is required")
			}
			timeout := 40 * time.Second
			if params.Timeout > 0 {
				timeout = time.Duration(params.Timeout) * time.Second
			}

			workdir := fs.Root()

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.Command("sh", "-c", params.Command)
			cmd.Dir = workdir
			setProcessGroup(cmd)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Start(); err != nil {
				return "", fmt.Errorf("bash: %w", err)
			}

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			var timedOut bool
			var err error
			select {
			case <-ctx.Done():
				timedOut = true
				killProcessGroup(cmd)
			case err = <-done:
			}

			if timedOut {
				return fmt.Sprintf("Command timed out after %s.\nstdout:\n%s\nstderr:\n%s\n", timeout, stdout.String(), stderr.String()), nil
			}

			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					return "", fmt.Errorf("bash: %w", err)
				}
			}

			result := fmt.Sprintf("Exit code: %d\n", exitCode)
			if stdout.Len() > 0 {
				result += fmt.Sprintf("stdout:\n%s\n", stdout.String())
			}
			if stderr.Len() > 0 {
				result += fmt.Sprintf("stderr:\n%s\n", stderr.String())
			}
			return result, nil
		},
	)
}
