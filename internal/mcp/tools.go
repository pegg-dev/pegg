package mcp

import (
	"context"
	"sort"
	"strings"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/agent/tools"
)

const ToolNameSeparator = "__"

type ServerTool struct {
	Name        string
	Description string
}

func ToolsForServer(server string) []ServerTool {
	var out []ServerTool
	for _, t := range tools.List() {
		mt, ok := t.(*MCPTool)
		if !ok || mt.Server() != server {
			continue
		}
		name := mt.Name()
		if i := strings.Index(name, ToolNameSeparator); i >= 0 {
			name = name[i+len(ToolNameSeparator):]
		}
		out = append(out, ServerTool{Name: name, Description: mt.Description()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type MCPTool struct {
	server      string
	tool        ToolSpec
	client      *Client
	description string
}

func NewTool(server string, spec ToolSpec, client *Client) *MCPTool {
	description := spec.Description
	if description == "" {
		description = "MCP tool provided by server " + server
	}
	return &MCPTool{
		server:      server,
		tool:        spec,
		client:      client,
		description: description,
	}
}

func (t *MCPTool) Name() string {
	return t.server + ToolNameSeparator + t.tool.Name
}

func (t *MCPTool) Server() string {
	return t.server
}

func (t *MCPTool) Description() string {
	return t.description
}

func (t *MCPTool) Parameters() any {
	if t.tool.InputSchema == nil {
		return map[string]any{"type": "object"}
	}
	return t.tool.InputSchema
}

func (t *MCPTool) Execute(ctx context.Context, args string) (string, error) {
	var arguments map[string]any
	args = strings.TrimSpace(args)
	if args != "" {
		if err := json.Unmarshal([]byte(args), &arguments); err != nil {
			return "", err
		}
	}

	result, err := t.client.CallTool(ctx, t.tool.Name, arguments)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			b.WriteString(content.Text)
		case "image":
			b.WriteString("[image data]")
		case "resource":
			b.WriteString(content.Text)
		}
		b.WriteString("\n")
	}

	out := strings.TrimSpace(b.String())
	if result.IsError {
		if out == "" {
			out = "tool returned an error"
		}
		return out, &CallError{Message: out}
	}
	return out, nil
}

type CallError struct {
	Message string
}

func (e *CallError) Error() string {
	return "mcp: " + e.Message
}
