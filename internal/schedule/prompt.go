package schedule

import "github.com/peggco/pegg/internal/agent/prompt"

func scheduleToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Create and manage recurring background schedules. A schedule runs you automatically on a cron expression, in a chosen project directory, and delivers its prompt to you as a fresh user message on every run. Each run is saved as its own session titled \"schedule HH.MM.SS\", so the user can review exactly what you did on each occurrence.").
		Paragraph("Reach for this tool whenever the user wants something to happen repeatedly, on a timer, or later without them present: reminders, recurring checks, periodic reports, daily or weekly summaries, and automated maintenance. Wording such as \"every ...\", \"each ...\", \"daily\", \"weekly\", \"every Monday\", \"at 9am\", \"on the first of the month\", or \"remind me to ...\" all signal that a schedule is wanted.").
		Paragraph("Do NOT use this tool for a one-off task the user wants done right now — just do it directly. A schedule always repeats on its cron expression; it cannot fire a single time. If the user asks for a one-off future reminder, tell them the schedule will repeat and confirm the recurrence.").
		Heading(2, "Actions").
		Paragraph("Every call is a single JSON object with an \"action\" field:").
		OrderedList(
			"create — make a new schedule. Requires name, cron and prompt. Optional: workspace, provider, model, tags, timeout, max_parallel, disabled.",
			"list — show all schedules, optionally filtered by tags.",
			"get — show a single schedule by id.",
			"update — change a schedule. Requires id plus any fields to change (name, cron, prompt, workspace, provider, model, tags, timeout, max_parallel, enabled). Set enabled=false to pause and enabled=true to resume.",
			"delete — remove a schedule by id. Sessions created by its past runs are kept.",
			"run — trigger a schedule immediately, in addition to its normal timing. Requires id.").
		Heading(2, "Fields").
		Table(
			[]string{"Field", "Used by", "Description"},
			[][]string{
				{"action", "all", "One of create, list, get, update, delete, run."},
				{"id", "get, update, delete, run", "Schedule id returned by create or list."},
				{"name", "create, update", "Short human-readable name."},
				{"cron", "create, update", "5-field cron expression (minute hour day-of-month month day-of-week)."},
				{"prompt", "create, update", "Instruction delivered to you as a user message on each run. Write it as a complete, self-contained task."},
				{"workspace", "create, update", "Absolute project directory the run executes in. Defaults to the current directory."},
				{"provider", "create, update", "Optional provider override; defaults to the last model used in that workspace."},
				{"model", "create, update", "Optional model override."},
				{"tags", "create, update, list", "Free-form labels; list can filter by them."},
				{"timeout", "create, update", "Seconds before a run is stopped; 0 means no limit."},
				{"max_parallel", "create, update", "Concurrent runs allowed; default 1."},
				{"enabled", "update", "true resumes, false pauses."},
				{"disabled", "create", "Create the schedule paused."},
			}).
		Heading(2, "Cron Expressions").
		Paragraph("Cron uses five fields: minute hour day-of-month month day-of-week. Times are in the machine's local time zone. Days and months accept names or numbers, so MON-FRI is the same as 1-5.").
		Table(
			[]string{"Schedule", "Expression"},
			[][]string{
				{"Every minute", "* * * * *"},
				{"Every 5 minutes", "*/5 * * * *"},
				{"Every 15 minutes", "*/15 * * * *"},
				{"Every 30 minutes", "*/30 * * * *"},
				{"Every hour", "0 * * * *"},
				{"Every 6 hours", "0 */6 * * *"},
				{"Every day at midnight", "0 0 * * *"},
				{"Every day at 9am", "0 9 * * *"},
				{"Every weekday at 9am", "0 9 * * 1-5"},
				{"Every Monday at 9am", "0 9 * * 1"},
				{"First of the month at midnight", "0 0 1 * *"},
			}).
		Paragraph("Translate the user's wording into the closest expression: \"every night\" -> \"0 0 * * *\", \"every morning\" -> \"0 9 * * *\", \"every Monday morning\" -> \"0 9 * * 1\", \"every weekday\" -> \"0 9 * * 1-5\".").
		Heading(2, "Guidelines").
		OrderedList(
			"Write the prompt as a complete task. The run starts with no memory of this conversation, so include the goal, the project, and what to report back.",
			"Set workspace explicitly when the user names a project; otherwise it defaults to the current directory.",
			"Leave provider and model unset unless the user asks for a specific one.",
			"Use tags to group related schedules (for example \"ci\" or \"reports\").",
			"To pause, change, or delete a schedule when you do not know its id, call list first to find it.",
			"Confirm back to the user in plain language what you scheduled and when it will next run.").
		Heading(2, "Examples").
		XMLTag("example",
			prompt.Paragraph("User: Remind me to review the open PRs every weekday at 9am."),
			prompt.Paragraph("Assistant: I'll set up a weekday reminder to review the open pull requests."),
			prompt.Code("json", `{
  "action": "create",
  "name": "Weekday PR review",
  "cron": "0 9 * * 1-5",
  "prompt": "Review the open pull requests in this repository. List each open PR with its author and review status, flag anything waiting for more than two days, then finish with a short summary.",
  "workspace": "/home/user/projects/app",
  "tags": ["reports"]
}`),
			prompt.Paragraph("Assistant: Done. It will run every weekday at 9am, and each run is saved as its own session.")).
		XMLTag("example",
			prompt.Paragraph("User: Do a dependency vulnerability check every Monday."),
			prompt.Paragraph("Assistant: I'll schedule a weekly dependency audit."),
			prompt.Code("json", `{
  "action": "create",
  "name": "Weekly dependency audit",
  "cron": "0 9 * * 1",
  "prompt": "Check the project dependencies for known vulnerabilities and outdated packages. Report any packages with security advisories, suggest upgrade versions, and note anything that needs manual attention.",
  "tags": ["ci", "security"]
}`),
			prompt.Paragraph("Assistant: Scheduled for every Monday at 9am.")).
		XMLTag("example",
			prompt.Paragraph("User: Every day at 6pm, summarize what changed in the repo."),
			prompt.Paragraph("Assistant: I'll create a daily end-of-day summary."),
			prompt.Code("json", `{
  "action": "create",
  "name": "Daily change summary",
  "cron": "0 18 * * *",
  "prompt": "Summarize today's changes in this repository: commits merged, files changed, and any open issues or failing tests. Keep it concise and end with anything that needs follow-up.",
  "workspace": "/home/user/projects/app"
}`),
			prompt.Paragraph("Assistant: It will run every day at 6pm.")).
		XMLTag("example",
			prompt.Paragraph("User: What schedules do I have?"),
			prompt.Paragraph("Assistant: Let me list them."),
			prompt.Code("json", `{
  "action": "list"
}`),
			prompt.Paragraph("Assistant: You have 3 schedules: \"Weekday PR review\" (weekdays 9am), \"Weekly dependency audit\" (Mondays 9am), and \"Daily change summary\" (daily 6pm).")).
		XMLTag("example",
			prompt.Paragraph("User: Pause the daily summary."),
			prompt.Paragraph("Assistant: Let me find it first, then pause it."),
			prompt.Code("json", `{
  "action": "list"
}`),
			prompt.Paragraph("*Assistant sees the id and pauses the schedule:*"),
			prompt.Code("json", `{
  "action": "update",
  "id": "b1f2c3d4-...",
  "enabled": false
}`),
			prompt.Paragraph("Assistant: Paused \"Daily change summary\". Say the word and I'll resume it.")).
		XMLTag("example",
			prompt.Paragraph("User: Run the dependency check right now."),
			prompt.Paragraph("Assistant: Triggering it immediately."),
			prompt.Code("json", `{
  "action": "run",
  "id": "a9e8d7c6-..."
}`),
			prompt.Paragraph("Assistant: The dependency check is running now. Its normal Monday schedule is unchanged."))
}
