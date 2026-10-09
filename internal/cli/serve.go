package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/acp"
	"github.com/peggco/pegg/internal/server"
)

func (c *CLI) newServeCommand() *cobra.Command {
	var port int
	var host string
	var acpMode bool
	var stdioMode bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start a server (HTTP API, or ACP over stdio/HTTP)",
		Long: `Start a Pegg server.

By default starts the HTTP REST API server on the configured port (default 8080).

With --acp starts the ACP (Agent Client Protocol) server over HTTP.
With --acp --stdio starts the ACP server over stdio (stdin/stdout).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := c.cfg
			if port > 0 {
				cfg.Server.Port = port
			}
			if host != "" {
				cfg.Server.Host = host
			}

			if acpMode {
				return c.runACPServe(stdioMode)
			}
			return c.runHTTPServe()
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "port to listen on (overrides config)")
	cmd.Flags().StringVar(&host, "host", "", "host to bind to (overrides config)")
	cmd.Flags().BoolVar(&acpMode, "acp", false, "start ACP server instead of HTTP API")
	cmd.Flags().BoolVar(&stdioMode, "stdio", false, "use stdio transport (only with --acp)")

	return cmd
}

func (c *CLI) runHTTPServe() error {
	srv := server.New(c.cfg, c.bus, c.log, c.fs, c.sessions, c.llmMgr, c.mcpMgr, c.lspMgr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (c *CLI) runACPServe(stdio bool) error {
	srv := acp.New(c.cfg, c.bus, c.log, c.fs, c.sessions, c.llmMgr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if stdio {
		transport := acp.NewStdioTransport()
		go func() {
			<-ctx.Done()
			transport.Close()
		}()
		if err := srv.Serve(ctx, transport); err != nil && err != io.EOF && err != context.Canceled {
			return fmt.Errorf("serve --acp --stdio: %w", err)
		}
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.cfg.Server.Host, c.cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("serve --acp: %w", err)
	}

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	c.log.Finfo("acp server listening on http://%s/acp", addr)
	if err := http.Serve(listener, srv.HTTPHandler()); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve --acp: %w", err)
	}
	return nil
}
