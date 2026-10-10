---
icon: lucide/calendar-clock
---

# Scheduling

Scheduling runs the agent **automatically on a cron schedule** — daily summaries,
weekly dependency checks, recurring reminders, periodic reports. Schedules persist
across restarts and fire while a long-running Pegg process is active, without a
terminal session of their own.

Every run is saved as its own [session](sessions.md) titled `schedule HH.MM.SS`, so
you can open it later and see exactly what the agent did on that occurrence.

## How it works

A lightweight scheduler starts with every long-running process:

| Process | Runs schedules |
|---|---|
| [TUI](../usage/tui.md) (`pegg`, `pegg tui`) | Yes |
| [HTTP server](../usage/http.md) (`pegg serve`) | Yes |
| [ACP server](../usage/acp.md) (`pegg serve --acp`) | Yes |
| [Connect](connect.md) (`pegg connect`) | Yes |
| One-shot CLI (`pegg run "..."`) | Starts, but the process exits before anything is due |

The scheduler is idle-cheap: one cron timer plus a 5-second check for changes to the
schedule file. When a schedule is due, Pegg:

1. Resolves the model (the schedule's override, else the last model used in that
   workspace, else your preferred model).
2. Creates a session titled `schedule HH.MM.SS` in the schedule's workspace.
3. Runs the agent in a child process rooted at that workspace, delivering the
   schedule's prompt as the user message.
4. Records the run's status, duration, session, and token usage.

Because each run is a separate process, a slow or crashing run never takes down the
scheduler, and a schedule can target any project directory — not just the one Pegg
was started from.

!!! note "Runs are skipped while nothing is running"

    A schedule only fires while one of the processes above is alive. Runs that
    would have happened while Pegg was stopped are **not** replayed on startup.

## Quick start

```bash
# Create a weekday reminder at 9am
pegg schedule create "PR review" \
  --cron "0 9 * * 1-5" \
  --prompt "Review the open pull requests and summarize what needs attention"

# List, inspect, and trigger
pegg schedule list
pegg schedule get <id>
pegg schedule trigger <id>
```

Run `pegg schedule` with no subcommand to list everything.

## Creating schedules

```bash
pegg schedule create "Weekly dependency audit" \
  --cron "0 9 * * 1" \
  --prompt "Check dependencies for vulnerabilities and outdated packages" \
  --workspace /path/to/repo \
  --tags ci,security
```

Any value not passed as an argument or flag is prompted for interactively. When
`--cron` is omitted, Pegg offers a menu of common expressions (every minute, every
hour, every day, every weekday, every week, every month) plus a **Custom
expression** option. You can also type a cron expression directly.

| Flag | Description |
|---|---|
| `--name` | Schedule name (may also be the positional argument) |
| `--cron` | 5-field cron expression (required) |
| `--prompt` | Prompt sent to the agent as a user message on each run (required) |
| `--workspace` | Project directory the run executes in. Defaults to the current directory |
| `--provider` / `--model` | Optional model override; defaults to the last model used in that workspace |
| `--tags` | Comma-separated tags |
| `--timeout` | Seconds before a run is stopped; `0` means no limit |
| `--max-parallel` | Concurrent runs allowed; default `1` |
| `--disabled` | Create the schedule paused |

## Managing schedules

| Command | Description |
|---|---|
| `pegg schedule` / `pegg schedule list [--tags a,b]` | List schedules, optionally filtered by tag |
| `pegg schedule get <id>` | Show one schedule |
| `pegg schedule upcoming` | Preview the next 10 runs |
| `pegg schedule active` | Show currently running executions |
| `pegg schedule trigger <id>` | Run a schedule immediately |
| `pegg schedule pause <id>` / `resume <id>` | Disable / re-enable a schedule |
| `pegg schedule update <id> [flags]` | Change any field, including `--enabled=false` |
| `pegg schedule history <id> [--limit N]` | Past runs with status, duration, session, and error |
| `pegg schedule stats <id>` | Success rate, average duration, last run, last failure |
| `pegg schedule delete <id>` | Remove a schedule (past run sessions are kept) |

