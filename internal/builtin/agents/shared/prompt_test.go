package shared

import (
	"os"
	"strings"
	"testing"

	"github.com/peggco/pegg/internal/agent/prompt"
)

func TestEnvVars(t *testing.T) {
	v := envVars()
	env, ok := v["env"].(map[string]any)
	if !ok {
		t.Fatalf("env vars missing: %#v", v)
	}
	for _, key := range []string{"Working_directory", "platform", "os_version", "date", "shell"} {
		if env[key] == "" {
			t.Fatalf("env.%s is empty", key)
		}
	}
	if env["platform"] != "linux" && env["platform"] != "darwin" && env["platform"] != "windows" {
		t.Fatalf("unexpected platform %q", env["platform"])
	}
}

func TestGitVarsInRepo(t *testing.T) {
	v := gitVars()
	git := v["git"].(map[string]any)
	if git["enabled"] != true {
		t.Skip("not inside a git repository")
	}
	if git["main_branch"] == "" {
		t.Fatal("git.main_branch empty in a git repo")
	}
	if _, ok := git["status"]; !ok {
		t.Fatal("git.status missing")
	}
}

func TestGitVarsNotInRepo(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	v := gitVars()
	git := v["git"].(map[string]any)
	if git["enabled"] != false {
		t.Fatalf("expected git.enabled=false outside a repo, got %#v", git["enabled"])
	}
	for _, key := range []string{"branch", "main_branch", "status", "recent_commits"} {
		if git[key] != "" {
			t.Fatalf("git.%s should be empty outside a repo, got %q", key, git[key])
		}
	}
}

func TestSharedPromptBuilds(t *testing.T) {
	out, err := SharedPromptBuilder("", "").Build(prompt.FormatMarkdown)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if out == "" {
		t.Fatal("empty prompt")
	}
	for _, want := range []string{
		"interactive CLI tool",
		"Working directory:",
		"Platform:",
		"Today",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSharedPromptBuildsInNonRepo(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	if _, err := SharedPromptBuilder("", "").Build(prompt.FormatMarkdown); err != nil {
		t.Fatalf("build failed outside a repo: %v", err)
	}
}
