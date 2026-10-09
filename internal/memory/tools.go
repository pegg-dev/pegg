package memory

import (
	"context"
	"fmt"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/agent/tool"
	agenttools "github.com/peggco/pegg/internal/agent/tools"
)

func generateSearchToolPrompt() (string, error) {
	return prompt.New().
		Paragraph("Search the persistent project memory for past observations relevant to a query. Memory contains distilled records of previous sessions: bugfixes, decisions, discoveries, lessons, and session summaries.").
		Paragraph("Use this BEFORE re-investigating something you suspect was already solved or learned. Results include the memory id; use mem-read to fetch the full entry.").
		Build(prompt.FormatMarkdown)
}

func generateReadToolPrompt() (string, error) {
	return prompt.New().
		Paragraph("Read a full memory entry by its id (e.g. obs-9f3a1b or sum-2c4d5e). IDs appear in search results, in the project memory panel, and in the memory index.").
		Build(prompt.FormatMarkdown)
}

func MemoryTools(m *Manager) []tool.Tool {
	return []tool.Tool{SearchTool(m), ReadTool(m)}
}

func RegisterTools(m *Manager) error {
	for _, t := range MemoryTools(m) {
		if err := agenttools.Register(t); err != nil {
			return err
		}
	}
	return nil
}

func SearchTool(m *Manager) tool.Tool {
	desc, err := generateSearchToolPrompt()
	if err != nil {
		desc = "Search the persistent project memory for past observations relevant to a query."
	}
	return tool.NewSpec(
		"mem-search",
		desc,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query: keywords, a file path, an error message, or a natural language question.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (1-10, default 5).",
				},
			},
			"required": []string{"query"},
		},
		func(ctx context.Context, args string) (string, error) {
			return m.search(ctx, args)
		},
	)
}

func ReadTool(m *Manager) tool.Tool {
	desc, err := generateReadToolPrompt()
	if err != nil {
		desc = "Read a full memory entry by id."
	}
	return tool.NewSpec(
		"mem-read",
		desc,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Memory entry id, e.g. obs-9f3a1b or sum-2c4d5e.",
				},
			},
			"required": []string{"id"},
		},
		func(_ context.Context, args string) (string, error) {
			return m.read(args)
		},
	)
}

type searchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (m *Manager) search(ctx context.Context, args string) (string, error) {
	if m == nil || m.store == nil {
		return "Memory is unavailable.", nil
	}
	if !m.enabled() {
		return "Memory is disabled.", nil
	}
	var in searchArgs
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("mem-search: invalid arguments: %w", err)
	}
	if strings.TrimSpace(in.Query) == "" {
		return "mem-search: query is required.", nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = m.cfgSnapshot().MaxResultsValue()
	}
	if limit > 10 {
		limit = 10
	}

	hits := m.store.SearchFiles(in.Query, limit)
	if len(hits) == 0 {
		return "No results found.\n", nil
	}
	hits = m.gateHits(ctx, in.Query, hits)
	if len(hits) == 0 {
		return "No relevant results found.\n", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<memory_results query=\"%s\" count=\"%d\">\n", xmlEscape(in.Query), len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "  <result>\n")
		if h.ID != "" {
			fmt.Fprintf(&b, "    <id>%s</id>\n", xmlEscape(h.ID))
		}
		if h.Type != "" {
			fmt.Fprintf(&b, "    <type>%s</type>\n", xmlEscape(h.Type))
		}
		if h.Title != "" {
			fmt.Fprintf(&b, "    <title>%s</title>\n", xmlEscape(h.Title))
		}
		if h.Date != "" {
			fmt.Fprintf(&b, "    <date>%s</date>\n", xmlEscape(h.Date))
		}
		if h.File != "" {
			fmt.Fprintf(&b, "    <file>%s</file>\n", xmlEscape(h.File))
		}
		body := stripBlockHeader(h.Text)
		excerpt := clampTo(strings.Join(strings.Fields(body), " "), 500)
		if excerpt != "" {
			fmt.Fprintf(&b, "    <snippet>%s</snippet>\n", xmlEscape(excerpt))
		}
		fmt.Fprintf(&b, "  </result>\n")
	}
	fmt.Fprintf(&b, "</memory_results>\n")
	return b.String(), nil
}

type readArgs struct {
	ID string `json:"id"`
}

func (m *Manager) read(args string) (string, error) {
	if m == nil || m.store == nil {
		return "Memory is unavailable.", nil
	}
	if !m.enabled() {
		return "Memory is disabled.", nil
	}
	var in readArgs
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return "", fmt.Errorf("mem-read: invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "mem-read: id is required.", nil
	}
	entries, err := m.store.ReadIndex()
	if err != nil {
		return "", fmt.Errorf("mem-read: read index: %w", err)
	}
	for _, e := range entries {
		if e.ID != in.ID {
			continue
		}
		if block, ok := m.store.ReadBlock(e.File, in.ID); ok {
			return block, nil
		}
		return "Memory entry not found in session file.", nil
	}
	return fmt.Sprintf("Memory entry %q not found.", in.ID), nil
}

func stripBlockHeader(block string) string {
	lines := strings.Split(block, "\n")
	start := 0
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") {
			start = i + 1
			break
		}
	}
	var out []string
	for _, l := range lines[start:] {
		if strings.HasPrefix(l, "- hash:") {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
