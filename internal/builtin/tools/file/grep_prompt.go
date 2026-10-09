package file

import "github.com/peggco/pegg/internal/agent/prompt"

func grepToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		List("Fast content search tool that works with any codebase size",
			"Searches file contents using regular expressions",
			`Supports full regex syntax (eg. 'log.*Error', 'function\s+\w+', etc.)`,
			"Filter files by pattern with the include parameter (eg. '*.js', '*.{ts,tsx}')",
			"Returns file paths with at least one match sorted by modification time",
			"Use this tool when you need to find files containing specific patterns",
			"If you need to identify/count the number of matches within files, use the 'files_with_matches' mode.",
			"When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Task tool instead")
}
