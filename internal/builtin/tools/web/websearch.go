package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/vfs"
	"golang.org/x/net/html"
)

func generateWebsearchToolPrompt() (string, error) {
	sys, err := websearchToolPromptBuilder().
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}

func websearchTool(fs *vfs.VFS) tool.Tool {
	prompt, err := generateWebsearchToolPrompt()
	if err != nil {
		panic(fmt.Sprintf("failed to generate search tool prompt: %v", err))
	}

	return tool.NewSpec(
		"websearch",
		prompt,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query. Use natural language or keywords. Examples: 'Go language documentation', 'how to use context.WithTimeout', 'golang testing best practices'.",
				},
				"maxResults": map[string]any{
					"type":        "integer",
					"description": "Maximum number of search results to return (default 10, max 20).",
					"minimum":     1,
					"maximum":     20,
				},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, args string) (string, error) {
			var params struct {
				Query      string `json:"query"`
				MaxResults int    `json:"maxResults"`
			}
			if err := json.Unmarshal([]byte(args), &params); err != nil {
				return "", fmt.Errorf("websearch: invalid arguments: %w", err)
			}
			if params.Query == "" {
				return "", fmt.Errorf("websearch: query is required")
			}
			if params.MaxResults <= 0 {
				params.MaxResults = 10
			}
			if params.MaxResults > 20 {
				params.MaxResults = 20
			}

			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			form := url.Values{"q": {params.Query}}
			req, err := http.NewRequestWithContext(ctx, "POST", "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
			if err != nil {
				return "", fmt.Errorf("websearch: create request: %w", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Pegg/1.0)")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("websearch: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
			if err != nil {
				return "", fmt.Errorf("websearch: read body: %w", err)
			}

			results := parseDuckDuckGoResults(string(body), params.MaxResults)

			if len(results) == 0 {
				return "No results found.\n", nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "<search_results query=\"%s\" count=\"%d\">\n", params.Query, len(results))
			for _, r := range results {
				fmt.Fprintf(&b, "  <result>\n")
				fmt.Fprintf(&b, "    <title>%s</title>\n", r.Title)
				fmt.Fprintf(&b, "    <url>%s</url>\n", r.URL)
				if r.Snippet != "" {
					fmt.Fprintf(&b, "    <snippet>%s</snippet>\n", r.Snippet)
				}
				fmt.Fprintf(&b, "  </result>\n")
			}
			fmt.Fprintf(&b, "</search_results>\n")
			return b.String(), nil
		},
	)
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func parseDuckDuckGoResults(htmlContent string, maxResults int) []searchResult {
	var results []searchResult
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			isResult := false
			var href string
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "result__a") {
					isResult = true
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if isResult && href != "" {
				title := extractText(n)
				title = cleanText(title)
				if title != "" && href != "javascript:;" {
					cleanURL := cleanDuckDuckGoURL(href)
					results = append(results, searchResult{
						Title: title,
						URL:   cleanURL,
					})
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "a" && len(results) > 0 {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "result__snippet") {
					last := &results[len(results)-1]
					if last.Snippet == "" {
						last.Snippet = cleanText(extractText(n))
					}
					break
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "result__snippet") {
					if len(results) > 0 {
						last := &results[len(results)-1]
						if last.Snippet == "" {
							last.Snippet = cleanText(extractText(n))
						}
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

func extractText(n *html.Node) string {
	var b strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return b.String()
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

func cleanDuckDuckGoURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Host == "" && strings.HasPrefix(raw, "//") {
		u, err = url.Parse("https:" + raw)
		if err != nil {
			return raw
		}
	}
	q := u.Query()
	if target := q.Get("uddg"); target != "" {
		decoded, err := url.QueryUnescape(target)
		if err == nil {
			return decoded
		}
	}
	return u.String()
}
