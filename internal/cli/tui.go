package cli

import (
	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/tui"
)

func (c *CLI) newTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the terminal user interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := c.tuiDeps()
			if err != nil {
				return err
			}
			return tui.Run(c.bus, deps)
		},
	}
}
