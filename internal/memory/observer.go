package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/llm"
)

func generateObserverSystemPrompt() (string, error) {
	return prompt.New().
		Paragraph("You are the memory observer of an AI coding assistant. A primary agent used tools to work on a task. You distill each tool use into a compact, durable observation that future sessions can reuse.").
		XMLTag("task",
			prompt.Paragraph("You are given one or more tool uses, each wrapped in an <observed_from_primary_session> block."),
			prompt.Paragraph("For each tool use that contains something durable and non-obvious, emit one <observation> block."),
			prompt.Paragraph("Skip noise: trivial file reads of already-known files, placeholder edits, shell commands that produced nothing, and anything already captured by an earlier observation in the same batch."),
			prompt.Paragraph("Return either one or more <observation>...</observation> blocks, or <skip_summary reason=\"noise\"/>. Never reply with prose.")).
		XMLTag("observation_types",
			prompt.KV("bugfix", "a defect was diagnosed and fixed"),
			prompt.KV("feature", "a new capability was added"),
			prompt.KV("refactor", "code was restructured without changing behavior"),
			prompt.KV("change", "behavior or configuration was changed"),
			prompt.KV("discovery", "something non-obvious was learned about the codebase or environment"),
			prompt.KV("decision", "a design or technical decision was made and why"),
			prompt.KV("security_alert", "a vulnerability or security problem was found"),
			prompt.KV("security_note", "a security-relevant fact, e.g. a permission model"),
			prompt.KV("sensitive", "sensitive user information or secrets were handled")).
		XMLTag("observation_schema",
			prompt.KV("type", "one of the types above"),
			prompt.KV("title", "short, specific summary (max 10 words)"),
			prompt.KV("subtitle", "optional one-line elaboration"),
			prompt.KV("facts", "bullet list of concrete facts: file paths, identifiers, commands, error text, status codes, exact values"),
			prompt.KV("concepts", "tags this observation relates to, e.g. auth, caching, deployment"),
			prompt.KV("narrative", "2-4 sentences explaining what happened and why it matters; keep file paths and identifiers intact")).
		XMLTag("example",
			prompt.Raw("<observation>\n  <type>bugfix</type>\n  <title>Fixed auth token refresh</title>\n  <subtitle>401 on expired refresh tokens</subtitle>\n  <facts>\n    <fact>internal/auth/token.go: refresh endpoint returned 401 on expired refresh tokens</fact>\n    <fact>Added retry-once with a fresh token request</fact>\n  </facts>\n  <concepts>\n    <concept>auth</concept>\n    <concept>tokens</concept>\n  </concepts>\n  <narrative>The refresh endpoint rejected expired refresh tokens with a 401. The fix requests a new token once and retries the original call before surfacing the error.</narrative>\n</observation>")).
		Build(prompt.FormatMarkdown)
}

type observedUse struct {
	ToolName   string
	At         time.Time
	WorkingDir string
	Params     string
	Outcome    string
}

func buildObservationInput(uses []observedUse) (string, error) {
	var parts []prompt.Part
	for _, u := range uses {
		parts = append(parts, prompt.XMLTag("observed_from_primary_session",
			prompt.KV("what_happened", u.ToolName),
			prompt.KV("occurred_at", u.At.Format(time.RFC3339)),
			prompt.KV("working_directory", u.WorkingDir),
			prompt.KV("parameters", u.Params),
			prompt.KV("outcome", u.Outcome),
		))
	}
	p := prompt.New().
		Paragraph("Distill the following tool uses into observations.").
		XMLTag("batch", parts...)
	return p.Build(prompt.FormatMarkdown)
}

func generateSummarySystemPrompt() (string, error) {
	return prompt.New().
		Paragraph("You are the memory librarian of an AI coding assistant. You summarize a finished agent run into a durable session summary.").
		XMLTag("task",
			prompt.Paragraph("You are given the user's request and the final assistant message of a completed run."),
			prompt.Paragraph("Return exactly one <summary> block. The root tag MUST be <summary>; any <observation> output is discarded."),
			prompt.Paragraph("If nothing durable happened, return <skip_summary reason=\"nothing durable\"/>.")).
		XMLTag("summary_schema",
			prompt.KV("request", "the user's request in one sentence"),
			prompt.KV("investigated", "what was investigated and what was found"),
			prompt.KV("learned", "durable knowledge gained"),
			prompt.KV("completed", "what was completed"),
			prompt.KV("next_steps", "what remains for future sessions"),
			prompt.KV("notes", "anything else worth remembering")).
		Build(prompt.FormatMarkdown)
}

