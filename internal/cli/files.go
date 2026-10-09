package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
)

func (c *CLI) newFilesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "files",
		Short: "Show system file paths",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runFiles(cmd.OutOrStdout())
		},
	}
}

func (c *CLI) runFiles(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	configPath, _ := config.GetConfigPath(config.GlobalConfigFileName)

	logPath := "console (stdout, not persisted)"
	if cfg.Logger.Driver == logger.DriverSQLite {
		if p, err := config.GetConfigPath("logs.db"); err == nil {
			logPath = p
		}
	}

	cachePath, _ := config.GetConfigPath("cache.json")
	if cfg.Cache.Driver == "sqlite" {
		if p, err := config.GetConfigPath("cache.db"); err == nil {
			cachePath = p
		}
	}

	sessionsPath, _ := config.GetConfigPath("sessions")
	if cfg.Session.Driver == "sqlite" {
		if p, err := config.GetConfigPath("sessions.db"); err == nil {
			sessionsPath = p
		}
	}

	lspsPath, _ := config.GetConfigPath("lsps")

	printFile(out, "config", configPath)
	printFile(out, "logs", logPath)
	printFile(out, "cache", cachePath)
	printFile(out, "sessions", sessionsPath)
	printFile(out, "lsps", lspsPath)
	return nil
}

func printFile(out io.Writer, label, path string) {
	status := "missing"
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			status = "exists"
		}
	}
	fmt.Fprintf(out, "%-6s %-7s %s\n", label, status, path)
}
