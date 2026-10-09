package orchestrator

import (
	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/builtin/agents/shared"
)

func generateOrchestratorPrompt(providerID, modelID string) (string, error) {
	sys, err := shared.SharedPromptBuilder(providerID, modelID).
		Build(prompt.FormatMarkdown)
	if err != nil {
		return "", err
	}
	return sys, nil
}
