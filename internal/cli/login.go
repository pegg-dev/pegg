package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/llm/subscription"
)

func (c *CLI) newLoginCommand() *cobra.Command {
	var provider string
	var apiKey string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add or update a provider with an API key",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runLogin(provider, apiKey, c.selectProvider, c.promptAPIKey)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "provider name to log in to")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "provider API key")

	return cmd
}

func (c *CLI) runLogin(provider, apiKey string, selectProvider func() (string, error), promptAPIKey func() (string, error)) error {
	if provider == "" {
		p, err := selectProvider()
		if err != nil {
			return err
		}
		provider = p
	} else if !llm.HasProvider(provider) {
		return fmt.Errorf("cli: unknown provider %q (available: %v)", provider, llm.ListProviders())
	}

	if info, ok := subscription.Get(provider); ok {
		return c.loginSubscription(provider, info)
	}

	if apiKey == "" {
		key, err := promptAPIKey()
		if err != nil {
			return err
		}
		apiKey = key
	}

	cfg := config.LLMConfig{
		Provider: provider,
		APIKey:   apiKey,
	}

	if err := c.syncProvider(cfg); err != nil {
		return err
	}

	if err := config.UpsertProvider(cfg); err != nil {
		return fmt.Errorf("cli: save provider %q: %w", provider, err)
	}

	c.log.Finfo("provider %q saved", provider)

	return nil
}

func (c *CLI) loginSubscription(provider string, info subscription.Info) error {
	st := info.Status()
	if !st.LoggedIn {
		return fmt.Errorf("cli: %s subscription is not signed in — %s", provider, info.Hint)
	}

	cfg := config.LLMConfig{Provider: provider}
	if err := c.syncProvider(cfg); err != nil {
		return err
	}
	if err := config.UpsertProvider(cfg); err != nil {
		return fmt.Errorf("cli: save provider %q: %w", provider, err)
	}
	c.log.Finfo("provider %q (subscription) saved", provider)
	return nil
}

func (c *CLI) newLogoutCommand() *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove a subscription provider from pegg (does not sign out the official CLI)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return c.runLogout(provider)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "subscription provider to remove")
	return cmd
}

func (c *CLI) runLogout(provider string) error {
	if provider == "" {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("cli: load config: %w", err)
		}
		var names []string
		for _, p := range cfg.Providers {
			if subscription.IsSubscription(p.Provider) {
				names = append(names, p.Provider)
			}
		}
		switch len(names) {
		case 0:
			return errors.New("cli: no subscription providers configured")
		case 1:
			provider = names[0]
		default:
			return fmt.Errorf("cli: multiple subscription providers configured (%v); pass --provider", names)
		}
	}

	if !subscription.IsSubscription(provider) {
		return fmt.Errorf("cli: %q is not a subscription provider", provider)
	}
	if err := config.RemoveProvider(provider); err != nil {
		return err
	}
	c.log.Finfo("provider %q removed (official CLI credentials untouched)", provider)
	return nil
}

func (c *CLI) syncProvider(cfg config.LLMConfig) error {
	loaded := make(chan llm.ModelsLoaded, 1)
	handler := func(ml llm.ModelsLoaded) {
		if ml.Provider == cfg.Provider {
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
			return fmt.Errorf("cli: provider %q: %w", cfg.Provider, ml.Err)
		}
		return nil
	case <-time.After(35 * time.Second):
		return fmt.Errorf("cli: timed out syncing provider %q", cfg.Provider)
	}
}

func (c *CLI) selectProvider() (string, error) {
	names := llm.ListProviders()
	if len(names) == 0 {
		return "", errors.New("cli: no providers registered")
	}

	p := promptui.Select{
		Label: "Select provider",
		Items: names,
		Size:  10,
		Searcher: func(input string, index int) bool {
			provider := strings.ToLower(names[index])
			searchValue := strings.ToLower(input)
			return strings.Contains(provider, searchValue)
		},
	}

	_, result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: select provider: %w", err)
	}
	return result, nil
}

func (c *CLI) promptAPIKey() (string, error) {
	p := promptui.Prompt{
		Label: "API key (optional)",
		Mask:  '*',
	}

	result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: prompt api key: %w", err)
	}
	return result, nil
}