## Cron expressions

Cron uses five fields — **minute hour day-of-month month day-of-week** — in the
machine's local time zone. Days and months accept names or numbers, so `MON-FRI` is
the same as `1-5`.

| Expression | Schedule |
|---|---|
| `* * * * *` | Every minute |
| `*/5 * * * *` | Every 5 minutes |
| `*/30 * * * *` | Every 30 minutes |
| `0 * * * *` | Every hour |
| `0 */6 * * *` | Every 6 hours |
| `0 0 * * *` | Every day at midnight |
| `0 9 * * *` | Every day at 9am |
| `0 9 * * 1-5` | Every weekday at 9am |
| `0 9 * * 1` | Every Monday at 9am |
| `0 0 1 * *` | First of the month at midnight |

`@daily`, `@hourly`, and the other descriptors are also accepted.

## How a run behaves

- **Session** — each run is stored as a session titled `schedule HH.MM.SS` with the
  schedule's workspace as its project directory. Open it with
  `pegg sessions list` / `pegg sessions show <id>`, or from the TUI session list.
- **Prompt** — the schedule's prompt is delivered to the agent as a fresh user
  message. The run starts with no memory of the conversation that created it, so
  write the prompt as a complete, self-contained task.
- **Workspace** — the agent's sandbox is rooted at the schedule's workspace, so file
  and shell tools operate inside that project.
- **Permissions** — nobody is present to approve prompts, so tools that would
  normally **ask** you are auto-approved. Tools configured for the **judge**
  (`semi-judge` / `judge`) still go through the judge, and `askuserquestion` is
  unavailable in scheduled runs.
- **Overlap** — with the default `max_parallel` of `1`, a run that is still going
  when the next is due is skipped rather than stacked.
- **Timeout** — when `--timeout` is set, a run exceeding it is stopped and recorded
  as timed out.

## Agent tool

The orchestrator can create and manage schedules itself through a single `schedule`
tool, so you can just ask in chat:

- *"Remind me to review the open PRs every weekday at 9am."*
- *"Do a dependency vulnerability check every Monday."*
- *"Pause the daily summary."*
- *"Run the dependency check right now."*

The tool takes one JSON object with an `action` of `create`, `list`, `get`,
`update`, `delete`, or `run`, plus the relevant fields.

## Configuration

```json
{
  "scheduler": {
    "enabled": true,
    "history_limit": 50
  }
}
```

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Start the background scheduler with the app |
| `store_dir` | string | `~/.pegg` | Directory holding `schedules.json` and run history |
| `history_limit` | int | `50` | Maximum run records kept per schedule |

See [Config](../config.md#scheduler) for the reference entry.

## Storage

| Path | Contents |
|---|---|
| `~/.pegg/schedules.json` | All schedules |
| `~/.pegg/schedule_runs/<schedule-id>/<run-id>.json` | One record per run (status, duration, session, usage) |

Schedules are global (not per-project): each one carries its own `workspace`, so a
single machine can schedule work across many projects.

## SDK

The [Go SDK](../../sdk/index.md) starts a scheduler too, running due tasks
in-process through `Engine.Chat`. Disable it with `Options.DisableScheduler` and
access it with `Engine.Scheduler()`.

```go
eng, _ := sdk.Open(ctx, sdk.Options{
    APIKeys:          map[string]string{"openai": os.Getenv("OPENAI_API_KEY")},
    Workspace:        ".",
    DisableScheduler: true,
})
```

## Troubleshooting

| Symptom | What it means | What to do |
|---|---|---|
| A schedule never fires | No long-running Pegg process is alive | Start `pegg connect`, the TUI, or `pegg serve` |
| `schedule: "…" not found` | Wrong id | Run `pegg schedule list` and copy the id |
| A run is marked `failed` immediately | Model resolution or the workspace path failed | Check `pegg schedule history <id>` and that the workspace exists |
| A due run was skipped | A previous run was still going (`max_parallel`) | Wait, or raise `--max-parallel` |
| Missed runs after downtime | Schedules don't catch up | Nothing to do; the next occurrence runs normally |
