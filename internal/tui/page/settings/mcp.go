package settings

import (
	"sort"
	"strconv"

	"github.com/gdamore/tcell/v2"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/tui/components"
	"github.com/peggco/pegg/internal/tui/layout"
)

type mcpTab struct {
	settings *Settings
	list     *components.List
}

func newMCP(s *Settings) *mcpTab {
	t := &mcpTab{settings: s, list: components.NewList("MCP servers")}
	t.rebuild()
	return t
}

func (t *mcpTab) rebuild() {
	var (
		items   []components.ListItem
		servers map[string]config.MCPServerConfig
		err     error
	)
	if t.settings.deps.Config != nil {
		servers, err = mcp.ConfiguredServers(t.settings.deps.Config.MCPServers)
	} else {
		servers, err = mcp.ConfiguredServers(nil)
	}
	if err != nil {
		items = append(items, components.ListItem{Label: "failed to load servers", Detail: err.Error()})
		t.list.SetItems(items)
		return
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		toolCount := len(mcp.ToolsForServer(name))
		status := "not connected"
		if t.settings.deps.MCP != nil && t.settings.deps.MCP.Clients()[name] != nil {
			status = "connected"
		}
		items = append(items, components.ListItem{
			Label:  name,
			Detail: strconv.Itoa(toolCount) + " tools · " + status,
			Data:   name,
		})
	}
	if len(items) == 0 {
		items = append(items, components.ListItem{Label: "(no MCP servers configured)"})
	}
	t.list.SetItems(items)
	t.list.SetOnSelect(func(_ int, item components.ListItem) {
		if server, ok := item.Data.(string); ok {
			t.settings.openTools(server)
		}
	})
}

func (t *mcpTab) HandleKey(ev *tcell.EventKey) bool {
	return t.list.HandleKey(ev)
}

func (t *mcpTab) HandleMouse(x, y int, buttons tcell.ButtonMask) bool {
	return t.list.HandleMouse(x, y, buttons)
}

func (t *mcpTab) Draw(s tcell.Screen, bounds layout.Region, focused bool) {
	t.list.Draw(s, bounds, focused)
}
