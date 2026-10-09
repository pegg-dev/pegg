package acp

import (
	"context"

	json "github.com/goccy/go-json"

	"github.com/peggco/pegg/internal/core/config"
)

type InitializeParams struct {
	ProtocolVersion    int                 `json:"protocolVersion"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
	ClientInfo         *ClientInfo         `json:"clientInfo,omitempty"`
}

type ClientCapabilities struct {
	FS          *FSCapabilities          `json:"fs,omitempty"`
	Terminal    *bool                    `json:"terminal,omitempty"`
	Elicitation *ElicitationCapabilities `json:"elicitation,omitempty"`
	Auth        *ClientAuthCapabilities  `json:"auth,omitempty"`
	Session     *ClientSessionCaps       `json:"session,omitempty"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type ElicitationCapabilities struct {
	Form *struct{} `json:"form,omitempty"`
	URL  *struct{} `json:"url,omitempty"`
}

type ClientAuthCapabilities struct {
	Terminal bool `json:"terminal,omitempty"`
}

type ClientSessionCaps struct {
	ConfigOptions *ClientConfigOptions `json:"configOptions,omitempty"`
}

type ClientConfigOptions struct {
	Boolean *struct{} `json:"boolean,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         *AgentInfo        `json:"agentInfo,omitempty"`
	AuthMethods       []AuthMethod      `json:"authMethods,omitempty"`
}

type AgentCapabilities struct {
	LoadSession        bool              `json:"loadSession"`
	PromptCapabilities *PromptCaps       `json:"promptCapabilities,omitempty"`
	MCPCapabilities    *MCPCaps          `json:"mcpCapabilities,omitempty"`
	SessionCaps        *AgentSessionCaps `json:"sessionCapabilities,omitempty"`
	Auth               *AgentAuthCaps    `json:"auth,omitempty"`
}

type PromptCaps struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

type MCPCaps struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

type AgentSessionCaps struct {
	Delete                *struct{} `json:"delete,omitempty"`
	Resume                *struct{} `json:"resume,omitempty"`
	Close                 *struct{} `json:"close,omitempty"`
	AdditionalDirectories *struct{} `json:"additionalDirectories,omitempty"`
}

type AgentAuthCaps struct {
	Logout *struct{} `json:"logout,omitempty"`
}

type AgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type AuthMethod struct {
	Type string `json:"type"`
}

func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
	var req InitializeParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid initialize params"}
	}

	if req.ProtocolVersion < 1 {
		return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "unsupported protocol version"}
	}

	caps := AgentCapabilities{
		LoadSession: true,
		PromptCapabilities: &PromptCaps{
			Image:           true,
			Audio:           false,
			EmbeddedContext: true,
		},
		MCPCapabilities: &MCPCaps{
			HTTP: true,
			SSE:  false,
		},
		SessionCaps: &AgentSessionCaps{
			Delete:                &struct{}{},
			Resume:                &struct{}{},
			Close:                 &struct{}{},
			AdditionalDirectories: &struct{}{},
		},
	}

	return InitializeResult{
		ProtocolVersion:   1,
		AgentCapabilities: caps,
		AgentInfo: &AgentInfo{
			Name:    "pegg",
			Title:   "Pegg AI Agent",
			Version: config.AppVersion,
		},
		AuthMethods: []AuthMethod{},
	}, nil
}
