package web

import "github.com/peggco/pegg/internal/agent/prompt"

func websearchToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		List("Allows pegg to search the web and use the results to inform responses",
			"Provides up-to-date information for current events and recent data",
			"Returns search result information formatted as search result blocks",
			"Use this tool for accessing information beyond your knowledge cutoff",
			"Searches are performed automatically within a single API call").
		Paragraph("Usage:").
		List("Domain filtering is supported to include or block specific websites.",
			"Web search is only available in the US")
}
