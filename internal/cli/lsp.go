package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/lsp"
)

const (
	lspScopeGlobal  = "global"
	lspScopeProject = "project"
)

func (c *CLI) newLSPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Manage language servers",
	}
	cmd.AddCommand(
		c.newLSPListCommand(),
		c.newLSPAddCommand(),
		c.newLSPRemoveCommand(),
		c.newLSPDiagCommand(),
	)
	return cmd
}

func (c *CLI) newLSPListCommand() *cobra.Command {
	var globalOnly, projectOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List language servers (global and project)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runLSPList(cmd.OutOrStdout(), globalOnly, projectOnly)
		},
	}
	cmd.Flags().BoolVar(&globalOnly, "global", false, "only list global servers")
	cmd.Flags().BoolVar(&projectOnly, "project", false, "only list project servers")
	return cmd
}

func (c *CLI) runLSPList(out io.Writer, globalOnly, projectOnly bool) error {
	if globalOnly && projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}
	project, err := lsp.LoadProjectServers(".")
	if err != nil {
		return fmt.Errorf("cli: load project servers: %w", err)
	}

	if globalOnly || projectOnly {
		servers := project
		label := "Project"
		if globalOnly {
			servers = cfg.LanguageServers
			label = "Global"
		}
		fmt.Fprintf(out, "%s (%d):\n", label, len(servers))
		printLSPServers(out, servers)
		return nil
	}

	fmt.Fprintf(out, "Global (%d):\n", len(cfg.LanguageServers))
	printLSPServers(out, cfg.LanguageServers)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Project (%d):\n", len(project))
	printLSPServers(out, project)
	return nil
}

func printLSPServers(out io.Writer, servers map[string]config.LanguageServerConfig) {
	if len(servers) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintf(out, "  %-28s %-10s %s\n", "NAME", "FILE TYPES", "COMMAND")
	for _, name := range names {
		s := servers[name]
		detail := strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
		if s.Download != "" {
			detail += " (download)"
		} else if s.Install != "" {
			detail += " (install)"
		}
		fmt.Fprintf(out, "  %-28s %-10s %s\n", name, strings.Join(s.FileTypes, ","), detail)
	}
}

type lspAddFlags struct {
	globalOnly  bool
	projectOnly bool
	name        string
	command     string
	args        []string
	filetypes   []string
	rootMarkers []string
	download    string
	env         map[string]string

	nameSet        bool
	commandSet     bool
	argsSet        bool
	filetypesSet   bool
	rootMarkersSet bool
	downloadSet    bool
	envSet         bool
}

func (c *CLI) newLSPAddCommand() *cobra.Command {
	f := &lspAddFlags{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a language server (interactive unless flags are given)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			f.nameSet = flags.Changed("name")
			f.commandSet = flags.Changed("command")
			f.argsSet = flags.Changed("arg")
			f.filetypesSet = flags.Changed("filetype")
			f.rootMarkersSet = flags.Changed("root-marker")
			f.downloadSet = flags.Changed("download")
			f.envSet = flags.Changed("env")
			return c.runLSPAdd(cmd.OutOrStdout(), f)
		},
	}
	cmd.Flags().BoolVar(&f.globalOnly, "global", false, "add to global config")
	cmd.Flags().BoolVar(&f.projectOnly, "project", false, "add to project .lsp.json")
	cmd.Flags().StringVar(&f.name, "name", "", "server name")
	cmd.Flags().StringVar(&f.command, "command", "", "command to run")
	cmd.Flags().StringSliceVar(&f.args, "arg", nil, "command argument (repeatable)")
	cmd.Flags().StringSliceVar(&f.filetypes, "filetype", nil, "file extension to serve (repeatable)")
	cmd.Flags().StringSliceVar(&f.rootMarkers, "rootmarker", nil, "root marker file (repeatable)")
	cmd.Flags().StringVar(&f.download, "download", "", "static binary URL to download if not installed")
	cmd.Flags().StringToStringVar(&f.env, "env", nil, "environment variable KEY=VALUE (repeatable)")
	return cmd
}

