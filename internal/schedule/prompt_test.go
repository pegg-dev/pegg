package schedule

import (
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent/prompt"
)

func TestScheduleToolPrompt(t *testing.T) {
	for _, format := range []prompt.Format{prompt.FormatMarkdown, prompt.FormatXML, prompt.FormatJSON} {
		out, err := scheduleToolPromptBuilder().Build(format)
		if err != nil {
			t.Fatalf("build %s: %v", format, err)
		}
		if strings.TrimSpace(out) == "" {
			t.Fatalf("empty prompt for %s", format)
		}
	}

	out, err := scheduleToolPromptBuilder().Build(prompt.FormatMarkdown)
	if err != nil {
		t.Fatalf("build markdown: %v", err)
	}
	for _, want := range []string{
		"Remind me",
		"every day",
		"Do a dependency vulnerability check every Monday",
		"action",
		"Cron Expressions",
		"Guidelines",
		"Examples",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
