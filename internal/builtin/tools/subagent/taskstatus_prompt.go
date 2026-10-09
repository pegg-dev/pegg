package subagent

import "github.com/peggco/pegg/internal/agent/prompt"

func taskstatusToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Show the status of subagents.").
		Heading(2, "Usage Notes").
		OrderedList("Use this only to see the status of agents running in the background.",
			"Use the `task_names` field to filter and use only the specific task status you need based on its name. Do not use it for tasks you know have finished or those that do not run in the background.")
}
