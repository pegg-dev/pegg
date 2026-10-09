package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/peggco/pegg/internal/connect"
	"github.com/peggco/pegg/internal/core/config"
)

type tokenSource int

const envConnectRelay = "PEGG_CONNECT_RELAY"

const (
	tokenFromArgument tokenSource = iota
	tokenFromFlag
	tokenFromConfig
	tokenFromPrompt
)

func (s tokenSource) isSaved() bool { return s == tokenFromConfig }

func resolveConnectToken(argToken, flagToken, savedToken string, interactive bool, prompt func() (string, error)) (string, tokenSource, error) {
	for _, candidate := range []struct {
		value  string
		source tokenSource
	}{
		{argToken, tokenFromArgument},
		{flagToken, tokenFromFlag},
		{savedToken, tokenFromConfig},
	} {
		if trimmed := strings.TrimSpace(candidate.value); trimmed != "" {
			return trimmed, candidate.source, nil
		}
	}
	if !interactive {
		return "", tokenFromConfig, errors.New(
			"connect: no pairing token — run `pegg connect <token>` (create one on the website under Agents → Connect a device)")
	}
	token, err := prompt()
	if err != nil {
		return "", tokenFromPrompt, err
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", tokenFromPrompt, errors.New("connect: no pairing token provided")
	}
	return token, tokenFromPrompt, nil
}