func buildSummaryInput(t *turn) string {
	p := prompt.New().
		Paragraph("Summarize this finished run.").
		XMLTag("user_request", prompt.Raw(clampTo(t.prompt, maxPromptChars)))
	if t.lastAssistant != "" {
		p = p.XMLTag("final_assistant_message", prompt.Raw(clampTo(t.lastAssistant, 3000)))
	}
	s, _ := p.Build(prompt.FormatMarkdown)
	return s
}

var (
	obsBlockRe = regexp.MustCompile(`<observation>([\s\S]*?)</observation>`)
	skipRe     = regexp.MustCompile(`<skip_summary(?:\s+reason="[^"]*")?\s*/>`)
	fieldRe    = map[string]*regexp.Regexp{
		"type":      regexp.MustCompile(`(?s)<type>\s*(.*?)\s*</type>`),
		"title":     regexp.MustCompile(`(?s)<title>\s*(.*?)\s*</title>`),
		"subtitle":  regexp.MustCompile(`(?s)<subtitle>\s*(.*?)\s*</subtitle>`),
		"narrative": regexp.MustCompile(`(?s)<narrative>\s*(.*?)\s*</narrative>`),
		"facts":     regexp.MustCompile(`(?s)<facts>\s*(.*?)\s*</facts>`),
		"concepts":  regexp.MustCompile(`(?s)<concepts>\s*(.*?)\s*</concepts>`),
	}
	factRe      = regexp.MustCompile(`(?s)<fact>\s*(.*?)\s*</fact>`)
	conceptRe   = regexp.MustCompile(`(?s)<concept>\s*(.*?)\s*</concept>`)
	summaryRe   = regexp.MustCompile(`<summary>([\s\S]*?)</summary>`)
	summaryFlds = map[string]*regexp.Regexp{
		"request":      regexp.MustCompile(`(?s)<request>\s*(.*?)\s*</request>`),
		"investigated": regexp.MustCompile(`(?s)<investigated>\s*(.*?)\s*</investigated>`),
		"learned":      regexp.MustCompile(`(?s)<learned>\s*(.*?)\s*</learned>`),
		"completed":    regexp.MustCompile(`(?s)<completed>\s*(.*?)\s*</completed>`),
		"next_steps":   regexp.MustCompile(`(?s)<next_steps>\s*(.*?)\s*</next_steps>`),
		"notes":        regexp.MustCompile(`(?s)<notes>\s*(.*?)\s*</notes>`),
	}
)

func stripFences(out string) string {
	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "```") {
		if idx := strings.Index(out, "\n"); idx >= 0 {
			out = out[idx+1:]
		}
		if strings.HasSuffix(out, "```") {
			out = strings.TrimSuffix(out, "```")
		}
	}
	return strings.TrimSpace(out)
}

