package middleware

import "github.com/peggco/pegg/internal/llm"

type Result struct {
	AgentName    string
	Model        llm.Model
	Provider     string
	Output       string
	History      []llm.Message
	Usage        llm.Usage
	Iterations   int
	FinishReason llm.FinishReason
}
