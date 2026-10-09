package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/agent/tools"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/mcp"
)

const (
	mcpScopeGlobal  = "global"
	mcpScopeProject = "project"
)

const (
	mcpTransportLocal = "local"
	mcpTransportHTTP  = "http"
)

var popularHeaderKeys = []string{
	"Authorization",
	"X-API-Key",
	"Cookie",
	"User-Agent",
}

func (c *CLI) newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers",
	}
	cmd.AddCommand(
		c.newMCPListCommand(),
		c.newMCPAddCommand(),
		c.newMCPRemoveCommand(),
		c.newMCPToolsCommand(),
	)
	return cmd
}

func (c *CLI) newMCPListCommand() *cobra.Command {
	var globalOnly, projectOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP servers (global and project)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runMCPList(cmd.OutOrStdout(), globalOnly, projectOnly)
		},
	}
	cmd.Flags().BoolVar(&globalOnly, "global", false, "only list global servers")
	cmd.Flags().BoolVar(&projectOnly, "project", false, "only list project servers")
	return cmd
}

func (c *CLI) runMCPList(out io.Writer, globalOnly, projectOnly bool) error {
	if globalOnly && projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}
	project, err := mcp.LoadProjectServers(".")
	if err != nil {
		return fmt.Errorf("cli: load project servers: %w", err)
	}

	if globalOnly || projectOnly {
		servers := project
		label := "Project"
		if globalOnly {
			servers = cfg.MCPServers
			label = "Global"
		}
		fmt.Fprintf(out, "%s (%d):\n", label, len(servers))
		printMCPServers(out, servers)
		return nil
	}

	fmt.Fprintf(out, "Global (%d):\n", len(cfg.MCPServers))
	printMCPServers(out, cfg.MCPServers)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Project (%d):\n", len(project))
	printMCPServers(out, project)
	return nil
}

func printMCPServers(out io.Writer, servers map[string]config.MCPServerConfig) {
	if len(servers) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintf(out, "  %-24s %-7s %s\n", "NAME", "TYPE", "DETAIL")
	for _, name := range names {
		s := servers[name]
		if s.URL != "" {
			fmt.Fprintf(out, "  %-24s %-7s %s\n", name, "http", s.URL)
			continue
		}
		detail := strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
		if detail == "" {
			detail = "(invalid: no command or url)"
		}
		fmt.Fprintf(out, "  %-24s %-7s %s\n", name, "stdio", detail)
	}
}

type mcpAddFlags struct {
	globalOnly  bool
	projectOnly bool
	name        string
	transport   string
	command     string
	args        []string
	env         map[string]string
	url         string
	headers     map[string]string

	nameSet      bool
	transportSet bool
	commandSet   bool
	argsSet      bool
	envSet       bool
	urlSet       bool
	headersSet   bool
}

func (c *CLI) newMCPAddCommand() *cobra.Command {
	f := &mcpAddFlags{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an MCP server (interactive unless flags are given)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			f.nameSet = flags.Changed("name")
			f.transportSet = flags.Changed("transport")
			f.commandSet = flags.Changed("command")
			f.argsSet = flags.Changed("arg")
			f.envSet = flags.Changed("env")
			f.urlSet = flags.Changed("url")
			f.headersSet = flags.Changed("header")
			return c.runMCPAdd(cmd.OutOrStdout(), f)
		},
	}
	cmd.Flags().BoolVar(&f.globalOnly, "global", false, "add to global config")
	cmd.Flags().BoolVar(&f.projectOnly, "project", false, "add to project .mcp.json")
	cmd.Flags().StringVar(&f.name, "name", "", "server name")
	cmd.Flags().StringVar(&f.transport, "transport", "", "transport type: local or http")
	cmd.Flags().StringVar(&f.command, "command", "", "local command to run")
	cmd.Flags().StringSliceVar(&f.args, "arg", nil, "local command argument (repeatable)")
	cmd.Flags().StringToStringVar(&f.env, "env", nil, "local environment variable KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&f.url, "url", "", "remote SSE URL")
	cmd.Flags().StringToStringVar(&f.headers, "header", nil, "request header KEY=VALUE (repeatable)")
	return cmd
}

