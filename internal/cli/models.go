package cli

import (
	"fmt"
	"io"

	json "github.com/goccy/go-json"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/router"
)

func (c *CLI) newModelsCommand() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List cached models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runModels(cmd.OutOrStdout(), provider)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "only show models for this provider")
	return cmd
}

func (c *CLI) runModels(out io.Writer, provider string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	for _, opt := range router.ModelOptionsHook.Apply(nil) {
		if provider != "" && provider != opt.Provider {
			continue
		}
		fmt.Fprintf(out, "[%s]\n", opt.Provider)
		fmt.Fprintf(out, "  - %s (%s)\n", opt.Label, opt.Detail)
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

	for _, p := range cfg.Providers {
		if provider != "" && p.Provider != provider {
			continue
		}
		fmt.Fprintf(out, "[%s]\n", p.Provider)
		raw, err := store.Get(p.Provider)
		if err != nil {
			fmt.Fprintln(out, "  no cached models")
			continue
		}
		var models []llm.Model
		if err := json.Unmarshal(raw, &models); err != nil {
			fmt.Fprintln(out, "  (unreadable cache)")
			continue
		}
		for _, m := range models {
			name := m.ID
			if m.Name != "" {
				name = m.Name
			}
			fmt.Fprintf(out, "  - %s\n", name)
		}
	}
	return nil
}
