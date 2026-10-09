package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/llm"
)

func generateLibrarianSystemPrompt() (string, error) {
	return prompt.New().
		Paragraph("You are the memory librarian of an AI coding assistant. You consolidate raw session observations into a small, high-signal memory bank of markdown files.").
		XMLTag("task",
			prompt.Paragraph("You are given the current contents of four memory files and the raw observations from the latest session."),
			prompt.Paragraph("Merge the new durable information into the existing files. Rewrite each file completely."),
			prompt.Paragraph("Rules:"),
			prompt.List(
				"Preserve ALL existing durable entries — merge, never drop unless clearly duplicated",
				"Drop noise: placeholder edits, trivial read-only facts, already-known information",
				"Deduplicate: if a new entry restates an existing one, keep the more complete version",
				"active_context.md holds the CURRENT task state: what is in progress, what was completed, and concrete next steps. It is short (max 12 lines)",
				"decisions.md holds durable technical decisions and why they were made (max 3KB)",
				"lessons.md holds gotchas, patterns, and trade-offs learned (max 3KB)",
				"notes.md holds architecture facts, APIs, conventions, and glossary terms (max 3KB)",
				"If a lesson would be useful in ANY project, prefix that bullet with \"[global]\"",
				"Keep bullet points terse; preserve file paths, identifiers, and commands exactly",
			)).
		XMLTag("output_contract",
			prompt.Paragraph("Return exactly one <memory_update> block containing four sections. Each section holds the FULL new markdown content of that file (headings and bullets). Leave a section empty to keep the existing file unchanged. Never reply with prose."),
			prompt.Raw("<memory_update>\n  <active_context>...full markdown...</active_context>\n  <decisions>...full markdown...</decisions>\n  <lessons>...full markdown...</lessons>\n  <notes>...full markdown...</notes>\n</memory_update>")).
		Build(prompt.FormatMarkdown)
}

type memoryUpdate struct {
	ActiveContext string
	Decisions     string
	Lessons       string
	Notes         string
}

var (
	updateRe = regexp.MustCompile(`<memory_update>([\s\S]*?)</memory_update>`)
	fileRe   = map[string]*regexp.Regexp{
		"active_context": regexp.MustCompile(`(?s)<active_context>\s*(.*?)\s*</active_context>`),
		"decisions":      regexp.MustCompile(`(?s)<decisions>\s*(.*?)\s*</decisions>`),
		"lessons":        regexp.MustCompile(`(?s)<lessons>\s*(.*?)\s*</lessons>`),
		"notes":          regexp.MustCompile(`(?s)<notes>\s*(.*?)\s*</notes>`),
	}
)

func parseMemoryUpdate(output string) (memoryUpdate, bool) {
	output = stripFences(output)
	m := updateRe.FindStringSubmatch(output)
	if len(m) < 2 {
		return memoryUpdate{}, false
	}
	inner := m[1]
	return memoryUpdate{
		ActiveContext: strings.TrimSpace(extractField(fileRe["active_context"], inner)),
		Decisions:     strings.TrimSpace(extractField(fileRe["decisions"], inner)),
		Lessons:       strings.TrimSpace(extractField(fileRe["lessons"], inner)),
		Notes:         strings.TrimSpace(extractField(fileRe["notes"], inner)),
	}, true
}

func buildLibrarianInput(blocks []string, activeCtx, decisions, lessons, notes string) (string, error) {
	return prompt.New().
		Paragraph("Consolidate the latest session's raw observations into the memory bank.").
		XMLTag("current_active_context", prompt.Raw(activeCtx)).
		XMLTag("current_decisions", prompt.Raw(decisions)).
		XMLTag("current_lessons", prompt.Raw(lessons)).
		XMLTag("current_notes", prompt.Raw(notes)).
		XMLTag("new_session_observations", prompt.Raw(strings.Join(blocks, "\n\n"))).
		Build(prompt.FormatMarkdown)
}

func runLibrarian(ctx context.Context, prov llm.Provider, model llm.Model, input string) (string, error) {
	sys, err := generateLibrarianSystemPrompt()
	if err != nil {
		sys = "You are the memory librarian of an AI coding assistant. Merge raw observations into the memory bank files. Return one <memory_update> block. Never reply with prose."
	}
	a := agent.New("memory-librarian",
		agent.WithProvider(prov),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
	)
	res, err := a.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("memory: librarian run: %w", err)
	}
	return res.Output, nil
}
