package skills

import (
	"github.com/peggco/pegg/internal/agent/prompt"
)

func InitSkill() *prompt.Prompt {
	return prompt.New().
		Hr(3).
		Paragraph("name: init").
		Paragraph("description: Instructs the /init command to analyze a codebase and create or improve a AGENTS.md file.").
		Paragraph("when_to_use: Use when user requested to create or improve a AGENTS.md file.").
		Paragraph("context: inline").
		Hr(3).
		Paragraph("").
		Paragraph("Analyze this codebase and create a AGENTS.md file, which will be given to future instances of Pegg to operate in this repository.").
		Paragraph("About AGENTS.md:").
		Paragraph("Modern AI coding tools converge on a simple idea: give the agent a single, well-structured Markdown file that explains how your repo \"works,\" and prepend that file to every LLM call so the agent never has to guess about architecture, commands, or conventions. Community gists, RFCs, and vendor playbooks all recommend the same core sections\u2014overview, project map, build/test scripts, code style, security, and other relevant information. This file is intended to be that single source of truth for Pegg.").
		Paragraph("What to add:").
		OrderedList(
			"Commands that will be commonly used, such as how to build, lint, and run tests. Include the necessary commands to develop in this codebase, such as how to run a single test.",
			"High-level code architecture and structure so that future instances can be productive more quickly. Focus on the \"big picture\" architecture that requires reading multiple files to understand.",
		).
		Paragraph("Usage Notes:").
		List(
			"If there's already a AGENTS.md, suggest improvements to it.",
			"When you make the initial AGENTS.md, do not repeat yourself and do not include obvious instructions like \"Provide helpful error messages to users\", \"Write unit tests for all new utilities\", \"Never include sensitive information (API keys, tokens) in code or commits\".",
			"Avoid listing every component or file structure that can be easily discovered.",
			"Don't include generic development practices.",
			"If there are Cursor rules (in .cursor/rules/ or .cursorrules) or Copilot rules (in .github/copilot-instructions.md), make sure to include the important parts.",
			"If there is a README.md, make sure to include the important parts.",
			"Do not make up information such as \"Common Development Tasks\", \"Tips for Development\", \"Support and Documentation\" unless this is expressly included in other files that you read.",
		).
		Paragraph("Be sure to prefix the file with the following text:").
		Code("", "# AGENTS.md\n\nThis file provides guidance to Pegg when working with code in this repository.").
		Paragraph("Required Sections:").
		OrderedList(
			"Project Overview \u2013 one-paragraph description and elevator pitch.",
			"Repository Structure \u2013 bullet list mirroring the directory tree; explain each top-level folder in \u2264 1 sentence.",
			"Build & Development Commands \u2013 shell-ready commands for install, test, lint, type-check, run, debug, deploy; use fenced code blocks.",
			"Code Style & Conventions \u2013 formatting rules, naming patterns, lint config, commit-message template.",
			"Architecture Notes \u2013 high-level diagram in Mermaid **or** ASCII plus a prose explanation of major components and data flow.",
			"Testing Strategy \u2013 unit, integration, e2e tools and how to run them locally + in CI.",
			"Security & Compliance \u2013 secrets handling, dependency-scanning, guardrails, license notes.",
			"Agent Guardrails \u2013 boundaries for automated agents (files never touched, required reviews, rate limits).",
			"Extensibility Hooks \u2013 plugin points, env vars, feature flags.",
			"Further Reading \u2013 relative links to deeper docs (docs/ARCH.md, ADRs, etc.).",
		).
		Paragraph("Style & Rules").
		List(
			"Write in concise, direct English; max line length \u2248 100 chars.",
			"Use Markdown only\u2014no HTML.",
			"Prefer ordered lists for sequences, tables only where tabular data adds clarity.",
			"Do NOT invent details; if information is missing, insert a `> TODO:` marker.",
			"Keep total tokens \u2264 12 k. If input tree is huge, summarise less-critical sub-dirs.",
			"Preserve any existing build commands verbatim.",
		)
}
