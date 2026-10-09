package file

import "github.com/peggco/pegg/internal/agent/prompt"

func writeToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Writes a file to the local filesystem.").
		Paragraph("Usage:").
		List("This tool will overwrite the existing file if there is one at the provided path.",
			"If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.",
			"ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.",
			"NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.",
			"Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.")
}
