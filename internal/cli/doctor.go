package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/update"
)

func (c *CLI) newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check provider connectivity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runDoctor(cmd.OutOrStdout())
		},
	}
}

func (c *CLI) runDoctor(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}
	if len(cfg.Providers) == 0 {
		fmt.Fprintln(out, "no providers configured")
		return nil
	}

	store, err := cache.CacheModule(cfg.Cache)
	if err != nil {
		return fmt.Errorf("cli: open cache: %w", err)
	}
	defer store.Close()

	fmt.Fprintln(out, "checking providers...")
	for _, p := range cfg.Providers {
		if err := c.forceSyncProvider(store, p); err != nil {
			fmt.Fprintf(out, "  %-12s FAIL  %v\n", p.Provider, err)
			continue
		}
		count := 0
		if raw, err := store.Get(p.Provider); err == nil {
			if models, ok := decodeModels(raw); ok {
				count = len(models)
			}
		}
		fmt.Fprintf(out, "  %-12s OK (%d models)\n", p.Provider, count)
	}

	fmt.Fprintln(out, "\nchecking for updates...")
	current := config.AppVersion
	ctx := context.Background()
	rel, found, err := update.DetectLatest(ctx)
	if err != nil {
		fmt.Fprintf(out, "  Update check failed: %v\n", err)
	} else if found && update.IsNewerThan(current, rel.Version) {
		fmt.Fprintf(out, "  New version available: %s (current: %s)\n", rel.Version, current)
		fmt.Print("  Do you want to update? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response == "y" || response == "Y" {
			fmt.Fprintln(out, "  Updating...")
			if err := update.UpdateToLatest(ctx); err != nil {
				fmt.Fprintf(out, "  Update failed: %v\n", err)
			} else {
				fmt.Fprintf(out, "  Successfully updated to version %s\n", rel.Version)
			}
		}
	} else {
		fmt.Fprintln(out, "  You are running the latest version")
	}

	return nil
}
