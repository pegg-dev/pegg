package connect

import (
	"context"
	"sort"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/mcp"
)

func (c *Connector) registerMCPSkillRulePluginSettings() {
	c.registerMCPMethods()
	c.registerSkillMethods()
	c.registerRuleMethods()
	c.registerPluginMethods()
	c.registerSystemMethods()
}

func (c *Connector) registerMCPMethods() {
	c.dispatch.register("settings.mcp.list", ScopeSettingsRead, c.handleMCPList)
	c.dispatch.register("settings.mcp.tools", ScopeSettingsRead, c.handleMCPTools)
	c.dispatch.register("settings.mcp.upsert", ScopeSettingsWrite, c.handleMCPUpsert)
	c.dispatch.register("settings.mcp.remove", ScopeSettingsWrite, c.handleMCPRemove)
}

func (c *Connector) handleMCPList(_ context.Context, _ *Request) (any, error) {
	var global map[string]config.MCPServerConfig
	if c.deps.Config != nil {
		global = c.deps.Config.MCPServers
	}
	servers, err := mcp.ConfiguredServers(global)
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "load mcp servers: " + err.Error()}
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	clients := map[string]bool{}
	if c.deps.MCP != nil {
		for name := range c.deps.MCP.Clients() {
			clients[name] = true
		}
	}

	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{
			"name":      name,
			"command":   servers[name].Command,
			"args":      servers[name].Args,
			"url":       servers[name].URL,
			"env":       servers[name].Env,
			"headers":   servers[name].Headers,
			"tools":     len(mcp.ToolsForServer(name)),
			"connected": clients[name],
		})
	}
	return map[string]any{"servers": out}, nil
}

type mcpRefParams struct {
	Name string `json:"name"`
}

func (c *Connector) handleMCPTools(_ context.Context, req *Request) (any, error) {
	var p mcpRefParams
	_ = req.Decode(&p)
	if p.Name == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "name is required"}
	}
	tools := mcp.ToolsForServer(p.Name)
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{"name": t.Name, "description": t.Description})
	}
	return map[string]any{"server": p.Name, "tools": out}, nil
}

type mcpUpsertParams struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Scope   string            `json:"scope"`
}

func (c *Connector) handleMCPUpsert(_ context.Context, req *Request) (any, error) {
	var p mcpUpsertParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Name == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "name is required"}
	}
	if p.Command == "" && p.URL == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "command or url is required"}
	}
	server := config.MCPServerConfig{
		Command: p.Command,
		Args:    p.Args,
		Env:     p.Env,
		URL:     p.URL,
		Headers: p.Headers,
	}
	scope := p.Scope
	if scope == "" {
		scope = "global"
	}
	var err error
	switch scope {
	case "project":
		err = mcp.UpsertProjectServer(c.workspace(), p.Name, server)
	case "global":
		err = config.UpsertMCPServer(p.Name, server)
	default:
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "scope must be global or project"}
	}
	if err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: "save mcp server: " + err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "mcp", "scope": scope, "name": p.Name})
	return map[string]any{"name": p.Name, "scope": scope, "saved": true}, nil
}

type mcpRemoveParams struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

func (c *Connector) handleMCPRemove(_ context.Context, req *Request) (any, error) {
	var p mcpRemoveParams
	if err := req.Decode(&p); err != nil {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "invalid params: " + err.Error()}
	}
	if p.Name == "" {
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "name is required"}
	}
	scope := p.Scope
	if scope == "" {
		scope = "global"
	}
	var err error
	switch scope {
	case "project":
		err = mcp.RemoveProjectServer(c.workspace(), p.Name)
	case "global":
		err = config.RemoveMCPServer(p.Name)
	default:
		return nil, &FrameError{Code: ErrCodeBadRequest, Message: "scope must be global or project"}
	}
	if err != nil {
		return nil, &FrameError{Code: ErrCodeNotFound, Message: err.Error()}
	}
	if err := c.reloadConfig(); err != nil {
		return nil, &FrameError{Code: ErrCodeInternal, Message: err.Error()}
	}
	c.emit(eventSettings, "", "", map[string]any{"group": "mcp", "scope": scope, "removed": p.Name})
	return map[string]any{"name": p.Name, "removed": true}, nil
}
