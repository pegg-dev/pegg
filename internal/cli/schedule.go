package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/schedule"
	"github.com/peggco/pegg/internal/session"
)

func (c *CLI) newScheduleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage recurring background agent schedules",
		Long: "Create and manage cron schedules that run the agent automatically in the background.\n" +
			"Schedules persist across restarts and run whenever a long-running pegg process\n" +
			"(connect, tui, serve) is active. Each run is saved as a session titled 'schedule HH.MM.SS'.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runScheduleList(cmd.OutOrStdout(), nil)
		},
	}
	cmd.AddCommand(
		c.newScheduleCreateCommand(),
		c.newScheduleListCommand(),
		c.newScheduleGetCommand(),
		c.newScheduleUpcomingCommand(),
		c.newScheduleActiveCommand(),
		c.newScheduleTriggerCommand(),
		c.newSchedulePauseCommand(),
		c.newScheduleResumeCommand(),
		c.newScheduleUpdateCommand(),
		c.newScheduleHistoryCommand(),
		c.newScheduleStatsCommand(),
		c.newScheduleDeleteCommand(),
		c.newScheduleRunCommand(),
	)
	return cmd
}

func (c *CLI) requireScheduler() error {
	if c.sched == nil {
		return errors.New("cli: scheduler unavailable")
	}
	return nil
}

func (c *CLI) resolveScheduleID(args []string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return args[0], nil
	}
	if !stdinInteractive() {
		return "", errors.New("cli: schedule id is required")
	}
	return c.selectSchedule()
}

func (c *CLI) selectSchedule() (string, error) {
	list := c.sched.List()
	if len(list) == 0 {
		return "", errors.New("cli: no schedules configured")
	}
	items := make([]string, 0, len(list))
	for _, s := range list {
		status := "active"
		if !s.Enabled {
			status = "paused"
		}
		items = append(items, fmt.Sprintf("%-24s %-7s %-16s %s",
			truncate(s.Name, 24), status, s.Cron, s.ID))
	}
	idx, err := c.picker(items, "Select schedule")
	if err != nil {
		return "", err
	}
	if idx < 0 || idx >= len(list) {
		return "", errors.New("cli: invalid schedule selection")
	}
	return list[idx].ID, nil
}

func (c *CLI) newScheduleCreateCommand() *cobra.Command {
	var opts schedule.CreateOptions
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a recurring schedule",
		Long: "Create a recurring schedule. Any of the required values (name, cron, prompt)\n" +
			"that are not provided as arguments or flags will be prompted for interactively.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			if len(args) > 0 && strings.TrimSpace(opts.Name) == "" {
				opts.Name = strings.Join(args, " ")
			}
			if err := c.fillScheduleInputs(cmd.OutOrStdout(), &opts); err != nil {
				return err
			}
			s, err := c.sched.Create(opts)
			if err != nil {
				return err
			}
			printSchedule(cmd.OutOrStdout(), s)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "schedule name")
	cmd.Flags().StringVar(&opts.Cron, "cron", "", "cron expression, e.g. '0 9 * * MON-FRI'")
	cmd.Flags().StringVar(&opts.Prompt, "prompt", "", "prompt sent to the agent on each run")
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "project directory the run executes in (defaults to current directory)")
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "provider override for the runs")
	cmd.Flags().StringVar(&opts.Model, "model", "", "model override for the runs")
	cmd.Flags().StringSliceVar(&opts.Tags, "tags", nil, "comma-separated tags")
	cmd.Flags().IntVar(&opts.TimeoutSecs, "timeout", 0, "stop a run that takes longer than this many seconds")
	cmd.Flags().IntVar(&opts.MaxParallel, "max-parallel", 0, "maximum concurrent runs (default 1)")
	cmd.Flags().BoolVar(&opts.Disabled, "disabled", false, "create the schedule paused")
	return cmd
}

func (c *CLI) fillScheduleInputs(out io.Writer, opts *schedule.CreateOptions) error {
	interactive := stdinInteractive()

	if strings.TrimSpace(opts.Name) == "" {
		if !interactive {
			return errors.New("cli: schedule name is required (pass it as an argument or --name)")
		}
		name, err := promptString("Schedule name")
		if err != nil {
			return err
		}
		opts.Name = name
	}

	if strings.TrimSpace(opts.Cron) == "" {
		if !interactive {
			return errors.New("cli: --cron is required")
		}
		cron, err := c.promptCron(out)
		if err != nil {
			return err
		}
		opts.Cron = cron
	}

	if strings.TrimSpace(opts.Prompt) == "" {
		if !interactive {
			return errors.New("cli: --prompt is required")
		}
		prompt, err := promptString("Prompt")
		if err != nil {
			return err
		}
		opts.Prompt = prompt
	}

	if interactive {
		def := opts.Workspace
		if def == "" {
			if wd, err := os.Getwd(); err == nil {
				def = wd
			}
		}
		ws, err := promptStringDefault("Workspace", def)
		if err != nil {
			return err
		}
		if ws != "" {
			opts.Workspace = ws
		}
	}

	return nil
}

