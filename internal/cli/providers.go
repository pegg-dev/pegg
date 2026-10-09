package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/subscription"
)

func maskAPIKey(key string) string {
	if key == "" {
		return "(none)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func (c *CLI) runProviders(out io.Writer) error {
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

	fmt.Fprintf(out, "%-12s %-24s %s\n", "PROVIDER", "API KEY", "MODELS")
	for _, p := range cfg.Providers {
		count := 0
		if raw, err := store.Get(p.Provider); err == nil {
			var models []llm.Model
			if json.Unmarshal(raw, &models) == nil {
				count = len(models)
			}
		}
		auth := maskAPIKey(p.APIKey)
		if info, ok := subscription.Get(p.Provider); ok {
			auth = "not signed in"
			if st := info.Status(); st.LoggedIn {
				auth = "subscription"
				if st.Plan != "" {
					auth = "subscription (" + st.Plan + ")"
				}
			}
		}
		fmt.Fprintf(out, "%-12s %-24s %d\n", p.Provider, auth, count)
	}
	return nil
}

func (c *CLI) newProviderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage providers",
	}
	cmd.AddCommand(c.newProviderAddCommand(), c.newProviderListCommand(), c.newProviderRemoveCommand(), c.newProviderRefreshCommand())
	return cmd
}

type providerAddFlags struct {
	name       string
	driver     string
	apiKey     string
	baseURL    string
	headers    map[string]string
	timeout    int
	maxRetries int

	nameSet    bool
	driverSet  bool
	apiKeySet  bool
	baseURLSet bool
	headersSet bool
}

type providerAddPrompts struct {
	selectProvider func() (string, error)
	selectDriver   func() (string, error)
	promptAPIKey   func() (string, error)
	promptString   func(label string) (string, error)
	promptHeaders  func() (map[string]string, error)
	sync           func(config.LLMConfig) error
}

func (c *CLI) newProviderAddCommand() *cobra.Command {
	f := &providerAddFlags{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a provider (interactive unless flags are given)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			f.nameSet = flags.Changed("name")
			f.driverSet = flags.Changed("driver")
			f.apiKeySet = flags.Changed("api-key")
			f.baseURLSet = flags.Changed("base-url")
			f.headersSet = flags.Changed("header")
			p := providerAddPrompts{
				selectProvider: c.selectProviderWithCustom,
				selectDriver:   c.selectDriver,
				promptAPIKey:   c.promptAPIKey,
				promptString:   promptString,
				promptHeaders:  promptHeaders,
				sync:           c.syncAddedProvider,
			}
			return c.runProviderAdd(cmd.OutOrStdout(), f, p)
		},
	}
	cmd.Flags().StringVar(&f.name, "name", "", "provider name (registered provider)")
	cmd.Flags().StringVar(&f.driver, "driver", "", "driver for custom endpoints: openai, claude, or gemini")
	cmd.Flags().StringVar(&f.apiKey, "api-key", "", "provider API key")
	cmd.Flags().StringVar(&f.baseURL, "base-url", "", "base URL (required for driver entries)")
	cmd.Flags().StringToStringVar(&f.headers, "header", nil, "request header KEY=VALUE (repeatable)")
	cmd.Flags().IntVar(&f.timeout, "timeout", 0, "request timeout in seconds")
	cmd.Flags().IntVar(&f.maxRetries, "max-retries", 0, "maximum retry count")
	return cmd
}

func (c *CLI) runProviderAdd(out io.Writer, f *providerAddFlags, p providerAddPrompts) error {
	name := strings.TrimSpace(f.name)
	driver := strings.ToLower(strings.TrimSpace(f.driver))

	interactive := !f.nameSet && !f.driverSet && !f.baseURLSet

	if driver != "" && !llm.HasDriver(driver) {
		return fmt.Errorf("cli: unknown driver %q (available: %v)", driver, llm.ListDrivers())
	}

	if name == "" && driver == "" {
		choice, err := p.selectProvider()
		if err != nil {
			return err
		}
		choice = strings.TrimSpace(choice)
		if choice == customEndpointOption {
			d, err := p.selectDriver()
			if err != nil {
				return err
			}
			driver = strings.ToLower(strings.TrimSpace(d))
		} else {
			name = choice
			if name == "" {
				return errors.New("cli: provider name is required")
			}
		}
	}
	if name != "" && !llm.HasProvider(name) && driver == "" {
		return fmt.Errorf("cli: unknown provider %q (available: %v)", name, llm.ListProviders())
	}

	if f.timeout < 0 {
		return errors.New("cli: timeout must be >= 0")
	}
	if f.maxRetries < 0 {
		return errors.New("cli: max-retries must be >= 0")
	}

	apiKey := strings.TrimSpace(f.apiKey)
	if interactive && !f.apiKeySet {
		k, err := p.promptAPIKey()
		if err != nil {
			return err
		}
		apiKey = k
	}

	baseURL := strings.TrimSpace(f.baseURL)
	headers := f.headers
	note := ""

	if name == "" || !llm.HasProvider(name) {
		if interactive {
			if baseURL == "" {
				u, err := p.promptString("Base URL")
				if err != nil {
					return err
				}
				baseURL = strings.TrimSpace(u)
			}
			if !f.headersSet || len(headers) == 0 {
				h, err := p.promptHeaders()
				if err != nil {
					return err
				}
				headers = h
			}
		} else if baseURL == "" {
			return fmt.Errorf("cli: base-url is required for driver entries (--driver %s)", driver)
		}
	} else if baseURL != "" || driver != "" {
		note = "note: named providers use built-in endpoints; base_url/driver are stored but ignored"
	}

	cfg := config.LLMConfig{
		Provider:   name,
		Driver:     driver,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Timeout:    f.timeout,
		MaxRetries: f.maxRetries,
		Headers:    headers,
	}

	existing, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	identity := cfg.Provider
	if identity == "" {
		identity = cfg.Driver
	}

	action := "added"
	if cfg.Provider == "" {
		for _, e := range existing.Providers {
			if e.Provider == "" {
				fmt.Fprintf(out, "warning: replacing existing driver-only entry (driver=%q)\n", e.Driver)
				action = "updated"
				break
			}
		}
	} else {
		for _, e := range existing.Providers {
			if e.Provider == cfg.Provider {
				action = "updated"
				break
			}
		}
	}

	if err := p.sync(cfg); err != nil {
		c.log.Fwarn("provider %q models sync failed: %v", identity, err)
	}

	if err := config.UpsertProvider(cfg); err != nil {
		return fmt.Errorf("cli: save provider: %w", err)
	}

	c.log.Finfo("provider %q %s", identity, action)
	if note != "" {
		fmt.Fprintln(out, note)
	}
	fmt.Fprintf(out, "provider %q %s (driver=%s base_url=%s api_key=%s)\n",
		identity, action, dashIfEmpty(cfg.Driver), dashIfEmpty(cfg.BaseURL), maskAPIKey(cfg.APIKey))
	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

const customEndpointOption = "Custom"

func (c *CLI) selectProviderWithCustom() (string, error) {
	items := append(llm.ListProviders(), customEndpointOption)

	p := promptui.Select{
		Label: "Select provider",
		Items: items,
		Size:  10,
		Searcher: func(input string, index int) bool {
			searchValue := strings.ToLower(input)
			return strings.Contains(strings.ToLower(items[index]), searchValue)
		},
	}
	_, result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: select provider: %w", err)
	}
	return result, nil
}