func promptConnectToken(out io.Writer) (string, error) {
	fd := int(os.Stdin.Fd())
	fmt.Fprint(out, "Paste your pairing token (create one on the website under Agents → Connect a device): ")
	defer fmt.Fprintln(out)
	if term.IsTerminal(fd) {
		raw, err := term.ReadPassword(fd)
		return string(raw), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return line, err
}

func (c *CLI) newConnectCommand() *cobra.Command {
	var token string
	var insecure bool
	var autoApprove bool
	var maxConcurrency int
	var allowed []string
	var deviceName string
	var responseTimeout int

	cmd := &cobra.Command{
		Use:   "connect [token]",
		Short: "Connect this app to the website (pairing token only)",
		Long: `Connect starts an outbound relay daemon. It dials the relay over a secure
WebSocket and executes the commands the website sends (chat, sessions,
settings). All work happens locally; the website never runs anything on your
machine.

Only a pairing token is needed — create one on the website under
Agents → Connect a device. The token is saved to the config on first use, so
later runs just need "pegg connect"; use "pegg connect logout" to forget it.

The relay endpoint is built in (connect.relay_url overrides it). The daemon
authenticates with a local Ed25519 device identity stored at
~/.pegg/connect/device.json and reconnects automatically.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			argToken := ""
			if len(args) > 0 {
				argToken = args[0]
			}
			return c.runConnect(cmd.OutOrStdout(), argToken, token, insecure, autoApprove, maxConcurrency, allowed, deviceName, responseTimeout)
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "pairing token (may also be passed as the argument)")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "allow ws:// to a non-loopback host (not recommended)")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "skip local confirmation for sensitive remote actions (not recommended)")
	cmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 0, "maximum parallel runs (default from config)")
	cmd.Flags().StringArrayVar(&allowed, "allowed-method", nil, "restrict methods the backend may call (repeatable, supports prefix*)")
	cmd.Flags().StringVar(&deviceName, "device-name", "", "human-friendly name for this device")
	cmd.Flags().IntVar(&responseTimeout, "response-timeout", 0, "seconds a run may produce no output before it is failed (0=default 180, <0=disabled)")

	cmd.AddCommand(
		c.newConnectLoginCommand(),
		c.newConnectLogoutCommand(),
		c.newConnectStatusCommand(deviceName),
		c.newConnectWhoamiCommand(deviceName),
		c.newConnectMethodsCommand(),
		c.newConnectPairCommand(deviceName),
	)
	return cmd
}

func (c *CLI) connectDeps() connect.Deps {
	return connect.Deps{
		Config:   c.cfg,
		Bus:      c.bus,
		Log:      c.log,
		VFS:      c.fs,
		Sessions: c.sessions,
		LLM:      c.llmMgr,
		Decision: c.decMgr,
		Memory:   c.memMgr,
		MCP:      c.mcpMgr,
		LSP:      c.lspMgr,
		Cache:    c.cache,
		Plugin:   c.pluginMgr,
		Version:  config.AppVersion,
		Trace: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[connect] "+format+"\n", args...)
		},
	}
}

func connectRelayURL(configured string) string {
	if trimmed := strings.TrimSpace(configured); trimmed != "" {
		return trimmed
	}
	if env := strings.TrimSpace(os.Getenv(envConnectRelay)); env != "" {
		return env
	}
	return config.DefaultConnectRelayURL
}

func (c *CLI) connectConfig(token string, insecure, autoApprove bool, maxConcurrency int, allowed []string, deviceName string, responseTimeout int) *config.ConnectConfig {
	cc := config.DefaultConfig().Connect
	savedRelay, savedToken := "", ""
	if c.cfg != nil && c.cfg.Connect != nil {
		*cc = *c.cfg.Connect
		savedRelay, savedToken = cc.RelayURL, cc.Token
	}
	if embedded := connect.TokenFromURL(savedRelay); embedded != "" {
		savedRelay = connect.StripToken(savedRelay)
		if savedToken == "" {
			savedToken = embedded
		}
	}
	cc.RelayURL = connectRelayURL(savedRelay)
	cc.Token = savedToken
	if token != "" {
		cc.Token = token
	}
	if insecure {
		cc.Insecure = true
	}
	if autoApprove {
		cc.AutoApprove = true
	}
	if maxConcurrency > 0 {
		cc.MaxConcurrency = maxConcurrency
	}
	if len(allowed) > 0 {
		cc.AllowedMethods = allowed
	}
	if deviceName != "" {
		cc.DeviceName = deviceName
	}
	if responseTimeout != 0 {
		cc.ResponseTimeoutSecs = responseTimeout
	}
	cc.Enabled = true
	return cc
}

func (c *CLI) savedConnectToken() string {
	if c.cfg == nil || c.cfg.Connect == nil {
		return ""
	}
	return c.cfg.Connect.Token
}

func (c *CLI) savedRelayURL() string {
	if c.cfg == nil || c.cfg.Connect == nil {
		return ""
	}
	return c.cfg.Connect.RelayURL
}

func (c *CLI) connectConfigForSave(cc *config.ConnectConfig) *config.ConnectConfig {
	if strings.TrimSpace(c.savedRelayURL()) != "" {
		return cc
	}
	clone := *cc
	clone.RelayURL = ""
	return &clone
}

func (c *CLI) runConnect(out io.Writer, argToken, flagToken string, insecure, autoApprove bool, maxConcurrency int, allowed []string, deviceName string, responseTimeout int) error {
	cc := c.connectConfig(flagToken, insecure, autoApprove, maxConcurrency, allowed, deviceName, responseTimeout)

	token, source, err := resolveConnectToken(
		argToken, flagToken, c.savedConnectToken(), stdinInteractive(),
		func() (string, error) { return promptConnectToken(out) },
	)
	if err != nil {
		return err
	}
	cc.Token = token
	if err := config.UpsertConnect(c.connectConfigForSave(cc)); err != nil {
		return fmt.Errorf("connect: save config: %w", err)
	}
	if !source.isSaved() {
		fmt.Fprintf(out, "pairing token saved to %s — future runs only need `pegg connect`\n", configFileLabel())
	}

	conn, err := connect.New(c.connectDeps(), cc)
	if err != nil {
		return err
	}
	defer conn.Stop()

	id := conn.Identity()
	fmt.Fprintf(out, "pegg connect: device %s (%s)\n", id.DeviceID, id.Name)
	fmt.Fprintf(out, "  public key: %s\n", id.PublicKey)
	fmt.Fprintf(out, "  relay:      %s\n", cc.RelayURL)
	fmt.Fprintf(out, "  token:      %s\n", connect.MaskToken(cc.Token))
	fmt.Fprintf(out, "  methods:    %d\n", len(conn.MethodNames()))
	if !cc.AutoApprove && !stdinInteractive() {
		fmt.Fprintln(out, "  note: non-interactive terminal — sensitive actions will require --auto-approve")
	}
	fmt.Fprintln(out, "dialing relay (press Ctrl+C to stop)...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return conn.Run(ctx)
}

func (c *CLI) runConnectLogin(out io.Writer, argToken, flagToken string) error {
	cc := c.connectConfig(flagToken, false, false, 0, nil, "", 0)
	token, source, err := resolveConnectToken(
		argToken, flagToken, c.savedConnectToken(), stdinInteractive(),
		func() (string, error) { return promptConnectToken(out) },
	)
	if err != nil {
		return err
	}
	cc.Token = token
	if err := config.UpsertConnect(c.connectConfigForSave(cc)); err != nil {
		return fmt.Errorf("connect: save config: %w", err)
	}
	if source.isSaved() {
		fmt.Fprintf(out, "pairing token already saved: %s\n", connect.MaskToken(token))
	} else {
		fmt.Fprintf(out, "pairing token saved: %s\n", connect.MaskToken(token))
	}
	fmt.Fprintf(out, "relay: %s\n", cc.RelayURL)
	fmt.Fprintln(out, "run `pegg connect` to start the relay")
	return nil
}

func configFileLabel() string {
	if path, err := config.GetConfigPath(config.GlobalConfigFileName); err == nil {
		return path
	}
	return "~/.pegg/" + config.GlobalConfigFileName
}

func stdinInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
