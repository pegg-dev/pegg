package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/core/cache"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/event"
	"github.com/peggco/pegg/internal/core/hook"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/decision"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/lsp"
	"github.com/peggco/pegg/internal/mcp"
	"github.com/peggco/pegg/internal/memory"
	"github.com/peggco/pegg/internal/plugin"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/tui"
	"github.com/peggco/pegg/internal/tui/page/settings"
	"github.com/peggco/pegg/internal/vfs"
)

type CLI struct {
	bus       event.Bus
	cfg       *config.Config
	log       *logger.Logger
	sessions  *session.Manager
	llmMgr    *llm.Manager
	memMgr    *memory.Manager
	decMgr    *decision.Manager
	mcpMgr    *mcp.Manager
	lspMgr    *lsp.Manager
	fs        *vfs.VFS
	cache     cache.Cache
	pluginMgr *plugin.Manager
	added     bool
	root      *cobra.Command
	commands  hook.Hook[[]*cobra.Command]
	picker    func(items []string, label string) (int, error)
}

func New(bus event.Bus, cfg *config.Config, log *logger.Logger, vfs *vfs.VFS, sessions *session.Manager, llmMgr *llm.Manager, decMgr *decision.Manager, memMgr *memory.Manager, mcpMgr *mcp.Manager, lspMgr *lsp.Manager, cache cache.Cache, pluginMgr *plugin.Manager) *CLI {
	c := &CLI{
		bus:       bus,
		cfg:       cfg,
		log:       log,
		fs:        vfs,
		cache:     cache,
		sessions:  sessions,
		llmMgr:    llmMgr,
		decMgr:    decMgr,
		memMgr:    memMgr,
		mcpMgr:    mcpMgr,
		lspMgr:    lspMgr,
		pluginMgr: pluginMgr,
		root:      newRootCommand(),
		picker:    defaultPicker,
	}

	c.root.RunE = func(cmd *cobra.Command, args []string) error {
		if !isTerminal(cmd.InOrStdin()) {
			var parts []string
			if argMsg := strings.TrimSpace(strings.Join(args, " ")); argMsg != "" {
				parts = append(parts, argMsg)
			}
			stdinMsg, err := readStdinMessage(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if stdinMsg != "" {
				parts = append(parts, stdinMsg)
			}
			message := strings.Join(parts, "\n\n")
			return c.runRun(cmd.OutOrStdout(), cmd.InOrStdin(), message, runOptions{})
		}
		deps, err := c.tuiDeps()
		if err != nil {
			return err
		}
		return tui.Run(bus, deps)
	}

	c.registerDefaultCommands()

	return c
}

func newRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pegg",
		Short: "pegg command line interface",
		Args:  cobra.ArbitraryArgs,
	}
}

func (c *CLI) OnRegisterCommand(fn func([]*cobra.Command) []*cobra.Command) {
	c.commands.Add(fn)
}

func (c *CLI) registerDefaultCommands() {
	c.OnRegisterCommand(func(cmds []*cobra.Command) []*cobra.Command {
		return append(cmds,
			c.newLoginCommand(),
			c.newLogoutCommand(),
			c.newLogsCommand(),
			c.newFilesCommand(),
			c.newCacheCommand(),
			c.newProviderCommand(),
			c.newModelsCommand(),
			c.newDoctorCommand(),
			c.newConfigCommand(),
			c.newSessionCommand(),
			c.newRunCommand(),
			c.newMCPCommand(),
			c.newLSPCommand(),
			c.newTUICommand(),
			c.newServeCommand(),
			c.newConnectCommand(),
			c.newPluginCommand(),
			c.newVersionCommand(),
			c.newUpdateCommand(),
		)
	})
}

func (c *CLI) tuiDeps() (settings.Deps, error) {
	orch, err := agents.New("orchestrator")
	if err != nil {
		return settings.Deps{}, fmt.Errorf("cli: create orchestrator: %w", err)
	}
	orch.Bus = c.bus
	return settings.Deps{
		Config:   c.cfg,
		LLM:      c.llmMgr,
		Decision: c.decMgr,
		Memory:   c.memMgr,
		MCP:      c.mcpMgr,
		Sessions: c.sessions,
		Agent:    orch,
		Bus:      c.bus,
		VFS:      c.fs,
		Cache:    c.cache,
		Plugin:   c.pluginMgr,
	}, nil
}

func (c *CLI) Execute(args []string) error {
	if !c.added {
		for _, cmd := range c.commands.Apply(nil) {
			c.root.AddCommand(cmd)
		}
		c.added = true
	}

	c.root.SetArgs(args)
	return c.root.Execute()
}

func readStdinMessage(in io.Reader) (string, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("cli: read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