func (c *CLI) newScheduleListCommand() *cobra.Command {
	var tags []string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List schedules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runScheduleList(cmd.OutOrStdout(), tags)
		},
	}
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "only list schedules with any of these tags")
	return cmd
}

func (c *CLI) runScheduleList(out io.Writer, tags []string) error {
	if err := c.requireScheduler(); err != nil {
		return err
	}
	list := c.sched.List()
	shown := 0
	for _, s := range list {
		if len(tags) > 0 && !scheduleHasAnyTag(s.Tags, tags) {
			continue
		}
		printSchedule(out, s)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(out, "no schedules configured")
	}
	return nil
}

func (c *CLI) newScheduleGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get [id]",
		Short: "Show a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			s, err := c.sched.Get(id)
			if err != nil {
				return err
			}
			printSchedule(cmd.OutOrStdout(), s)
			return nil
		},
	}
}

func (c *CLI) newScheduleUpcomingCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upcoming",
		Short: "Preview upcoming runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			list, err := c.sched.Upcoming(10)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(list) == 0 {
				fmt.Fprintln(out, "no upcoming runs")
				return nil
			}
			for _, s := range list {
				next := "-"
				if s.NextRun != nil {
					next = s.NextRun.Format("2006-01-02 15:04:05")
				}
				fmt.Fprintf(out, "%-20s %-16s %s\n", truncate(s.Name, 20), next, s.ID)
			}
			return nil
		},
	}
}

func (c *CLI) newScheduleActiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "Show currently running executions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			active, err := c.sched.Active()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(active) == 0 {
				fmt.Fprintln(out, "no active executions")
				return nil
			}
			for _, r := range active {
				fmt.Fprintf(out, "%-38s %-20s started %s\n", r.RunID, r.ScheduleID, r.StartedAt.Format(time.RFC3339))
			}
			return nil
		},
	}
}

func (c *CLI) newScheduleTriggerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger [id]",
		Short: "Run a schedule immediately",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			if err := c.sched.Trigger(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "triggered %s\n", id)
			return nil
		},
	}
}

func (c *CLI) newSchedulePauseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pause [id]",
		Short: "Pause a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			if _, err := c.sched.Pause(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "paused %s\n", id)
			return nil
		},
	}
}

func (c *CLI) newScheduleResumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [id]",
		Short: "Resume a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			if _, err := c.sched.Resume(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resumed %s\n", id)
			return nil
		},
	}
}

func (c *CLI) newScheduleUpdateCommand() *cobra.Command {
	var (
		name, cron, prompt, workspace, provider, model string
		tags                                           []string
		timeout, maxParallel                           int
		enabled                                        bool
	)
	cmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Update a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			opts := schedule.UpdateOptions{}
			flags := cmd.Flags()
			if flags.Changed("name") {
				opts.Name = &name
			}
			if flags.Changed("cron") {
				opts.Cron = &cron
			}
			if flags.Changed("prompt") {
				opts.Prompt = &prompt
			}
			if flags.Changed("workspace") {
				opts.Workspace = &workspace
			}
			if flags.Changed("provider") {
				opts.Provider = &provider
			}
			if flags.Changed("model") {
				opts.Model = &model
			}
			if flags.Changed("tags") {
				opts.Tags = &tags
			}
			if flags.Changed("timeout") {
				opts.TimeoutSecs = &timeout
			}
			if flags.Changed("max-parallel") {
				opts.MaxParallel = &maxParallel
			}
			if flags.Changed("enabled") {
				opts.Enabled = &enabled
			}
			s, err := c.sched.Update(id, opts)
			if err != nil {
				return err
			}
			printSchedule(cmd.OutOrStdout(), s)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&cron, "cron", "", "new cron expression")
	cmd.Flags().StringVar(&prompt, "prompt", "", "new prompt")
	cmd.Flags().StringVar(&workspace, "workspace", "", "new workspace")
	cmd.Flags().StringVar(&provider, "provider", "", "new provider")
	cmd.Flags().StringVar(&model, "model", "", "new model")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "new tags")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "new timeout in seconds")
	cmd.Flags().IntVar(&maxParallel, "max-parallel", 1, "new max parallel")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable or disable")
	return cmd
}

