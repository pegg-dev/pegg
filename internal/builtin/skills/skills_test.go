package skills

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/peggco/pegg/internal/agent/prompt"
)

func TestBuiltinSkillsBuildAndParse(t *testing.T) {
	for name, build := range All() {
		out, err := build().Build(prompt.FormatMarkdown)
		if err != nil {
			t.Fatalf("skill %q: build: %v", name, err)
		}
		if !strings.HasPrefix(out, "---\n") {
			t.Fatalf("skill %q: missing frontmatter:\n%s", name, out)
		}
		body := out[len("---\n"):]
		end := strings.Index(body, "\n---")
		if end < 0 {
			t.Fatalf("skill %q: unclosed frontmatter:\n%s", name, out)
		}
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			WhenToUse   string `yaml:"when_to_use"`
			Context     string `yaml:"context"`
		}
		if err := yaml.Unmarshal([]byte(body[:end]), &meta); err != nil {
			t.Fatalf("skill %q: frontmatter yaml: %v", name, err)
		}
		if meta.Name != name {
			t.Errorf("skill %q: frontmatter name = %q", name, meta.Name)
		}
		if meta.Description == "" {
			t.Errorf("skill %q: empty description", name)
		}
		if meta.WhenToUse == "" {
			t.Errorf("skill %q: empty when_to_use", name)
		}
	}
}

func TestSkillifyMentionsPeggPaths(t *testing.T) {
	out := SkillifySkill().MustBuild(prompt.FormatMarkdown)
	for _, want := range []string{".pegg/skills/", "~/.pegg/skills/", "askuserquestion", "SKILL.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("skillify output missing %q", want)
		}
	}
	if strings.Contains(out, ".claude") {
		t.Error("skillify output should not reference .claude paths")
	}
}
