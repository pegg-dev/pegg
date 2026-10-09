package shared

import (
	"strings"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/builtin/agents/shared/prompts"
	"github.com/peggco/pegg/internal/skill"
)

func SharedPromptBuilder(providerID, modelID string) *prompt.Prompt {
	var p *prompt.Prompt

	modelIDLower := strings.ToLower(modelID)
	providerIDLower := strings.ToLower(providerID)

	switch {
	case strings.Contains(modelIDLower, "gpt-4") || strings.Contains(modelIDLower, "o1") || strings.Contains(modelIDLower, "o3"):
		p = prompts.BeastPromptBuilder()
	case strings.Contains(modelIDLower, "gpt"):
		if strings.Contains(modelIDLower, "gpt-6") {
			p = prompts.GPTAstraPromptBuilder()
		} else if strings.Contains(modelIDLower, "codex") {
			p = prompts.CodexPromptBuilder()
		} else if strings.Contains(modelIDLower, "copilot") {
			p = prompts.CopilotGPT5PromptBuilder()
		} else {
			p = prompts.GPTPromptBuilder()
		}
	case strings.Contains(modelIDLower, "gemini-"):
		p = prompts.GeminiPromptBuilder()
	case strings.Contains(modelIDLower, "claude"):
		p = prompts.AntrophicPromptBuilder()
	case strings.Contains(modelIDLower, "trinity"):
		p = prompts.TrinityPromptBuilder()
	case strings.Contains(modelIDLower, "kimi") ||
		providerIDLower == "kimi-for-coding" ||
		providerIDLower == "moonshotai" ||
		providerIDLower == "moonshotai-cn":
		p = prompts.KimiPromptBuilder()
	default:
		p = prompts.DefaultPromptBuilder()
	}

	return p.
		SetVars(sharedVars(providerID, modelID)).
		Paragraph("Here is some useful information about the environment you are running in:").
		XMLTag("env",
			prompt.Paragraph("Working directory: {{env.Working_directory}}"),
			prompt.Paragraph("Platform: {{env.platform}}"),
			prompt.Paragraph("OS Version: {{env.os_version}}"),
			prompt.Paragraph("Today's date: {{env.date}}")).
		Paragraph("You are powered by the model named {{model}}.").
		Add(prompt.IfFn(
			func(vars prompt.Vars) bool { return len(skill.List()) > 0 },
			prompt.Heading(1, "Available Skills"),
			prompt.Paragraph("You can load a skill into the conversation by including `/<skill-name>` in your input or in a subagent task message. Its instructions are injected automatically."),
			prompt.SkillsList(skillInfos()),
		)).
		AgentsMd().
		Rules()
}

func skillInfos() []prompt.SkillInfo {
	all := skill.List()
	infos := make([]prompt.SkillInfo, 0, len(all))
	for _, s := range all {
		infos = append(infos, prompt.SkillInfo{Name: s.Name, Description: s.Description})
	}
	return infos
}
