package mcp

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
)

type discardHandler struct{}

func (discardHandler) Write(logger.Record) error { return nil }
func (discardHandler) Close() error              { return nil }

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelDebug, discardHandler{})
}

func TestLoadProjectServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectConfigFileName)
	content := `{
		"mcpServers": {
			"db": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost:5432/db"],
				"env": {"DEBUG": "true"}
			},
			"remote": {
				"url": "https://example.com/sse",
				"headers": {"Authorization": "Bearer token"}
			}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	servers, err := LoadProjectServers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %+v", servers)
	}
	db := servers["db"]
	if db.Command != "npx" || len(db.Args) != 3 || db.Env["DEBUG"] != "true" {
		t.Fatalf("db = %+v", db)
	}
	remote := servers["remote"]
	if remote.URL != "https://example.com/sse" || remote.Headers["Authorization"] != "Bearer token" {
		t.Fatalf("remote = %+v", remote)
	}
}

func TestLoadProjectServersMissingFile(t *testing.T) {
	servers, err := LoadProjectServers(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %+v", servers)
	}
}

func TestLoadProjectServersInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectConfigFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectServers(dir); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMergeServersProjectOverridesGlobal(t *testing.T) {
	global := map[string]config.MCPServerConfig{
		"shared":      {Command: "global-cmd"},
		"only-global": {Command: "g"},
	}
	project := map[string]config.MCPServerConfig{
		"shared":       {Command: "project-cmd"},
		"only-project": {URL: "https://example.com"},
	}
	merged := MergeServers(global, project)
	if len(merged) != 3 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged["shared"].Command != "project-cmd" {
		t.Fatalf("shared = %+v", merged["shared"])
	}
	if merged["only-global"].Command != "g" || merged["only-project"].URL == "" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestNewTransportSelection(t *testing.T) {
	if _, err := NewTransport(config.MCPServerConfig{URL: "https://x/sse"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransport(config.MCPServerConfig{Command: "cat"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransport(config.MCPServerConfig{}, io.Discard); err != ErrUnsupported {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestMCPToolExecute(t *testing.T) {
	duplex := newScriptedDuplex(
		scriptedResponse(1, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "srv", "version": "1.0"},
		}),
		scriptedResponse(2, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "42"}},
		}),
	)
	c := NewClient("srv", duplex, Options{Name: "srv"})
	defer c.Close()
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	tool := NewTool("srv", ToolSpec{Name: "sum", InputSchema: map[string]any{"type": "object"}}, c)
	if tool.Name() != "srv__sum" {
		t.Fatalf("name = %q", tool.Name())
	}
	out, err := tool.Execute(context.Background(), `{"a":1,"b":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "42" {
		t.Fatalf("out = %q", out)
	}
}

func TestMCPToolExecuteError(t *testing.T) {
	duplex := newScriptedDuplex(
		scriptedResponse(1, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "srv", "version": "1.0"},
		}),
		scriptedResponse(2, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "failed"}},
			"isError": true,
		}),
	)
	c := NewClient("srv", duplex, Options{Name: "srv"})
	defer c.Close()
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	tool := NewTool("srv", ToolSpec{Name: "boom", InputSchema: map[string]any{"type": "object"}}, c)
	out, err := tool.Execute(context.Background(), "{}")
	if err == nil {
		t.Fatal("expected error")
	}
	if out != "failed" {
		t.Fatalf("out = %q", out)
	}
}

func TestToolNameCollisionPrevention(t *testing.T) {
	a := NewTool("srv-a", ToolSpec{Name: "get"}, nil)
	b := NewTool("srv-b", ToolSpec{Name: "get"}, nil)
	if a.Name() == b.Name() {
		t.Fatal("names should differ")
	}
	if a.Name() != "srv-a__get" || b.Name() != "srv-b__get" {
		t.Fatalf("names = %q, %q", a.Name(), b.Name())
	}
}

func TestManagerCloseWhileConnecting(t *testing.T) {
	for _, name := range tools.Names() {
		tools.Unregister(name)
	}

	srv := newSSETestServer("")
	srv.gate = make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.serve(ln)

	mgr := NewManager(newTestLogger())
	mgr.Start(map[string]config.MCPServerConfig{
		"slow-srv": {URL: "http://" + ln.Addr().String() + "/sse"},
	})

	go func() {
		time.Sleep(200 * time.Millisecond)
		close(srv.gate)
	}()

	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
	mgr.Wait()

	if _, ok := tools.Get("slow-srv__weather"); ok {
		t.Fatal("tool must not remain registered after manager close")
	}
}

func TestManagerStartsAndRegistersTools(t *testing.T) {
	for _, name := range tools.Names() {
		tools.Unregister(name)
	}

	srv := newSSETestServer("")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.serve(ln)

	log := newTestLogger()
	mgr := NewManager(log)
	mgr.Start(map[string]config.MCPServerConfig{
		"sse-srv": {URL: "http://" + ln.Addr().String() + "/sse"},
	})
	defer mgr.Close()

	deadline := time.After(5 * time.Second)
	for {
		names := tools.Names()
		found := false
		for _, n := range names {
			if n == "sse-srv__weather" {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("tool never registered; names = %v", tools.Names())
		case <-time.After(20 * time.Millisecond):
		}
	}

	mgr.Close()
	if _, ok := tools.Get("sse-srv__weather"); ok {
		t.Fatal("tool should be unregistered after close")
	}
}
