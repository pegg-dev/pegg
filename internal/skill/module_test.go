package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
)

func TestMaterializeTo(t *testing.T) {
	root := t.TempDir()
	if err := MaterializeTo(root); err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(root, "init", SkillFileName)
	data, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("SKILL.md not materialized: %v", err)
	}
	if !strings.Contains(string(data), "init") {
		t.Fatalf("SKILL.md content unexpected:\n%s", data)
	}

	if err := MaterializeTo(root); err != nil {
		t.Fatalf("materialize must be idempotent: %v", err)
	}
}

func TestMaterializedSkillParsesAndExpands(t *testing.T) {
	root := t.TempDir()
	if err := MaterializeTo(root); err != nil {
		t.Fatal(err)
	}
	if err := LoadDirs(root); err != nil {
		t.Fatal(err)
	}

	s, ok := Get("init")
	if !ok {
		t.Fatal("init must be registered")
	}

	out := ExpandMessage(agent.MessageInput{Text: "implement it /init"})
	if out.Text != "implement it " {
		t.Fatalf("skill token must be stripped: %q", out.Text)
	}
	if len(out.Calls) != 1 || out.Calls[0].Function.Name != "loadskill" {
		t.Fatalf("expected a loadskill tool call, got %+v", out.Calls)
	}

	infos := []prompt.SkillInfo{{Name: s.Name, Description: s.Description}}
	md, err := prompt.New().Skills(infos).Build(prompt.FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "init") {
		t.Fatalf("prompt listing missing skill: %q", md)
	}
}

func TestSkillModule(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SkillModule(); err != nil {
		t.Fatalf("SkillModule: %v", err)
	}
	if err := SkillModule(); err != nil {
		t.Fatalf("SkillModule must be idempotent: %v", err)
	}
}

func TestSkillifyMaterializesAndLoads(t *testing.T) {
	root := t.TempDir()
	if err := MaterializeTo(root); err != nil {
		t.Fatal(err)
	}
	if err := LoadDirs(root); err != nil {
		t.Fatal(err)
	}
	s, ok := Get("skillify")
	if !ok {
		t.Fatal("skillify must be registered after materialization")
	}
	if s.Description == "" || s.WhenToUse == "" {
		t.Fatalf("skillify metadata incomplete: %+v", s)
	}
	if !strings.Contains(s.Instructions, ".pegg/skills/") {
		t.Error("skillify instructions should reference .pegg/skills/")
	}
	if strings.Contains(s.Instructions, ".claude") {
		t.Error("skillify instructions should not reference .claude")
	}
}
