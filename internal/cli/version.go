package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
)

func (c *CLI) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "pegg version %s\n", config.AppVersion)
			return nil
		},
	}
}