func (c *CLI) selectDriver() (string, error) {
	names := llm.ListDrivers()
	if len(names) == 0 {
		return "", errors.New("cli: no drivers registered")
	}

	p := promptui.Select{
		Label: "Select driver",
		Items: names,
		Size:  10,
		Searcher: func(input string, index int) bool {
			searchValue := strings.ToLower(input)
			return strings.Contains(strings.ToLower(names[index]), searchValue)
		},
	}
	_, result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: select driver: %w", err)
	}
	return result, nil
}

func (c *CLI) syncAddedProvider(cfg config.LLMConfig) error {
	identity := cfg.Provider
	if identity == "" {
		identity = cfg.Driver
	}

	loaded := make(chan llm.ModelsLoaded, 1)
	handler := func(ml llm.ModelsLoaded) {
		if ml.Provider == identity {
			select {
			case loaded <- ml:
			default:
			}
		}
	}
	if err := c.bus.Subscribe(event.TopicModelsLoaded, handler); err != nil {
		return fmt.Errorf("cli: subscribe models loaded: %w", err)
	}
	defer c.bus.Unsubscribe(event.TopicModelsLoaded, handler)

	c.bus.Publish(event.TopicProviderAdded, cfg)

	select {
	case ml := <-loaded:
		if ml.Err != nil {
			return fmt.Errorf("cli: provider %q: %w", identity, ml.Err)
		}
		return nil
	case <-time.After(35 * time.Second):
		return fmt.Errorf("cli: timed out syncing provider %q", identity)
	}
}

func (c *CLI) newProviderListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runProviders(cmd.OutOrStdout())
		},
	}
}

func (c *CLI) newProviderRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return c.runProviderRemove(args[0])
		},
	}
}

func (c *CLI) runProviderRemove(name string) error {
	if err := config.RemoveProvider(name); err != nil {
		return fmt.Errorf("cli: remove provider: %w", err)
	}

	cfg, err := config.Load()
	if err == nil {
		if store, cerr := cache.CacheModule(cfg.Cache); cerr == nil {
			_ = store.Delete(name)
			_ = store.Close()
		}
	}

	c.bus.Publish(event.TopicProviderRemoved, name)
	c.log.Finfo("provider %q removed", name)
	return nil
}

func (c *CLI) newProviderRefreshCommand() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Re-fetch models for providers (bypasses cache)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runProviderRefresh(cmd.OutOrStdout(), provider)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "only refresh this provider")
	return cmd
}

func (c *CLI) runProviderRefresh(out io.Writer, provider string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	targets := configuredProviders(cfg, provider)
	if len(targets) == 0 {
		return fmt.Errorf("cli: provider %q not configured", provider)
	}

	store, err := cache.CacheModule(cfg.Cache)
	if err != nil {
		return fmt.Errorf("cli: open cache: %w", err)
	}
	defer store.Close()

	for _, p := range targets {
		if err := c.forceSyncProvider(store, p); err != nil {
			fmt.Fprintf(out, "%-12s FAIL  %v\n", p.Provider, err)
		} else {
			fmt.Fprintf(out, "%-12s OK\n", p.Provider)
		}
	}
	return nil
}

func (c *CLI) forceSyncProvider(store cache.Cache, cfg config.LLMConfig) error {
	_ = store.Delete(cfg.Provider)
	return c.syncProvider(cfg)
}

func configuredProviders(cfg *config.Config, provider string) []config.LLMConfig {
	var targets []config.LLMConfig
	for _, p := range cfg.Providers {
		if provider == "" || p.Provider == provider {
			targets = append(targets, p)
		}
	}
	return targets
}

func decodeModels(raw []byte) ([]llm.Model, bool) {
	var models []llm.Model
	if err := json.Unmarshal(raw, &models); err != nil {
		return nil, false
	}
	return models, true
}
