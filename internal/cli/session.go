package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/utils/query"
)

func (c *CLI) newSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage sessions",
	}
	cmd.AddCommand(
		c.newSessionListCommand(),
		c.newSessionShowCommand(),
	)
	return cmd
}

func (c *CLI) newSessionListCommand() *cobra.Command {
	var search string
	var all bool
	var page, size int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runSessionList(cmd.OutOrStdout(), search, all, page, size)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "search sessions by title")
	cmd.Flags().BoolVar(&all, "all", false, "list sessions from all projects")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&size, "size", 50, "page size")
	return cmd
}

func (c *CLI) runSessionList(out io.Writer, search string, all bool, page, size int) error {
	q := query.Query{
		Page: query.Page{Number: page, Size: size},
		Filters: []query.Filter{
			{Column: "compaction_parent_id", Operator: query.OpEqual, Value: ""},
		},
	}
	if search != "" {
		q.Search = search
		q.SearchColumns = []string{"title"}
	}
	if !all {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cli: get working directory: %w", err)
		}
		q.Filters = append(q.Filters, query.Filter{Column: "project_dir", Operator: query.OpEqual, Value: dir})
	}

	sessions, total, err := c.sessions.List(q)
	if err != nil {
		return fmt.Errorf("cli: list sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Fprintf(out, "no sessions found (total %d)\n", total)
		return nil
	}
	for _, s := range sessions {
		fmt.Fprintf(out, "%-38s %-24s %-20s %-20s %s\n",
			s.ID, truncate(s.Title, 24), s.Provider+"/"+s.Model, s.ProjectDir,
			s.CreatedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(out, "\nshowing %d of %d sessions\n", len(sessions), total)
	return nil
}

func (c *CLI) newSessionShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a session and its messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runSessionShow(cmd.OutOrStdout(), args[0])
		},
	}
}

func (c *CLI) runSessionShow(out io.Writer, id string) error {
	s, err := c.sessions.Get(id)
	if err != nil {
		return fmt.Errorf("cli: get session: %w", err)
	}
	fmt.Fprintf(out, "ID:          %s\n", s.ID)
	fmt.Fprintf(out, "Title:       %s\n", s.Title)
	fmt.Fprintf(out, "Provider:    %s\n", s.Provider)
	fmt.Fprintf(out, "Model:       %s\n", s.Model)
	fmt.Fprintf(out, "ProjectDir:  %s\n", s.ProjectDir)
	fmt.Fprintf(out, "ParentID:    %s\n", s.ParentID)
	fmt.Fprintf(out, "CreatedAt:   %s\n", s.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "UpdatedAt:   %s\n", s.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "Usage:       %d tokens (prompt %d, completion %d, cost %.6f)\n",
		s.Usage.TotalTokens, s.Usage.PromptTokens, s.Usage.CompletionTokens, s.Usage.Cost)

	msgs, err := c.sessions.Messages(id)
	if err != nil {
		return fmt.Errorf("cli: get messages: %w", err)
	}
	fmt.Fprintln(out, "\nMessages:")
	for _, m := range msgs {
		fmt.Fprintf(out, "  #%d [%s] %s\n", m.Seq, m.Role, truncate(fmt.Sprint(m.Content), 120))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n-3]) + "..."
}
