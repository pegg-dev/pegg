package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/mcp"
)

func TestMCPAddGlobal(t *testing.T) {
	c, _ := newTestCLI(t)

	if err := c.Execute([]string{
		"mcp", "add",
		"--global",
		"--name", "db",
		"--transport", "local",
		"--command", "npx",
		"--arg", "-y",
		"--arg", "@modelcontextprotocol/server-postgres",
		"--env", "DEBUG=true",
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := cfg.MCPServers["db"]
	if !ok {
		t.Fatalf("mcp servers = %+v, want db", cfg.MCPServers)
	}
	if s.Command != "npx" || len(s.Args) != 2 || s.Env["DEBUG"] != "true" {
		t.Fatalf("db = %+v", s)
	}
}

func TestMCPAddProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	c, _ := newTestCLI(t)
	if err := c.Execute([]string{
		"mcp", "add",
		"--project",
		"--name", "remote",
		"--transport", "http",
		"--url", "https://mcp.example.com/sse",
		"--header", "Authorization=Bearer token",
	}); err != nil {
		t.Fatal(err)
	}

	servers, err := mcp.LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := servers["remote"]
	if !ok {
		t.Fatalf("project servers = %+v, want remote", servers)
	}
	if s.URL != "https://mcp.example.com/sse" || s.Headers["Authorization"] != "Bearer token" {
		t.Fatalf("remote = %+v", s)
	}
}

func TestMCPAddInvalidTransport(t *testing.T) {
	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"mcp", "add", "--global", "--name", "x", "--transport", "banana"}); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestMCPRemoveGlobal(t *testing.T) {
	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.MCPServers["db"] = config.MCPServerConfig{Command: "npx"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := c.Execute([]string{"mcp", "remove", "db", "--global"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["db"]; ok {
		t.Fatalf("mcp servers = %+v, want db removed", loaded.MCPServers)
	}
}

func TestMCPRemoveProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := mcp.UpsertProjectServer(dir, "db", config.MCPServerConfig{Command: "npx"}); err != nil {
		t.Fatal(err)
	}

	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"mcp", "remove", "db", "--project"}); err != nil {
		t.Fatal(err)
	}

	servers, err := mcp.LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("project servers = %+v, want none", servers)
	}
}

func TestMCPRemoveAutoScope(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := mcp.UpsertProjectServer(dir, "db", config.MCPServerConfig{Command: "npx"}); err != nil {
		t.Fatal(err)
	}

	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"mcp", "remove", "db"}); err != nil {
		t.Fatal(err)
	}

	servers, err := mcp.LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("project servers = %+v, want none", servers)
	}
}

func TestMCPRemoveMissing(t *testing.T) {
	c, _ := newTestCLI(t)
	if err := c.Execute([]string{"mcp", "remove", "nope", "--global"}); err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestMCPListBothScopesSeparated(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	c, _ := newTestCLI(t)

	cfg := config.DefaultConfig()
	cfg.MCPServers["global-db"] = config.MCPServerConfig{Command: "npx", Args: []string{"-y"}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := mcp.UpsertProjectServer(dir, "project-remote", config.MCPServerConfig{URL: "https://x/sse"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"mcp", "list"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Global (1):") || !strings.Contains(out, "Project (1):") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(out, "global-db") || !strings.Contains(out, "project-remote") {
		t.Fatalf("output = %q", out)
	}
}

func TestMCPListFiltered(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := mcp.UpsertProjectServer(dir, "proj", config.MCPServerConfig{Command: "go"}); err != nil {
		t.Fatal(err)
	}

	c, _ := newTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"mcp", "list", "--project"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Project (1):") || !strings.Contains(out, "proj") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "Global") {
		t.Fatalf("output should not contain Global section: %q", out)
	}
}

func TestMCPTools(t *testing.T) {
	for _, name := range tools.Names() {
		tools.Unregister(name)
	}
	_ = tools.Register(tool.NewSpec("plain-tool", "not mcp", nil, nil))
	defer tools.Unregister("plain-tool")

	mt := mcp.NewTool("db", mcp.ToolSpec{Name: "query", InputSchema: map[string]any{"type": "object"}}, nil)
	if err := tools.Register(mt); err != nil {
		t.Fatal(err)
	}
	defer tools.Unregister(mt.Name())

	c, _ := newTestCLI(t)
	var buf bytes.Buffer
	c.root.SetOut(&buf)
	if err := c.Execute([]string{"mcp", "tools"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "db") || !strings.Contains(out, "db__query") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "plain-tool") {
		t.Fatalf("non-MCP tool leaked into output: %q", out)
	}

	buf.Reset()
	if err := c.Execute([]string{"mcp", "tools", "--server", "db"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "db__query") {
		t.Fatalf("filtered output = %q", buf.String())
	}

	buf.Reset()
	if err := c.Execute([]string{"mcp", "tools", "--server", "nope"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no MCP tools registered for server") {
		t.Fatalf("filtered output = %q", buf.String())
	}
}