func (c *CLI) newScheduleHistoryCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history [id]",
		Short: "Show past runs of a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			runs, err := c.sched.History(id, limit)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(runs) == 0 {
				fmt.Fprintln(out, "no runs recorded")
				return nil
			}
			for _, r := range runs {
				dur := "-"
				if r.DurationMs > 0 {
					dur = (time.Duration(r.DurationMs) * time.Millisecond).Round(time.Second).String()
				}
				fmt.Fprintf(out, "%-20s %-9s %-10s %-38s %s\n",
					r.StartedAt.Format("2006-01-02 15:04:05"), r.Status, dur, r.SessionID, truncate(r.Error, 40))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of runs to show")
	return cmd
}

func (c *CLI) newScheduleStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats [id]",
		Short: "Show statistics for a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			st, err := c.sched.Stats(id)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "total:        %d\n", st.Total)
			fmt.Fprintf(out, "success:      %d\n", st.Success)
			fmt.Fprintf(out, "failed:       %d\n", st.Failed)
			fmt.Fprintf(out, "running:      %d\n", st.Running)
			fmt.Fprintf(out, "success rate: %.1f%%\n", st.SuccessRate*100)
			fmt.Fprintf(out, "avg duration: %s\n", (time.Duration(st.AvgDurationMs) * time.Millisecond).Round(time.Second))
			if st.LastRun != nil {
				fmt.Fprintf(out, "last run:     %s\n", st.LastRun.Format(time.RFC3339))
			}
			if st.LastFailure != nil {
				fmt.Fprintf(out, "last failure: %s\n", st.LastFailure.Format(time.RFC3339))
			}
			return nil
		},
	}
}

func (c *CLI) newScheduleDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a schedule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.requireScheduler(); err != nil {
				return err
			}
			id, err := c.resolveScheduleID(args)
			if err != nil {
				return err
			}
			if err := c.sched.Delete(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", id)
			return nil
		},
	}
}

