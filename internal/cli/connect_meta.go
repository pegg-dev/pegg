package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/connect"
	"github.com/peggco/pegg/internal/core/config"
)

func (c *CLI) newConnectLoginCommand() *cobra.Command {
	var token string

	cmd := &cobra.Command{
		Use:   "login [token]",
		Short: "Save a pairing token so `pegg connect` runs without arguments",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			argToken := ""
			if len(args) > 0 {
				argToken = args[0]
			}
			return c.runConnectLogin(cmd.OutOrStdout(), argToken, token)
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "pairing token to save (may also be passed as the argument)")
	return cmd
}

func (c *CLI) newConnectLogoutCommand() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Forget the saved pairing token",
		Long: `Removes the pairing token from the config. The device identity is kept, so
re-pairing with a new token reuses the same device. Use --all to also delete the
device identity (a new one is generated on the next connect).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc := c.connectConfig("", false, false, 0, nil, "", 0)
			hadToken := cc.Token != ""
			cc.Token = ""
			cc.Enabled = false
			if err := config.UpsertConnect(c.connectConfigForSave(cc)); err != nil {
				return fmt.Errorf("connect: save config: %w", err)
			}

			out := cmd.OutOrStdout()
			if hadToken {
				fmt.Fprintf(out, "pairing token removed from %s\n", configFileLabel())
			} else {
				fmt.Fprintln(out, "no pairing token was saved")
			}
			if all {
				if err := connect.ResetIdentity(); err != nil {
					return err
				}
				fmt.Fprintln(out, "device identity removed (a new one is generated on the next connect)")
			} else {
				fmt.Fprintln(out, "device identity kept — run `pegg connect <token>` with a new token to reconnect")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "also delete the device identity")
	return cmd
}

func (c *CLI) newConnectPairCommand(deviceName string) *cobra.Command {
	return &cobra.Command{
		Use:   "pair",
		Short: "Show this device's identity and how to pair it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc := c.connectConfig("", false, false, 0, nil, deviceName, 0)
			id, err := connect.LoadOrCreateIdentity(cc.DeviceName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Device ID:   %s\n", id.DeviceID)
			fmt.Fprintf(out, "Device name: %s\n", id.Name)
			fmt.Fprintf(out, "Public key:  %s\n", id.PublicKey)
			fmt.Fprintf(out, "Relay URL:   %s\n\n", relayOrEmpty(cc.RelayURL))
			fmt.Fprintln(out, "Create a pairing token on the website (Agents → Connect a device), then run:")
			fmt.Fprintln(out, "    pegg connect <token>")
			return nil
		},
	}
}

func (c *CLI) newConnectStatusCommand(deviceName string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show connect configuration, token and device identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc := c.connectConfig("", false, false, 0, nil, deviceName, 0)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "relay:          %s\n", relayOrEmpty(cc.RelayURL))
			fmt.Fprintf(out, "token:          %s\n", tokenStatus(cc.Token))
			fmt.Fprintf(out, "insecure:       %v\n", cc.Insecure)
			fmt.Fprintf(out, "auto-approve:   %v\n", cc.AutoApprove)
			fmt.Fprintf(out, "max-concurrency:%d\n", cc.MaxConcurrency)
			rt := cc.ResponseTimeoutSecs
			if rt == 0 {
				rt = config.DefaultConnectResponseTimeoutSecs
			}
			fmt.Fprintf(out, "response-timeout:%ds\n", rt)
			fmt.Fprintf(out, "allowed:        %v\n", orAll(cc.AllowedMethods))
			id, err := connect.LoadOrCreateIdentity("")
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "device id:      %s\n", id.DeviceID)
			fmt.Fprintf(out, "device name:    %s\n", id.Name)
			return nil
		},
	}
}

func (c *CLI) newConnectWhoamiCommand(deviceName string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print this device's identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := connect.LoadOrCreateIdentity(deviceName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", id.DeviceID, id.Name)
			fmt.Fprintf(out, "public key: %s\n", id.PublicKey)
			fmt.Fprintf(out, "created:    %s\n", id.CreatedAt)
			return nil
		},
	}
}

func (c *CLI) newConnectMethodsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "methods",
		Short: "List methods the backend may call",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			conn, err := connect.New(c.connectDeps(), c.connectConfig("", false, false, 0, nil, "", 0))
			if err != nil {
				return err
			}
			defer conn.Stop()
			out := cmd.OutOrStdout()
			for _, name := range conn.MethodNames() {
				fmt.Fprintln(out, name)
			}
			return nil
		},
	}
}

func relayOrEmpty(s string) string {
	if s == "" {
		return "(not configured)"
	}
	return s
}

func tokenStatus(token string) string {
	if token == "" {
		return "(none — run `pegg connect <token>`)"
	}
	return connect.MaskToken(token) + " (saved)"
}

func orAll(items []string) string {
	if len(items) == 0 {
		return "all"
	}
	return fmt.Sprintf("%v", items)
}