func (c *CLI) runLSPAdd(out io.Writer, f *lspAddFlags) error {
	if f.globalOnly && f.projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	scope := ""
	switch {
	case f.globalOnly:
		scope = lspScopeGlobal
	case f.projectOnly:
		scope = lspScopeProject
	default:
		s, err := promptScope()
		if err != nil {
			return err
		}
		scope = s
	}

	name := strings.TrimSpace(f.name)
	if !f.nameSet || name == "" {
		n, err := promptString("Server name")
		if err != nil {
			return err
		}
		name = n
	}

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

	filetypes := f.filetypes
	if !f.filetypesSet || len(filetypes) == 0 {
		ft, err := promptArgs()
		if err != nil {
			return err
		}
		filetypes = ft
	}

	rootMarkers := f.rootMarkers
	if !f.rootMarkersSet || len(rootMarkers) == 0 {
		rm, err := promptArgs()
		if err != nil {
			return err
		}
		rootMarkers = rm
	}

	download := strings.TrimSpace(f.download)
	if !f.downloadSet || download == "" {
		download = ""
	}

	env := f.env
	if !f.envSet || len(env) == 0 {
		e, err := promptEnv()
		if err != nil {
			return err
		}
		env = e
	}

	server := config.LanguageServerConfig{
		Command:     command,
		Args:        args,
		Env:         env,
		FileTypes:   filetypes,
		RootMarkers: rootMarkers,
		Download:    download,
	}

	if scope == lspScopeGlobal {
		if err := config.UpsertLanguageServer(name, server); err != nil {
			return fmt.Errorf("cli: save language server: %w", err)
		}
	} else {
		if err := lsp.UpsertProjectServer(".", name, server); err != nil {
			return fmt.Errorf("cli: save project language server: %w", err)
		}
	}

	c.log.Finfo("language server %q saved (%s)", name, scope)
	fmt.Fprintf(out, "saved language server %q to %s\n", name, scope)
	return nil
}

func (c *CLI) newLSPRemoveCommand() *cobra.Command {
	var globalOnly, projectOnly bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a language server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runLSPRemove(cmd.OutOrStdout(), args[0], globalOnly, projectOnly)
		},
	}
	cmd.Flags().BoolVar(&globalOnly, "global", false, "remove from global config")
	cmd.Flags().BoolVar(&projectOnly, "project", false, "remove from project .lsp.json")
	return cmd
}

func (c *CLI) runLSPRemove(out io.Writer, name string, globalOnly, projectOnly bool) error {
	if globalOnly && projectOnly {
		return errors.New("cli: cannot combine --global and --project")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}
	project, err := lsp.LoadProjectServers(".")
	if err != nil {
		return fmt.Errorf("cli: load project servers: %w", err)
	}

	_, inGlobal := cfg.LanguageServers[name]
	_, inProject := project[name]

	scope := ""
	switch {
	case globalOnly:
		scope = lspScopeGlobal
	case projectOnly:
		scope = lspScopeProject
	case inGlobal && !inProject:
		scope = lspScopeGlobal
	case !inGlobal && inProject:
		scope = lspScopeProject
	case inGlobal && inProject:
		s, err := promptScope()
		if err != nil {
			return err
		}
		scope = s
	default:
		return fmt.Errorf("cli: language server %q not found", name)
	}

	if scope == lspScopeGlobal {
		if err := config.RemoveLanguageServer(name); err != nil {
			return fmt.Errorf("cli: remove language server: %w", err)
		}
	} else {
		if err := lsp.RemoveProjectServer(".", name); err != nil {
			return fmt.Errorf("cli: remove project language server: %w", err)
		}
	}

	c.log.Finfo("language server %q removed (%s)", name, scope)
	fmt.Fprintf(out, "removed language server %q from %s\n", name, scope)
	return nil
}

func (c *CLI) newLSPDiagCommand() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "diag",
		Short: "Show diagnostics for a file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runLSPDiag(cmd.OutOrStdout(), file)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "virtual path to query (required)")
	return cmd
}

func (c *CLI) runLSPDiag(out io.Writer, file string) error {
	if c.lspMgr == nil {
		return errors.New("cli: language server manager not available")
	}
	if strings.TrimSpace(file) == "" {
		return errors.New("cli: --file is required")
	}
	diags := c.lspMgr.Diagnostics(file)
	if len(diags) == 0 {
		fmt.Fprintf(out, "no diagnostics for %q\n", file)
		return nil
	}
	for _, d := range diags {
		fmt.Fprintf(out, "%s:%d:%d %s\n", file, d.Line()+1, d.Column()+1, d.Message)
	}
	return nil
}
