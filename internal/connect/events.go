package connect

import (
	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/llm"
)

type runStartedPayload struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
}

type runTokenPayload struct {
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

type runToolCallPayload struct {
	CallID   string `json:"call_id,omitempty"`
	ToolName string `json:"tool_name"`
	Args     string `json:"args,omitempty"`
}

type runToolResultPayload struct {
	CallID   string `json:"call_id,omitempty"`
	ToolName string `json:"tool_name"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

type runAskPayload struct {
	AgentName string              `json:"agent_name,omitempty"`
	Questions []agent.AskQuestion `json:"questions"`
}

type runUsagePayload struct {
	llm.Usage
}

type runCompactionPayload struct {
	Strategy string `json:"strategy,omitempty"`
	Messages int    `json:"messages,omitempty"`
	Tokens   int    `json:"tokens,omitempty"`
}

type runProgressPayload struct {
	ElapsedSecs   int  `json:"elapsed_secs"`
	AwaitingModel bool `json:"awaiting_model,omitempty"`
}

type runDonePayload struct {
	Output       string           `json:"output,omitempty"`
	Usage        llm.Usage        `json:"usage"`
	Iterations   int              `json:"iterations"`
	FinishReason llm.FinishReason `json:"finish_reason,omitempty"`
}

type runErrorPayload struct {
	Message string `json:"message"`
}

type runAgentErrorPayload struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Message   string `json:"message"`
}

type runWarningPayload struct {
	Message string `json:"message"`
}
