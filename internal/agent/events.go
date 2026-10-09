package agent

import "github.com/peggco/pegg/internal/llm"

const (
	TopicAgentStarted         = "agent.started"
	TopicAgentInput           = "agent.input"
	TopicAgentMessage         = "agent.message"
	TopicAgentToken           = "agent.token"
	TopicAgentToolCall        = "agent.tool.call"
	TopicAgentToolResult      = "agent.tool.result"
	TopicAgentUsage           = "agent.usage"
	TopicAgentFinished        = "agent.finished"
	TopicAgentError           = "agent.error"
	TopicAgentAsk             = "agent.ask"
	TopicAgentAskAnswer       = "agent.ask.answer"
	TopicErrorMessage         = "agent.error.message"
	TopicErrorMessageFinished = "agent.error.message.finished"
	TopicSubAgentNotification = "agent.subagent.notification"
)

type AgentInput struct {
	AgentID     string
	AgentName   string
	Model       llm.Model
	Input       string
	Attachments []llm.Attachment
}

type AgentStarted struct {
	AgentID         string
	AgentName       string
	DisplayName     string
	ParentAgentID   string
	Model           llm.Model
	Provider        llm.Provider
	ReasoningEffort string
	Agent           *Agent
}

type AgentMessage struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	Message   llm.Message
}

type AgentToken struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	Content   string
	Reasoning string
}

type AgentToolCall struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	Call      llm.ToolCall
}

type AgentToolResult struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	CallID    string
	ToolName  string
	Output    string
	Err       error
}

type AgentUsage struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	Usage     llm.Usage
}

type AgentFinished struct {
	AgentID      string
	AgentName    string
	Model        llm.Model
	Output       string
	Usage        llm.Usage
	Iterations   int
	FinishReason llm.FinishReason
}

type AgentError struct {
	AgentID   string
	AgentName string
	Model     llm.Model
	Err       error
}

type AgentAsk struct {
	AgentID   string
	AgentName string
	Questions []AskQuestion
}

type AskQuestion struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required"`
}

type AgentAskAnswer struct {
	AgentID string
	Answers map[string]string
}

type ErrorMessage struct {
	AgentID   string
	AgentName string
	Message   string
}

type ErrorMessageFinished struct {
	AgentID   string
	AgentName string
}

type SubAgentNotification struct {
	ParentAgentID string
	SubAgentName  string
	TaskIDs       []string
	Output        string
	Err           string
}