func (c *CLI) newScheduleRunCommand() *cobra.Command {
	var id, runID, sessionID string
	cmd := &cobra.Command{
		Use:    "run",
		Short:  "Execute a scheduled run (internal)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runScheduled(cmd.OutOrStdout(), id, runID, sessionID)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "schedule id")
	cmd.Flags().StringVar(&runID, "run-id", "", "run id")
	cmd.Flags().StringVar(&sessionID, "session", "", "session id to record into")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("run-id")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

func (c *CLI) runScheduled(out io.Writer, id, runID, sessionID string) error {
	if err := c.requireScheduler(); err != nil {
		return err
	}
	s, err := c.sched.Get(id)
	if err != nil {
		return err
	}
	sess, err := c.sessions.Get(sessionID)
	if err != nil {
		return err
	}

	orch, err := agents.New("orchestrator")
	if err != nil {
		return fmt.Errorf("cli: create orchestrator: %w", err)
	}
	orch.Bus = c.bus
	orch.ToolNames = dropToolName(orch.ToolNames, "askuserquestion")

	prov, mdl, err := c.selectModel(sess.Provider, sess.Model, false)
	if err != nil {
		return fmt.Errorf("cli: select model: %w", err)
	}
	orch.SetModelProvider(mdl, prov)

	msgs, err := c.sessions.Messages(sessionID)
	if err != nil {
		return fmt.Errorf("cli: get session messages: %w", err)
	}
	history := make([]llm.Message, 0, len(msgs)+1)
	if orch.SystemPrompt != "" {
		history = append(history, llm.SystemMessage(orch.SystemPrompt))
	}
	history = append(history, session.MessagesToLLM(msgs)...)

	c.bus.Publish(session.TopicSessionResume, session.SessionResume{
		AgentID:   orch.ID,
		SessionID: sessionID,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if timeout := s.TimeoutValue(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	rec := schedule.RunRecord{
		RunID:      runID,
		ScheduleID: id,
		SessionID:  sessionID,
		Provider:   prov.Name(),
		Model:      mdl.ID,
		Status:     schedule.StatusSuccess,
		StartedAt:  time.Now(),
	}
	if existing, lerr := c.sched.Store().LoadRun(id, runID); lerr == nil {
		rec.StartedAt = existing.StartedAt
		rec.Trigger = existing.Trigger
		rec.PID = os.Getpid()
	}

	result, runErr := orch.ResumeStream(ctx, s.Prompt, history, func(agent.StreamEvent) error { return nil })
	if result != nil {
		rec.Usage = result.Usage
	}
	switch {
	case runErr != nil && errors.Is(runErr, context.DeadlineExceeded):
		rec.Status = schedule.StatusTimeout
		rec.Error = "scheduled run timed out"
	case runErr != nil && ctx.Err() == context.DeadlineExceeded:
		rec.Status = schedule.StatusTimeout
		rec.Error = "scheduled run timed out"
	case runErr != nil:
		rec.Status = schedule.StatusFailed
		rec.Error = runErr.Error()
	}
	if err := c.sched.FinishRun(rec); err != nil && c.log != nil {
		c.log.Fwarn("schedule: record run finish: %v", err)
	}
	if rec.Status != schedule.StatusSuccess {
		return fmt.Errorf("scheduled run: %s", rec.Error)
	}
	fmt.Fprintf(out, "scheduled run %s completed\n", runID)
	return nil
}

func printSchedule(out io.Writer, s schedule.Schedule) {
	status := "active"
	if !s.Enabled {
		status = "paused"
	}
	next := "-"
	if s.NextRun != nil {
		next = s.NextRun.Format("2006-01-02 15:04:05")
	}
	fmt.Fprintf(out, "%s\n", s.ID)
	fmt.Fprintf(out, "  name:      %s\n", s.Name)
	fmt.Fprintf(out, "  status:    %s\n", status)
	fmt.Fprintf(out, "  cron:      %s\n", s.Cron)
	fmt.Fprintf(out, "  next run:  %s\n", next)
	fmt.Fprintf(out, "  workspace: %s\n", s.Workspace)
	if s.Provider != "" || s.Model != "" {
		fmt.Fprintf(out, "  model:     %s/%s\n", s.Provider, s.Model)
	}
	if len(s.Tags) > 0 {
		fmt.Fprintf(out, "  tags:      %s\n", strings.Join(s.Tags, ", "))
	}
	fmt.Fprintf(out, "  prompt:    %s\n", truncate(strings.ReplaceAll(s.Prompt, "\n", " "), 80))
}

func promptStringDefault(label, def string) (string, error) {
	p := promptui.Prompt{Label: label, Default: def}
	result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("cli: %s: %w", label, err)
	}
	return strings.TrimSpace(result), nil
}

type cronPreset struct {
	label string
	expr  string
}

var cronPresets = []cronPreset{
	{"Every minute", "* * * * *"},
	{"Every 5 minutes", "*/5 * * * *"},
	{"Every 15 minutes", "*/15 * * * *"},
	{"Every 30 minutes", "*/30 * * * *"},
	{"Every hour", "0 * * * *"},
	{"Every 6 hours", "0 */6 * * *"},
	{"Every day at midnight", "0 0 * * *"},
	{"Every day at 9am", "0 9 * * *"},
	{"Every weekday at 9am", "0 9 * * 1-5"},
	{"Every week (Monday 9am)", "0 9 * * 1"},
	{"Every month (1st, midnight)", "0 0 1 * *"},
}

const cronCustomLabel = "Custom expression"

func (c *CLI) promptCron(out io.Writer) (string, error) {
	fmt.Fprintln(out, "Cron schedule:")
	for i, p := range cronPresets {
		fmt.Fprintf(out, "  %2d) %-28s %s\n", i+1, p.label, p.expr)
	}
	custom := len(cronPresets) + 1
	fmt.Fprintf(out, "  %2d) %s\n", custom, cronCustomLabel)

	sel, err := promptStringDefault(fmt.Sprintf("Select [1-%d] or type an expression", custom), "1")
	if err != nil {
		return "", err
	}
	sel = strings.TrimSpace(sel)
	if sel == "" {
		sel = "1"
	}

	if n, err := strconv.Atoi(sel); err == nil {
		switch {
		case n >= 1 && n <= len(cronPresets):
			return cronPresets[n-1].expr, nil
		case n == custom:
			return promptString("Cron expression (5 fields: minute hour day-of-month month day-of-week)")
		default:
			return "", fmt.Errorf("cli: invalid selection %d (choose 1-%d)", n, custom)
		}
	}

	if strings.ContainsAny(sel, "*/,-") {
		return sel, nil
	}
	return "", fmt.Errorf("cli: invalid selection %q", sel)
}

func dropToolName(names []string, drop string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

func scheduleHasAnyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}
