package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/utils/query"
)

func (c *CLI) newLogsCommand() *cobra.Command {
	var search string
	var level string
	var page, size int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View application logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runLogs(cmd.OutOrStdout(), search, level, page, size)
		},
	}

	cmd.Flags().StringVar(&search, "search", "", "search log messages (sqlite only)")
	cmd.Flags().StringVar(&level, "level", "", "filter by level: DEBUG, INFO, WARN, ERROR (sqlite only)")
	cmd.Flags().IntVar(&page, "page", 1, "page number (sqlite only)")
	cmd.Flags().IntVar(&size, "size", 50, "page size (sqlite only)")

	return cmd
}

func (c *CLI) runLogs(out io.Writer, search, level string, page, size int) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	if cfg.Logger.Driver == logger.DriverSQLite {
		return c.queryLogs(out, cfg.Logger, search, level, page, size)
	}

	return c.showBufferedLogs(out)
}

func (c *CLI) showBufferedLogs(out io.Writer) error {
	recs := c.log.Records()
	if len(recs) == 0 {
		fmt.Fprintln(out, "no logs in memory")
		return nil
	}
	for _, r := range recs {
		fmt.Fprintf(out, "[%s] [%-5s] %s\n",
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Level.String(),
			r.Message,
		)
	}
	return nil
}

func (c *CLI) queryLogs(out io.Writer, lcfg config.LoggerConfig, search, level string, page, size int) error {
	h, err := logger.NewSQLiteHandler(lcfg)
	if err != nil {
		return fmt.Errorf("cli: open sqlite logs: %w", err)
	}
	defer h.Close()

	q := query.Query{
		Page: query.Page{Number: page, Size: size},
	}
	if search != "" {
		q.Search = search
		q.SearchColumns = []string{"message", "level"}
	}
	if level != "" {
		q.Filters = append(q.Filters, query.Filter{
			Column:   "level",
			Operator: query.OpEqual,
			Value:    strings.ToUpper(level),
		})
	}

	recs, total, err := h.Query(q)
	if err != nil {
		return fmt.Errorf("cli: query logs: %w", err)
	}

	if len(recs) == 0 {
		fmt.Fprintf(out, "no logs found (total %d)\n", total)
		return nil
	}
	for _, r := range recs {
		fmt.Fprintf(out, "[%s] [%-5s] %s\n",
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Level.String(),
			r.Message,
		)
	}
	fmt.Fprintf(out, "\nshowing %d of %d logs\n", len(recs), total)
	return nil
}
