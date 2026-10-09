package plan

import "github.com/peggco/pegg/internal/agent/prompt"

func exitplanmodeToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Use this tool when you are in plan mode and have finished planning and are ready for user approval.").
		Heading(2, "How This Tool Works").
		List("The plan should already exist - the planner subagent wrote it to a file under .pegg/plans/ during plan mode",
			"This tool does NOT take the plan content as a parameter - it reads the plan file written during plan mode",
			"This tool asks the user to review and approve the plan; the plan file path is shown in the approval prompt when found",
			"If the user approves, plan mode ends and you may start implementing",
			"If the user rejects, plan mode stays active and their feedback is returned so you can revise the plan").
		Heading(2, "When to Use This Tool").
		Paragraph("IMPORTANT: Only use this tool when the task requires planning the implementation steps of a task that requires writing code. For research tasks where you're gathering information, searching files, reading files or in general trying to understand the codebase - do NOT use this tool.").
		Heading(2, "Before Using This Tool").
		Paragraph("Ensure your plan is complete and unambiguous:").
		List("If you have unresolved questions about requirements or approach, use askuserquestion first (in earlier phases)",
			"Once your plan is finalized, use THIS tool to request approval").
		Paragraph("**Important:** Do NOT use askuserquestion to ask 'Is this plan okay?' or 'Should I proceed?' - that's exactly what THIS tool does. ExitPlanMode inherently requests user approval of your plan.").
		Heading(2, "Examples").
		OrderedList("Initial task: 'Search for and understand the implementation of vim mode in the codebase' - Do not use the exit plan mode tool because you are not planning the implementation steps of a task.",
			"Initial task: 'Help me implement yank mode for vim' - Use the exit plan mode tool after you have finished planning the implementation steps of the task.",
			"Initial task: 'Add a new feature to handle user authentication' - If unsure about auth method (OAuth, JWT, etc.), use askuserquestion first, then use exit plan mode tool after clarifying the approach.")
}