func extractField(re *regexp.Regexp, block string) string {
	m := re.FindStringSubmatch(block)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractItems(blockRe, itemRe *regexp.Regexp, s string) []string {
	if s == "" {
		return nil
	}
	if matches := itemRe.FindAllStringSubmatch(s, -1); len(matches) > 0 {
		var out []string
		for _, m := range matches {
			if v := strings.TrimSpace(m[1]); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	}) {
		if v := strings.TrimSpace(strings.TrimPrefix(part, "-")); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseObservations(output string) ([]Observation, bool) {
	output = stripFences(output)
	if skipRe.MatchString(output) {
		return nil, true
	}
	blocks := obsBlockRe.FindAllStringSubmatch(output, -1)
	if len(blocks) == 0 {
		return nil, false
	}
	var out []Observation
	for _, b := range blocks {
		inner := b[1]
		obs := Observation{
			Type:      extractField(fieldRe["type"], inner),
			Title:     extractField(fieldRe["title"], inner),
			Subtitle:  extractField(fieldRe["subtitle"], inner),
			Narrative: extractField(fieldRe["narrative"], inner),
		}
		if !validObservationType(obs.Type) {
			obs.Type = "change"
		}
		obs.Facts = extractItems(fieldRe["facts"], factRe, extractField(fieldRe["facts"], inner))
		obs.Concepts = extractItems(fieldRe["concepts"], conceptRe, extractField(fieldRe["concepts"], inner))
		if obs.Title == "" {
			if obs.Narrative == "" {
				continue
			}
			first := strings.SplitN(obs.Narrative, "\n", 2)
			obs.Title = clampTo(strings.TrimSpace(first[0]), 120)
			if len(first) > 1 {
				obs.Narrative = first[1]
			}
		}
		obs.Title = clampTo(obs.Title, 150)
		out = append(out, obs)
	}
	return out, len(out) > 0
}

func parseSummary(output string) (Summary, bool) {
	output = stripFences(output)
	if skipRe.MatchString(output) {
		return Summary{}, false
	}
	m := summaryRe.FindStringSubmatch(output)
	if len(m) < 2 {
		return Summary{}, false
	}
	inner := m[1]
	sum := Summary{
		Request:      extractField(summaryFlds["request"], inner),
		Investigated: extractField(summaryFlds["investigated"], inner),
		Learned:      extractField(summaryFlds["learned"], inner),
		Completed:    extractField(summaryFlds["completed"], inner),
		NextSteps:    extractField(summaryFlds["next_steps"], inner),
		Notes:        extractField(summaryFlds["notes"], inner),
	}
	has := sum.Request != "" || sum.Investigated != "" || sum.Learned != "" ||
		sum.Completed != "" || sum.NextSteps != ""
	return sum, has
}

func observationBlock(obs Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s [%s] %s — %s\n", obs.ID, obs.Type, obs.Title,
		obs.At.Format(time.RFC3339))
	if obs.Subtitle != "" {
		fmt.Fprintf(&b, "- subtitle: %s\n", sanitize(obs.Subtitle))
	}
	if len(obs.FilesRead) > 0 || len(obs.FilesMod) > 0 {
		files := append(append([]string{}, obs.FilesMod...), obs.FilesRead...)
		fmt.Fprintf(&b, "- files: %s\n", strings.Join(files, ", "))
	}
	if len(obs.Concepts) > 0 {
		fmt.Fprintf(&b, "- keywords: %s\n", strings.Join(obs.Concepts, ", "))
	}
	if len(obs.Facts) > 0 {
		fmt.Fprintf(&b, "- facts:\n")
		for _, f := range obs.Facts {
			fmt.Fprintf(&b, "  - %s\n", sanitize(f))
		}
	}
	if obs.Session != "" {
		fmt.Fprintf(&b, "- session: %s\n", obs.Session)
	}
	if obs.Agent != "" {
		fmt.Fprintf(&b, "- agent: %s\n", obs.Agent)
	}
	fmt.Fprintf(&b, "- hash: %s\n", obs.Hash)
	if obs.Narrative != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(obs.Narrative))
	}
	return strings.TrimRight(b.String(), "\n")
}

func summaryBlock(sum Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s [summary] Session summary — %s\n", sum.ID,
		sum.At.Format(time.RFC3339))
	write := func(label, val string) {
		if val != "" {
			fmt.Fprintf(&b, "- %s: %s\n", label, sanitize(val))
		}
	}
	write("request", sum.Request)
	write("investigated", sum.Investigated)
	write("learned", sum.Learned)
	write("completed", sum.Completed)
	write("next_steps", sum.NextSteps)
	write("notes", sum.Notes)
	if sum.Session != "" {
		fmt.Fprintf(&b, "- session: %s\n", sum.Session)
	}
	fmt.Fprintf(&b, "- hash: %s\n", sum.Hash)
	return strings.TrimRight(b.String(), "\n")
}

func fileEvidence(name, args string) (read, mod []string) {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return nil, nil
	}
	collect := func(keys ...string) []string {
		var out []string
		for _, k := range keys {
			switch v := m[k].(type) {
			case string:
				if v != "" {
					out = append(out, v)
				}
			case []any:
				for _, e := range v {
					if s, ok := e.(string); ok && s != "" {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
	switch name {
	case "read", "list", "glob", "grep":
		return collect("path", "file_path", "include", "pattern", "file_paths", "filepaths"), nil
	case "write", "edit", "delete":
		return nil, collect("path", "file_path", "file_paths", "filepaths", "target_file")
	case "bash":
		return nil, nil
	}
	return nil, nil
}

func runObserver(ctx context.Context, prov llm.Provider, model llm.Model, input string) (string, error) {
	sys, err := generateObserverSystemPrompt()
	if err != nil {
		sys = "You are the memory observer of an AI coding assistant. Distill tool uses into <observation> XML blocks. Never reply with prose."
	}
	a := agent.New("memory-observer",
		agent.WithProvider(prov),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
	)
	res, err := a.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("memory: observer run: %w", err)
	}
	return res.Output, nil
}

func runSummarizer(ctx context.Context, prov llm.Provider, model llm.Model, input string) (string, error) {
	sys, err := generateSummarySystemPrompt()
	if err != nil {
		sys = "You are the memory librarian of an AI coding assistant. Summarize the run into one <summary> XML block. Never reply with prose."
	}
	a := agent.New("memory-summarizer",
		agent.WithProvider(prov),
		agent.WithModel(model),
		agent.WithSystemPrompt(sys),
		agent.WithMaxIterations(1),
	)
	res, err := a.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("memory: summarizer run: %w", err)
	}
	return res.Output, nil
}
