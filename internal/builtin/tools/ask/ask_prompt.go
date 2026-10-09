package ask

import "github.com/peggco/pegg/internal/agent/prompt"

func askToolPromptBuilder() *prompt.Prompt {
	return prompt.New().
		Paragraph("Use this tool only when you are blocked on a decision that is genuinely the user's to make: one you cannot resolve from the request, the code, or sensible defaults.").
		Paragraph("Usage:").
		List("Users will always be able to select 'Other' to provide custom text input.",
			"If you recommend a specific option, make that the first option in the list and add '(Recommended)' at the end of the label.").
		Paragraph("Each entry's type must be one of \"text\", \"select\", \"multiselect\" or \"boolean\". \"select\" and \"multiselect\" entries must include a non-empty options array; \"boolean\" renders as Yes/No and needs no options. The arguments must match this JSON shape:").
		Code("json", askExpectedJSON)
}
