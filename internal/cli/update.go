package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/update"
)

func (c *CLI) newUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Check for updates and install if available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runUpdate(cmd.OutOrStdout())
		},
	}
}

func (c *CLI) runUpdate(out io.Writer) error {
	current := config.AppVersion
	fmt.Fprintf(out, "Current version: %s\n", current)

	ctx := context.Background()
	rel, found, err := update.DetectLatest(ctx)
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}
	if !found {
		fmt.Fprintln(out, "No releases found")
		return nil
	}

	fmt.Fprintf(out, "Latest version: %s\n", rel.Version)

	if !update.IsNewerThan(current, rel.Version) {
		fmt.Fprintln(out, "You are running the latest version")
		return nil
	}

	fmt.Fprintf(out, "Updating to version %s...\n", rel.Version)
	if err := update.UpdateToLatest(ctx); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Fprintf(out, "Successfully updated to version %s\n", rel.Version)
	return nil
}
