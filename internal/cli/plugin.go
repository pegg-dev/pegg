package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/plugin"
)

func (c *CLI) newPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
	}

	cmd.AddCommand(
		c.newPluginListCommand(),
		c.newPluginInfoCommand(),
		c.newPluginToggleCommand(),
	)

	return cmd
}

func (c *CLI) newPluginListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := plugin.NewManager(c.log, c.cfg, c.bus, c.cache, c.llmMgr, c.sessions, c.fs, c.mcpMgr, c.lspMgr)
			mgr.SetExcludedPlugins(c.cfg.Plugins.Exclude)

			if err := mgr.LoadPlugins(); err != nil {
				return fmt.Errorf("load plugins: %w", err)
			}
			defer mgr.Close()

			plugins := mgr.ListPlugins()
			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
				fmt.Fprintln(cmd.OutOrStdout())
				home, _ := os.UserHomeDir()
				pluginDir := filepath.Join(home, ".pegg", "plugins")
				fmt.Fprintf(cmd.OutOrStdout(), "Plugin directory: %s\n", pluginDir)
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION")
			fmt.Fprintln(w, "----\t-------\t-----------")
			for _, p := range plugins {
				fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.Version, p.Description)
			}
			w.Flush()

			fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d plugins\n", len(plugins))
			return nil
		},
	}
}

func (c *CLI) newPluginInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info [name]",
		Short: "Show plugin information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			mgr := plugin.NewManager(c.log, c.cfg, c.bus, c.cache, c.llmMgr, c.sessions, c.fs, c.mcpMgr, c.lspMgr)
			mgr.SetExcludedPlugins(c.cfg.Plugins.Exclude)

			if err := mgr.LoadPlugins(); err != nil {
				return fmt.Errorf("load plugins: %w", err)
			}
			defer mgr.Close()

			p, ok := mgr.GetPlugin(name)
			if !ok {
				return fmt.Errorf("plugin %q not found", name)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Name:        %s\n", p.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Version:     %s\n", p.Version)
			fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", p.Description)
			fmt.Fprintf(cmd.OutOrStdout(), "Path:        %s\n", p.Path)

			return nil
		},
	}
}

func (c *CLI) newPluginToggleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "toggle [name]",
		Short: "Toggle plugin enabled/disabled",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			enabled, err := config.TogglePlugin(name)
			if err != nil {
				return fmt.Errorf("toggle plugin: %w", err)
			}
			state := "enabled"
			if !enabled {
				state = "disabled"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plugin %s is now %s\n", name, state)
			return nil
		},
	}
}
