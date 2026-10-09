package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/builtin/skills"
	"github.com/peggco/pegg/internal/core/config"
)

const agentsSkillsDir = ".agents/skills"

func SkillModule() error {
	peggSkills, err := config.GetConfigPath("skills")
	if err != nil {
		return err
	}
	if err := MaterializeTo(peggSkills); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("skill: home dir: %w", err)
	}
	projSkills, err := config.GetProjectConfigPath("skills")
	if err != nil {
		return err
	}
	if err := LoadDirs(
		filepath.Join(home, agentsSkillsDir),
		peggSkills,
		projSkills,
	); err != nil {
		return err
	}
	agent.OnMessageInput(ExpandMessage)
	return nil
}

func MaterializeTo(root string) error {
	for name, build := range skills.All() {
		targetDir := filepath.Join(root, name)
		if dirExists(targetDir) {
			continue
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("skill: mkdir %s: %w", targetDir, err)
		}
		skillMD, err := build().Build(prompt.FormatMarkdown)
		if err != nil {
			return fmt.Errorf("skill: render %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(targetDir, SkillFileName), []byte(skillMD), 0o644); err != nil {
			return fmt.Errorf("skill: write %s: %w", targetDir, err)
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