func (c *CLI) runMCPAdd(out io.Writer, f *mcpAddFlags) error {
	if f.globalOnly && f.projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	scope := ""
	switch {
	case f.globalOnly:
		scope = mcpScopeGlobal
	case f.projectOnly:
		scope = mcpScopeProject
	default:
		s, err := promptScope()
		if err != nil {
			return err
		}
		scope = s
	}

	transport := strings.ToLower(strings.TrimSpace(f.transport))
	if !f.transportSet || transport == "" {
		t, err := promptTransport()
		if err != nil {
			return err
		}
		transport = t
	}
	if transport != mcpTransportLocal && transport != mcpTransportHTTP {
		return fmt.Errorf("cli: unknown transport %q (use local or http)", transport)
	}

	name := strings.TrimSpace(f.name)
	if !f.nameSet || name == "" {
		n, err := promptString("Server name")
		if err != nil {
			return err
		}
		name = n
	}

	var server config.MCPServerConfig
	switch transport {
	case mcpTransportLocal:
		command := strings.TrimSpace(f.command)
		if !f.commandSet || command == "" {
			cmd, err := promptString("Command")
			if err != nil {
				return err
			}
			command = cmd
		}
		args := f.args
		if !f.argsSet || len(args) == 0 {
			a, err := promptArgs()
			if err != nil {
				return err
			}
			args = a
		}
		env := f.env
		if !f.envSet || len(env) == 0 {
			e, err := promptEnv()
			if err != nil {
				return err
			}
			env = e
		}
		server = config.MCPServerConfig{Command: command, Args: args, Env: env}
	case mcpTransportHTTP:
		url := strings.TrimSpace(f.url)
		if !f.urlSet || url == "" {
			u, err := promptString("URL")
			if err != nil {
				return err
			}
			url = u
		}
		headers := f.headers
		if !f.headersSet || len(headers) == 0 {
			h, err := promptHeaders()
			if err != nil {
				return err
			}
			headers = h
		}
		server = config.MCPServerConfig{URL: url, Headers: headers}
	}

	if scope == mcpScopeGlobal {
		if err := config.UpsertMCPServer(name, server); err != nil {
			return fmt.Errorf("cli: save mcp server: %w", err)
		}
	} else {
		if err := mcp.UpsertProjectServer(".", name, server); err != nil {
			return fmt.Errorf("cli: save project mcp server: %w", err)
		}
	}

	c.log.Finfo("mcp server %q saved (%s)", name, scope)
	fmt.Fprintf(out, "saved mcp server %q to %s\n", name, scope)
	return nil
}

func (c *CLI) newMCPRemoveCommand() *cobra.Command {
	var globalOnly, projectOnly bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runMCPRemove(cmd.OutOrStdout(), args[0], globalOnly, projectOnly)
		},
	}
	cmd.Flags().BoolVar(&globalOnly, "global", false, "remove from global config")
	cmd.Flags().BoolVar(&projectOnly, "project", false, "remove from project .mcp.json")
	return cmd
}

func (c *CLI) runMCPRemove(out io.Writer, name string, globalOnly, projectOnly bool) error {
	if globalOnly && projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}
	project, err := mcp.LoadProjectServers(".")
	if err != nil {
		return fmt.Errorf("cli: load project servers: %w", err)
	}

	_, inGlobal := cfg.MCPServers[name]
	_, inProject := project[name]

	scope := ""
	switch {
	case globalOnly:
		scope = mcpScopeGlobal
	case projectOnly:
		scope = mcpScopeProject
	case inGlobal && !inProject:
		scope = mcpScopeGlobal
	case !inGlobal && inProject:
		scope = mcpScopeProject
	case inGlobal && inProject:
		s, err := promptScope()
		if err != nil {
			return err
		}
		scope = s
	default:
		return fmt.Errorf("cli: mcp server %q not found", name)
	}

	if scope == mcpScopeGlobal {
		if err := config.RemoveMCPServer(name); err != nil {
			return fmt.Errorf("cli: remove mcp server: %w", err)
		}
	} else {
		if err := mcp.RemoveProjectServer(".", name); err != nil {
			return fmt.Errorf("cli: remove project mcp server: %w", err)
		}
	}

	c.log.Finfo("mcp server %q removed (%s)", name, scope)
	fmt.Fprintf(out, "removed mcp server %q from %s\n", name, scope)
	return nil
}

