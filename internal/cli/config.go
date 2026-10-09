package cli

import (
	"fmt"
	"io"

	json "github.com/goccy/go-json"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
)

func (c *CLI) newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	cmd.AddCommand(c.newConfigShowCommand())
	return cmd
}

func (c *CLI) newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration (API keys masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runConfigShow(cmd.OutOrStdout())
		},
	}
}

func (c *CLI) runConfigShow(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	path, _ := config.GetConfigPath(config.GlobalConfigFileName)
	fmt.Fprintf(out, "config: %s\n\n", path)

	masked := *cfg
	masked.Providers = append([]config.LLMConfig(nil), cfg.Providers...)
	for i := range masked.Providers {
		masked.Providers[i].APIKey = maskAPIKey(masked.Providers[i].APIKey)
	}

	data, err := json.MarshalIndent(masked, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(data))
	return nil
}
