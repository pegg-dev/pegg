package skills

import (
	"github.com/peggco/pegg/internal/agent/prompt"
)

func BatchSkill() *prompt.Prompt {
	return prompt.New().
		Hr(3).
		Paragraph("name: batch").
		Paragraph("description: Instructs the /batch command. Instructions for orchestrating a large, parallelizable change across a codebase. Use it to make a change to many files at once, such as building an application from scratch, or performing a major refactoring. The /batch command is designed for large-scale code modifications that can be executed in parallel, ensuring efficiency and consistency across the codebase.").
		Paragraph("when_to_use: Use when the user wants to make a large-scale change across many files in parallel, such as a major refactoring, building an application from scratch, or performing a bulk operation that can be decomposed into independent work units.").
		Paragraph("context: inline").
		Hr(3).
		Heading(1, "Batch: Parallel Work Orchestration").
		Paragraph("You are orchestrating a large, parallelizable change across this codebase.").
		Heading(1, "Phase 1: Research and Plan (Plan Mode)").
		Paragraph("Call the `enter_plan_mode` tool now to enter plan mode, then:").
		OrderedList(
			prompt.ListItem("Understand the scope.",
				prompt.List("Launch one or more subagents (in the foreground — you need their results) to deeply research what this instruction touches. Find all the files, patterns, and call sites that need to change. Understand the existing conventions so the migration is consistent.")),
			prompt.ListItem("Decompose into independent units.",
				prompt.List("Break the work into 5–30 self-contained units. Each unit must:",
					"Be independently implementable in an isolated git worktree (no shared state with sibling units)",
					"Be mergeable on its own without depending on another unit's PR landing first",
					"Be roughly uniform in size (split large units, merge trivial ones)",
					"Scale the count to the actual work: few files → closer to 5; hundreds of files → closer to 30. Prefer per-directory or per-module slicing over arbitrary file lists.")),
			prompt.ListItem("Determine the e2e test recipe.",
				prompt.List("Figure out how a worker can verify its change actually works end-to-end — not just that unit tests pass. Look for:",
					"A `claude-in-chrome` skill or browser-automation tool (for UI changes: click through the affected flow, screenshot the result)",
					"A `tmux` or CLI-verifier skill (for CLI changes: launch the app interactively, exercise the changed behavior)",
					"A dev-server + curl pattern (for API changes: start the server, hit the affected endpoints)",
					"An existing e2e/integration test suite the worker can run",
					"If you cannot find a concrete e2e path, use the `ask_user_question` tool to ask the user how to verify this change end-to-end. Offer 2–3 specific options based on what you found. Do not skip this — the workers cannot ask the user themselves.",
					"Write the recipe as a short, concrete set of steps that a worker can execute autonomously. Include any setup (start a dev server, build first) and the exact command/interaction to verify.")),
			prompt.ListItem("Write the plan.",
				prompt.List("In your plan file, include:",
					"A summary of what you found during research",
					"A numbered list of work units — for each: a short title, the list of files/directories it covers, and a one-line description of the change",
					"The e2e test recipe (or \"skip e2e because …\" if the user chose that)",
					"The exact worker instructions you will give each agent (the shared template)")),
			prompt.ListItem("Call `exit_plan_mode` to present the plan for approval.")).
		Heading(1, "Phase 2: Spawn Workers (After Plan Approval)").
		Paragraph("Once the plan is approved, spawn one background agent per work unit using the `task` tool. **All agents must use `isolation: \"worktree\"` and `run_in_background: true`.** Launch them all in a single message block so they run in parallel.").
		Paragraph("For each agent, the prompt must be fully self-contained. Include:").
		List("The overall goal (the user's instruction)",
			"This unit's specific task (title, file list, change description — copied verbatim from your plan)",
			"Any codebase conventions you discovered that the worker needs to follow",
			"The e2e test recipe from your plan (or \"skip e2e because …\")",
			"The worker instructions below, copied verbatim").
		Paragraph("Use `subagent_type: \"developer\"` unless a more specific agent type fits.").
		Heading(1, "Phase 3: Track Progress").
		Paragraph("After launching all workers, render an initial status table:").
		Code("markdown", "| # | Unit | Status | PR |\n|---|------|--------|----|\n| 1 | <title> | running | — |\n| 2 | <title> | running | — |").
		Paragraph("As background-agent completion notifications arrive, parse the `PR: <url>` line from each agent's result and re-render the table with updated status (`done` / `failed`) and PR links. Keep a brief failure note for any agent that did not produce a PR.").
		Paragraph("When all agents have reported, render the final table and a one-line summary (e.g., \"22/24 units landed as PRs\").")
}