func (c *CLI) newMCPToolsCommand() *cobra.Command {
	var server string
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List tools exposed by connected MCP servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runMCPTools(cmd.OutOrStdout(), server)
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "only list tools for this server")
	return cmd
}

func (c *CLI) runMCPTools(out io.Writer, server string) error {
	if c.mcpMgr != nil {
		c.mcpMgr.Wait()
	}

	type row struct {
		server string
		name   string
	}
	var rows []row
	for _, t := range tools.List() {
		mt, ok := t.(*mcp.MCPTool)
		if !ok {
			continue
		}
		if server != "" && mt.Server() != server {
			continue
		}
		rows = append(rows, row{server: mt.Server(), name: t.Name()})
	}

	if len(rows) == 0 {
		if server != "" {
			fmt.Fprintf(out, "no MCP tools registered for server %q\n", server)
		} else {
			fmt.Fprintln(out, "no MCP tools registered")
		}
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].server != rows[j].server {
			return rows[i].server < rows[j].server
		}
		return rows[i].name < rows[j].name
	})
	fmt.Fprintf(out, "%-24s %s\n", "SERVER", "TOOL")
	for _, r := range rows {
		fmt.Fprintf(out, "%-24s %s\n", r.server, r.name)
	}
	return nil
}

func promptScope() (string, error) {
	items := []string{mcpScopeGlobal, mcpScopeProject}
	return promptSelect("Target", items)
}

func promptTransport() (string, error) {
	items := []string{"local (stdio)", "http (sse)"}
	res, err := promptSelect("Transport", items)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(res, "local") {
		return mcpTransportLocal, nil
	}
	return mcpTransportHTTP, nil
}

func promptSelect(label string, items []string) (string, error) {
	p := promptui.Select{
		Label: label,
		Items: items,
		Size:  len(items),
	}
	_, result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: %s: %w", label, err)
	}
	return result, nil
}

func promptString(label string) (string, error) {
	p := promptui.Prompt{Label: label}
	result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: %s: %w", label, err)
	}
	return strings.TrimSpace(result), nil
}

func promptArgs() ([]string, error) {
	var args []string
	for {
		a, err := promptString("Argument (empty to finish)")
		if err != nil {
			return nil, err
		}
		if a == "" {
			break
		}
		args = append(args, a)
	}
	return args, nil
}

func promptEnv() (map[string]string, error) {
	env := make(map[string]string)
	for {
		key, err := promptString("Env key (empty to finish)")
		if err != nil {
			return nil, err
		}
		if key == "" {
			break
		}
		value, err := promptString("Value for " + key)
		if err != nil {
			return nil, err
		}
		env[key] = value
	}
	return env, nil
}

func promptHeaders() (map[string]string, error) {
	headers := make(map[string]string)
	items := append(append([]string{}, popularHeaderKeys...), "Custom header", "Done")
	for {
		sel, err := promptSelect("Header (Done to finish)", items)
		if err != nil {
			return nil, err
		}
		if sel == "Done" {
			break
		}
		key := sel
		if sel == "Custom header" {
			key, err = promptString("Header name")
			if err != nil {
				return nil, err
			}
			if key == "" {
				continue
			}
		}
		value, err := promptString("Value for " + key)
		if err != nil {
			return nil, err
		}
		headers[key] = value
	}
	return headers, nil
}
