package sdk

import (
	"context"

	"github.com/peggco/pegg/internal/agent"
	"github.com/peggco/pegg/internal/agent/agents"
	"github.com/peggco/pegg/internal/agent/middleware"
	"github.com/peggco/pegg/internal/agent/tool"
	"github.com/peggco/pegg/internal/builtin/middlewares/compaction"
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/core/logger"
	"github.com/peggco/pegg/internal/llm"
	"github.com/peggco/pegg/internal/session"
	"github.com/peggco/pegg/internal/skill"
	"github.com/peggco/pegg/internal/vfs"
)

type (
	Agent        = agent.Agent
	RunResult    = agent.RunResult
	AgentFactory = agents.Factory

	Tool             = tool.Tool
	ToolRegistry     = tool.Registry
	Middleware       = middleware.Middleware
	BaseMiddleware   = middleware.BaseMiddleware
	MiddlewareResult = middleware.Result

	Role           = llm.Role
	FinishReason   = llm.FinishReason
	Model          = llm.Model
	ModelConfig    = llm.ModelConfig
	Message        = llm.Message
	Content        = llm.Content
	Attachment     = llm.Attachment
	AttachmentType = llm.AttachmentType
	ToolCall       = llm.ToolCall
	Usage          = llm.Usage
	Request        = llm.Request
	Response       = llm.Response
	Choice         = llm.Choice
	StreamChunk    = llm.StreamChunk
	StreamHandler  = llm.StreamHandler
	StreamResponse = llm.StreamResponse
	Provider       = llm.Provider
	ProviderError  = llm.ProviderError
	JSONSchema     = llm.JSONSchema
	ResponseFormat = llm.ResponseFormat
	LLMTool        = llm.Tool

	Session         = session.Session
	SessionMessage  = session.Message
	SessionSnapshot = session.Snapshot

	AgentStarted    = agent.AgentStarted
	AgentInput      = agent.AgentInput
	AgentMessage    = agent.AgentMessage
	AgentToken      = agent.AgentToken
	AgentToolCall   = agent.AgentToolCall
	AgentToolResult = agent.AgentToolResult
	AgentUsage      = agent.AgentUsage
	AgentFinished   = agent.AgentFinished
	AgentError      = agent.AgentError
	AgentAsk        = agent.AgentAsk
	AskQuestion     = agent.AskQuestion
	AgentAskAnswer  = agent.AgentAskAnswer

	SessionResume         = session.SessionResume
	SessionAttached       = session.SessionAttached
	SessionCreated        = session.SessionCreated
	SessionDeleted        = session.SessionDeleted
	SessionUpdated        = session.SessionUpdated
	SessionMessageAdded   = session.SessionMessageAdded
	SessionForked         = session.SessionForked
	SessionReverted       = session.SessionReverted
	SessionRestored       = session.SessionRestored
	SessionCurrentChanged = session.SessionCurrentChanged

	Skill = skill.Skill

	VFS        = vfs.VFS
	FileInfo   = vfs.FileInfo
	ListResult = vfs.ListResult

	Logger = logger.Logger
	Level  = logger.Level

	Config    = config.Config
	LLMConfig = config.LLMConfig
)

const (
	TopicAgentStarted    = agent.TopicAgentStarted
	TopicAgentInput      = agent.TopicAgentInput
	TopicAgentMessage    = agent.TopicAgentMessage
	TopicAgentToken      = agent.TopicAgentToken
	TopicAgentToolCall   = agent.TopicAgentToolCall
	TopicAgentToolResult = agent.TopicAgentToolResult
	TopicAgentUsage      = agent.TopicAgentUsage
	TopicAgentFinished   = agent.TopicAgentFinished
	TopicAgentError      = agent.TopicAgentError
	TopicAgentAsk        = agent.TopicAgentAsk
	TopicAgentAskAnswer  = agent.TopicAgentAskAnswer

	TopicCompactionStarted  = compaction.TopicCompactionStarted
	TopicCompactionFinished = compaction.TopicCompactionFinished
)

type CompactionEvent = compaction.Event

const (
	TopicSessionCreated        = session.TopicSessionCreated
	TopicSessionDeleted        = session.TopicSessionDeleted
	TopicSessionUpdated        = session.TopicSessionUpdated
	TopicSessionMessageAdded   = session.TopicSessionMessageAdded
	TopicSessionForked         = session.TopicSessionForked
	TopicSessionReverted       = session.TopicSessionReverted
	TopicSessionRestored       = session.TopicSessionRestored
	TopicSessionCurrentChanged = session.TopicSessionCurrentChanged
	TopicSessionResume         = session.TopicSessionResume
	TopicSessionAttached       = session.TopicSessionAttached
)

const (
	RoleUser      = llm.RoleUser
	RoleAssistant = llm.RoleAssistant
	RoleSystem    = llm.RoleSystem
	RoleTool      = llm.RoleTool
)

const (
	FinishReasonStop          = llm.FinishReasonStop
	FinishReasonLength        = llm.FinishReasonLength
	FinishReasonContentFilter = llm.FinishReasonContentFilter
	FinishReasonToolCalls     = llm.FinishReasonToolCalls
	FinishReasonNull          = llm.FinishReasonNull
)

const (
	AttachmentTypeImage = llm.AttachmentTypeImage
	AttachmentTypeAudio = llm.AttachmentTypeAudio
	AttachmentTypeFile  = llm.AttachmentTypeFile
)

const (
	LevelDebug = logger.LevelDebug
	LevelInfo  = logger.LevelInfo
	LevelWarn  = logger.LevelWarn
	LevelError = logger.LevelError
)

func UserMessage(content any) Message      { return llm.UserMessage(content) }
func SystemMessage(content any) Message    { return llm.SystemMessage(content) }
func AssistantMessage(content any) Message { return llm.AssistantMessage(content) }
func ToolMessage(content, toolCallID string) Message {
	return llm.ToolMessage(content, toolCallID)
}

func TextContent(text string) Content { return llm.TextContent(text) }

func NewImageAttachmentFromBase64(mediaType, data string) Attachment {
	return llm.NewImageAttachmentFromBase64(mediaType, data)
}

func NewImageAttachmentFromURL(url string) Attachment {
	return llm.NewImageAttachmentFromURL(url)
}

func NewAudioAttachmentFromBase64(mediaType, data string) Attachment {
	return llm.NewAudioAttachmentFromBase64(mediaType, data)
}

func NewAudioAttachmentFromURL(url string) Attachment {
	return llm.NewAudioAttachmentFromURL(url)
}

func NewFileAttachmentFromBase64(mediaType, data, fileName string) Attachment {
	return llm.NewFileAttachmentFromBase64(mediaType, data, fileName)
}

func NewFileAttachmentFromURL(url, fileName string) Attachment {
	return llm.NewFileAttachmentFromURL(url, fileName)
}

func EncodeFileToBase64(data []byte) string { return llm.EncodeFileToBase64(data) }

func NewTool(name, description string, parameters any, fn func(ctx context.Context, args string) (string, error)) Tool {
	return tool.NewSpec(name, description, parameters, fn)
}
