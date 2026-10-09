package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
)

func (c *CLI) newCacheCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the cache",
	}

	var provider string
	clear := &cobra.Command{
		Use:   "clear",
		Short: "Clear cached data",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runCacheClear(provider)
		},
	}
	clear.Flags().StringVar(&provider, "provider", "", "only clear this provider's cached models")

	cmd.AddCommand(clear)
	return cmd
}

func (c *CLI) runCacheClear(provider string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	store, err := cache.CacheModule(cfg.Cache)
	if err != nil {
		return fmt.Errorf("cli: open cache: %w", err)
	}
	defer store.Close()

	if provider != "" {
		if err := store.Delete(provider); err != nil {
			return fmt.Errorf("cli: clear provider cache: %w", err)
		}
		c.log.Finfo("cleared cache for provider %q", provider)
		return nil
	}

	if err := store.Clear(); err != nil {
		return fmt.Errorf("cli: clear cache: %w", err)
	}
	c.log.Info("cache cleared")
	return nil
}
