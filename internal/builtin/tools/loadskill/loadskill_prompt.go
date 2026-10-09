package loadskill

import (
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/skill"
)

func loadSkillPromptBuilder() *prompt.Prompt {
	p := prompt.New().
		Paragraph("Use this tool to load a skill by name and optionally pass arguments to it.").
		Paragraph("Usage:").
		List(
			"Provide the skill name exactly as registered.",
			"Optionally pass an arguments map to substitute $arg_name placeholders in the skill content.",
			"If the skill defines 'arguments' in its frontmatter, supply values for those names.",
			"The tool returns the expanded skill content with arguments substituted.",
		).
		Add(prompt.IfFn(
			func(vars prompt.Vars) bool { return len(skill.List()) > 0 },
			prompt.Paragraph("Available Skills:"),
			prompt.SkillsList(skillInfos()),
		))

	return p
}

func skillInfos() []prompt.SkillInfo {
	all := skill.List()
	infos := make([]prompt.SkillInfo, 0, len(all))
	for _, s := range all {
		infos = append(infos, prompt.SkillInfo{
			Name:         s.Name,
			Description:  s.Description,
			WhenToUse:    s.WhenToUse,
			ArgumentHint: s.ArgumentHint,
			Arguments:    s.Arguments,
			Context:      s.Context,
		})
	}
	return infos
}
