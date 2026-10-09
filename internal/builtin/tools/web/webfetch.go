package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
)

func generateWebfetchToolPrompt() (string, error) {
	sys, err := webfetchToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func webfetchTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateWebfetchToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate fetch tool prompt: %v", err))
	}

	return tool.NewSpec(
		"webfetch",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch content from",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"text", "markdown", "html"},
					"description": "The format to return the content in (text, markdown, or html, default markdown)",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (max 120, default 30)",
					"minimum":     1,
					"maximum":     120,
				},
			},
			"required": []string{"url"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				URL     string `json:"url"`
				Format  string `json:"format"`
				Timeout int    `json:"timeout"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("webfetch: invalid arguments: %w", err)
			}
			if params.URL == "" {
				return "", fmt.Errorf("webfetch: url is required")
			}

			timeout := 30 * time.Second
			if params.Timeout > 0 {
				if params.Timeout > 120 {
					params.Timeout = 120
				}
				timeout = time.Duration(params.Timeout) * time.Second
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", params.URL, nil)
			if err != nil {
				return "", fmt.Errorf("webfetch: create request: %w", err)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Pegg/1.0)")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("webfetch: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
			if err != nil {
				return "", fmt.Errorf("webfetch: read body: %w", err)
			}

			contentType := resp.Header.Get("Content-Type")
			if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
				contentType = strings.TrimSpace(contentType[:idx])
			}

			out := fmt.Sprintf("URL: %s\nStatus: %d\nContent-Type: %s\nSize: %d bytes\n\n",
				params.URL, resp.StatusCode, contentType, len(body))

			if !strings.HasPrefix(contentType, "text/") && !strings.Contains(contentType, "html") {
				out += "(binary content, not displayed)\n"
				return out, nil
			}

			format := params.Format
			if format == "" {
				format = "markdown"
			}

			switch format {
			case "html":
				out += string(body)
			case "text":
				if strings.Contains(contentType, "html") || isHTML(body) {
					markdown, err := md.ConvertString(string(body))
					if err != nil {
						return "", fmt.Errorf("webfetch: convert to markdown: %w", err)
					}
					out += stripMarkdownFormatting(markdown)
				} else {
					out += string(body)
				}
			default:
				if strings.Contains(contentType, "html") || isHTML(body) {
					markdown, err := md.ConvertString(string(body))
					if err != nil {
						return "", fmt.Errorf("webfetch: convert to markdown: %w", err)
					}
					out += markdown
				} else {
					out += string(body)
				}
			}

			return out, nil
		},
	)
}

func stripMarkdownFormatting(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "```", "")
	return s
}

func isHTML(data []byte) bool {
	check := string(data)
	if len(check) > 1024 {
		check = check[:1024]
	}
	lower := strings.ToLower(check)
	return strings.Contains(lower, "<!doctype html") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") ||
		strings.Contains(lower, "<body")
}
