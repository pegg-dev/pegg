package skills

import (
	"github.com/peggco/pegg/internal/agent/prompt"
)

func All() map[string]func() *prompt.Prompt {
	return map[string]func() *prompt.Prompt{
		"batch":    BatchSkill,
		"review":   ReviewSkill,
		"init":     InitSkill,
		"rule":     RuleSkill,
		"skillify": SkillifySkill,
	}
}
