package compaction

import "github.com/peggco/pegg/internal/agent/prompt"

func summarizerPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("You are a conversation summarizer for an AI coding assistant.").
		Paragraph("Your task is to summarize the following conversation history into a concise, information-dense summary.").
		XMLTag("rules",
			prompt.List("Preserve: file paths mentioned, decisions made, task progress, errors encountered, key context",
				"Preserve: tool names used and their results (summarized)",
				"Preserve: any constraints or requirements mentioned",
				"Output ONLY the summary text, no preamble or explanation",
				"Keep the summary under 500 words",
				"Use bullet points for key items when appropriate"))
}
