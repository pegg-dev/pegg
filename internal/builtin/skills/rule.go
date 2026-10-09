package skills

import (
	"github.com/peggco/pegg/internal/agent/prompt"
)

func RuleSkill() *prompt.Prompt {
	return prompt.New().
		Hr(3).
		Paragraph("name: rule").
		Paragraph("description: Creates or updates a rule file in the project `.pegg/rules/` or global `~/.pegg/rules/` directory. Rules are loaded into every future conversation, so Pegg keeps following them. Use when the user asks to add a rule, save a preference or convention, or wants Pegg to remember something across conversations.").
		Paragraph("when_to_use: Use when the user explicitly asks to create, update, or remove a rule; when the user says something like \"remember this\", \"always do X\", \"never do Y\", or \"add a rule\"; or when the user repeatedly emphasizes a preference or correction that should be persisted as guidance.").
		Paragraph("context: inline").
		Hr(3).
		Heading(1, "Rule Creation").
		Paragraph("Create or update a rule file that is injected into every future conversation. Rules are loaded from `<project>/.pegg/rules/*.md` (project scope) and `~/.pegg/rules/*.md` (global scope), in alphabetical filename order, and take effect immediately.").
		Heading(2, "Step 1: Determine the rule content").
		OrderedList(
			prompt.ListItem("If the user stated the rule explicitly (e.g. \"always use tabs\", \"never commit .env\"), use their words. Do not paraphrase away specifics.",
				prompt.List("If the user gave a general intent but no exact wording, restate it as concise, imperative guidance and confirm with the user before writing.")),
			prompt.ListItem("If the user did not say what the rule should be, call the `askuserquestion` tool to ask.",
				prompt.List("Ask an open text question: \"What rule should I save?\".",
					"Optionally offer candidate rules you inferred from the conversation as select options, e.g. based on instructions the user repeated, corrections they made, or conventions they emphasized. Mark each option with a short rule text.")),
			prompt.ListItem("The agent may propose rules on its own only when the user asked for that (e.g. \"add a rule based on our conversation\"). Scan the conversation for repeated preferences and corrections, draft 1-3 candidate rules, and present them with `askuserquestion` (select type) for the user to pick or edit.",
				prompt.List("Never write a rule the user has not seen. Always confirm agent-proposed rules with the user before writing them to disk.")),
		).
		Heading(2, "Step 2: Determine the scope").
		OrderedList(
			"Project rule: write to `<project>/.pegg/rules/`. Choose this by default when the rule is specific to this codebase (its language, framework, layout, or workflow).",
			"Global rule: write to `~/.pegg/rules/`. Choose this when the rule is a general preference that applies to every project (coding style, commit style, how to handle secrets).",
			"If the scope is not clear from the user's request, ask with `askuserquestion`: select type with options \"Project (this repo only)\" and \"Global (all projects)\".",
		).
		Heading(2, "Step 3: Choose the filename").
		Paragraph("Use `name.md` where `name` is a short, descriptive, kebab-case topic (e.g. `go-conventions.md`, `commit-messages.md`, `secrets.md`).").
		List(
			"List the existing rule files in both directories first. If a rule on the same topic already exists, edit that file instead of creating a duplicate.",
			"Never overwrite an existing rule with different content without asking the user first.",
		).
		Heading(2, "Step 4: Write the rule file").
		Paragraph("Write the file with the `write` tool. Rule files are plain Markdown:").
		List(
			"Start with a single `# Title` heading.",
			"Use short, imperative sentences: \"Always ...\", \"Never ...\", \"Prefer ... over ...\".",
			"Keep each file focused on one cohesive rule or a tightly related set; do not write an essay.",
			"Do not add YAML frontmatter, timestamps, or meta-commentary — the file content is injected verbatim into the system prompt.",
			"Include concrete examples only when they remove ambiguity.",
		).
		Heading(2, "Step 5: Verify").
		Paragraph("After writing, read the file back and confirm the content matches what the user approved. Tell the user where the rule was saved and that it now applies to future conversations.")
}
